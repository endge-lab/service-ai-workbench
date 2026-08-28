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
	return entities.ModelRequest{
		Model: input.RunInput.Model, SystemPrompt: system.Content,
		Messages: []entities.ModelMessage{{Role: "user", Content: request.Content}},
	}, []entities.PromptUsage{{ID: system.ID, Version: system.Version, SHA256: system.SHA256}, {ID: request.ID, Version: request.Version, SHA256: request.SHA256}}, nil
}
