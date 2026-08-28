package preparation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

// ModelRequestAssembler receives only the resolved plan and selected blocks.
// It cannot retrieve or resolve additional context.
type ModelRequestAssembler struct{ prompts ports.PromptCatalog }

func NewModelRequestAssembler(prompts ports.PromptCatalog) ModelRequestAssembler {
	return ModelRequestAssembler{prompts: prompts}
}

func (a ModelRequestAssembler) OverheadChars(ctx context.Context, input Input, plan entities.TaskPlan) (int, error) {
	request, _, err := a.Assemble(ctx, input, plan, nil)
	if err != nil {
		return 0, err
	}
	total := len(request.SystemPrompt)
	for _, message := range request.Messages {
		total += len(message.Content)
	}
	return total, nil
}

func (a ModelRequestAssembler) Assemble(ctx context.Context, input Input, plan entities.TaskPlan, blocks []entities.RetrievedBlock) (entities.ModelRequest, []entities.PromptUsage, error) {
	if blocks == nil {
		blocks = []entities.RetrievedBlock{}
	}
	payload := map[string]any{
		"request":      input.RunInput.Prompt,
		"plan":         plan,
		"context":      blocks,
		"conversation": minimalHistory(input.Conversation.Messages, 8),
		"workspace":    map[string]string{"generation": input.RunInput.Generation, "snapshotSha256": input.RunInput.SnapshotSHA256},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return entities.ModelRequest{}, nil, fmt.Errorf("encode final model request: %w", err)
	}
	system, err := a.prompts.Render(ctx, entities.PromptFinalAnswerSystem, nil)
	if err != nil {
		return entities.ModelRequest{}, nil, err
	}
	request, err := a.prompts.Render(ctx, entities.PromptFinalAnswerRequest, map[string]string{"PayloadJSON": string(encoded)})
	if err != nil {
		return entities.ModelRequest{}, nil, err
	}
	responseFormat, err := finalAnswerSchema(plan, blocks)
	if err != nil {
		return entities.ModelRequest{}, nil, fmt.Errorf("build final answer schema: %w", err)
	}
	return entities.ModelRequest{
		Model: input.RunInput.Model, SystemPrompt: system.Content,
		Messages: []entities.ModelMessage{{Role: "user", Content: request.Content}}, ResponseFormat: responseFormat,
	}, []entities.PromptUsage{{ID: system.ID, Version: system.Version, SHA256: system.SHA256}, {ID: request.ID, Version: request.Version, SHA256: request.SHA256}}, nil
}

func finalAnswerSchema(plan entities.TaskPlan, blocks []entities.RetrievedBlock) (json.RawMessage, error) {
	entityOptions := make([]any, 0)
	for _, citation := range allowedEntityCitations(plan, blocks) {
		entityOptions = append(entityOptions, map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"documentType", "identity"},
			"properties": map[string]any{
				"documentType": map[string]any{"const": citation.DocumentType},
				"identity":     map[string]any{"const": citation.Identity},
			},
		})
	}
	documentationIDs := make([]string, 0)
	seenDocumentation := map[string]struct{}{}
	for _, block := range blocks {
		if block.SourceKind != "documentation" {
			continue
		}
		if _, exists := seenDocumentation[block.SourceKey]; exists {
			continue
		}
		seenDocumentation[block.SourceKey] = struct{}{}
		documentationIDs = append(documentationIDs, block.SourceKey)
	}

	entityCitations := map[string]any{"type": "array", "uniqueItems": true}
	if len(entityOptions) == 0 {
		entityCitations["maxItems"] = 0
		entityCitations["items"] = map[string]any{"type": "object"}
	} else {
		entityCitations["items"] = map[string]any{"oneOf": entityOptions}
	}
	documentationCitations := map[string]any{"type": "array", "uniqueItems": true}
	if len(documentationIDs) == 0 {
		documentationCitations["maxItems"] = 0
		documentationCitations["items"] = map[string]any{"type": "string"}
	} else {
		documentationCitations["items"] = map[string]any{"type": "string", "enum": documentationIDs}
	}

	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"answer", "entityCitations", "documentationCitations", "limitations"},
		"properties": map[string]any{
			"answer":                 map[string]any{"type": "string"},
			"entityCitations":        entityCitations,
			"documentationCitations": documentationCitations,
			"limitations":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
	return json.Marshal(schema)
}
