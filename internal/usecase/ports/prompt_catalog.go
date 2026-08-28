package ports

import (
	"context"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type PromptCatalog interface {
	Render(context.Context, entities.PromptID, any) (entities.RenderedPrompt, error)
}
