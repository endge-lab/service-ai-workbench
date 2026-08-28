package ports

import (
	"context"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type KnowledgeRetriever interface {
	Retrieve(context.Context, string, int) entities.KnowledgeRetrieval
}

type DomainContextSelector interface {
	Select(context.Context, entities.DomainSelectionInput) entities.DomainContext
}
