package preparation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/modelcalls"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type modelCallBudget struct {
	limit int
	used  int
}

func (b *modelCallBudget) consume() error {
	if b.used >= b.limit {
		return fmt.Errorf("preparation model call budget exceeded")
	}
	b.used++
	return nil
}

func invokeStructured(
	ctx context.Context,
	prompts ports.PromptCatalog,
	models ports.StructuredModelInvoker,
	input entities.RunInput,
	systemID entities.PromptID,
	requestID entities.PromptID,
	payload []byte,
	responseFormat json.RawMessage,
	budget *modelCallBudget,
) ([]byte, []entities.PromptUsage, error) {
	if err := budget.consume(); err != nil {
		return nil, nil, err
	}
	if err := modelcalls.Consume(ctx); err != nil {
		return nil, nil, err
	}
	system, err := prompts.Render(ctx, systemID, nil)
	if err != nil {
		return nil, nil, err
	}
	request, err := prompts.Render(ctx, requestID, map[string]string{"PayloadJSON": string(payload)})
	if err != nil {
		return nil, nil, err
	}
	usages := []entities.PromptUsage{
		{ID: system.ID, Version: system.Version, SHA256: system.SHA256},
		{ID: request.ID, Version: request.Version, SHA256: request.SHA256},
	}
	raw, err := models.Invoke(ctx, ports.StructuredModelRequest{
		Model:          input.Model,
		ProviderAccess: input.ProviderAccess,
		SystemPrompt:   system.Content,
		UserPrompt:     request.Content,
		ResponseFormat: responseFormat,
	})
	return raw, usages, err
}

func repairStructured(
	ctx context.Context,
	prompts ports.PromptCatalog,
	models ports.StructuredModelInvoker,
	input entities.RunInput,
	raw []byte,
	expected json.RawMessage,
	validationErrors []string,
	budget *modelCallBudget,
) ([]byte, []entities.PromptUsage, error) {
	payload, err := json.Marshal(map[string]any{"response": string(raw), "expectedSchema": json.RawMessage(expected), "validationErrors": validationErrors})
	if err != nil {
		return nil, nil, err
	}
	return invokeStructured(ctx, prompts, models, input, entities.PromptRepairSystem, entities.PromptRepairRequest, payload, expected, budget)
}

func extractJSONObject(raw []byte) []byte {
	value := strings.TrimSpace(string(raw))
	if start := strings.Index(value, "{"); start >= 0 {
		if end := strings.LastIndex(value, "}"); end >= start {
			return []byte(value[start : end+1])
		}
	}
	return raw
}
