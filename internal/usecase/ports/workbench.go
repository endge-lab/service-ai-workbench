package ports

import (
	"context"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type ConversationRepository interface {
	ListConversations(ctx context.Context, actorID, workspaceID string, includeArchived bool, limit int, before *time.Time) ([]entities.Conversation, *time.Time, error)
	CreateConversation(ctx context.Context, actor entities.Actor, workspace entities.Workspace, model entities.ModelSnapshot) (entities.Conversation, error)
	ResetConversation(ctx context.Context, actor entities.Actor, workspace entities.Workspace, currentID string, model entities.ModelSnapshot) (entities.Conversation, error)
	UpdateConversationModel(ctx context.Context, actorID, workspaceID, conversationID string, model entities.ModelSnapshot) (entities.Conversation, error)
	ListMessages(ctx context.Context, actorID, workspaceID, conversationID string, limit int, beforeSequence *int64) ([]entities.Message, *int64, error)
	StartRun(ctx context.Context, input entities.RunInput) (entities.Run, error)
	CompleteRun(ctx context.Context, runID, conversationID, content string) (entities.Message, error)
	FailRun(ctx context.Context, runID, status, code, message string) error
}

type Generator interface {
	Generate(ctx context.Context, prompt string, model entities.ModelSnapshot) ([]string, error)
}

type GeneratorResolver interface {
	Resolve(adapter string) (Generator, bool)
}
