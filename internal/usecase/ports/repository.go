package ports

import (
	"context"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type ConversationRepository interface {
	ListConversations(context.Context, string, string, bool, int, *time.Time) ([]entities.Conversation, *time.Time, error)
	CreateConversation(context.Context, entities.Actor, entities.Workspace, entities.ModelSnapshot) (entities.Conversation, error)
	ResetConversation(context.Context, entities.Actor, entities.Workspace, string, entities.ModelSnapshot) (entities.Conversation, error)
	UpdateConversationModel(context.Context, string, string, string, entities.ModelSnapshot) (entities.Conversation, error)
	ListMessages(context.Context, string, string, string, int, *int64) ([]entities.Message, *int64, error)
	StartRun(context.Context, entities.RunInput) (entities.Run, error)
	CompleteRun(context.Context, string, string, string) (entities.Message, error)
	FailRun(context.Context, string, string, string, string) error
}

type InteractionRepository interface {
	GetActiveInteraction(context.Context, string, string, string) (*entities.Interaction, error)
	GetInteraction(context.Context, string, string, string, string) (*entities.Interaction, error)
	GetOpenClarification(context.Context, string, string, string) (*entities.Clarification, error)
	CreateInteraction(context.Context, string, string, string, string, string, string) (entities.Interaction, error)
	AttachRunInteraction(context.Context, string, string) error
	SupersedeInteraction(context.Context, string) error
	SaveInteraction(context.Context, entities.Interaction, int) (entities.Interaction, error)
	CreateClarification(context.Context, string, entities.Interaction, entities.Clarification) (entities.Clarification, entities.Message, error)
	ApplyClarification(context.Context, entities.ClarificationAnswer, entities.Interaction, int) (entities.Interaction, error)
	CompleteInteractionRun(context.Context, string, string, string, string) (entities.Message, error)
	FailInteraction(context.Context, string, string, string, string, string) error
}
