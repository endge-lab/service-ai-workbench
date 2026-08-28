package ports

import (
	"context"
	"encoding/json"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type Generator interface {
	Generate(context.Context, entities.GenerationRequest, func(string) error) error
}

type GeneratorResolver interface {
	Resolve(string) (Generator, bool)
}

type StructuredModelRequest struct {
	Model                    entities.ModelSnapshot
	ProviderAccess           entities.ProviderAccess
	SystemPrompt, UserPrompt string
	ResponseFormat           json.RawMessage
}

type StructuredModelInvoker interface {
	Invoke(context.Context, StructuredModelRequest) ([]byte, error)
}
