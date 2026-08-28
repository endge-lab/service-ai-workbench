package ports

import (
	"context"

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
}

type StructuredModelInvoker interface {
	Invoke(context.Context, StructuredModelRequest) ([]byte, error)
}
