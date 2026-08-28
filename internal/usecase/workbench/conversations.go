package workbench

import (
	"context"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
)

func (u *UseCase) ListConversations(ctx context.Context, actorID, workspaceID string, includeArchived bool, limit int, cursor string) ([]entities.Conversation, string, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, "", domainerrors.ErrInvalidInput
	}
	if limit == 0 {
		limit = 50
	}
	before, err := decodeTimeCursor(cursor)
	if err != nil {
		return nil, "", domainerrors.ErrInvalidInput
	}
	items, next, err := u.repository.ListConversations(ctx, actorID, workspaceID, includeArchived, limit, before)
	if err != nil {
		return nil, "", err
	}
	return items, encodeTimeCursor(next), nil
}

func (u *UseCase) CreateConversation(ctx context.Context, actor entities.Actor, workspace entities.Workspace, model entities.ModelSnapshot) (entities.Conversation, error) {
	if err := validateCreate(actor, workspace, model); err != nil {
		return entities.Conversation{}, err
	}
	return u.repository.CreateConversation(ctx, actor, workspace, model)
}

func (u *UseCase) ResetConversation(ctx context.Context, actor entities.Actor, workspace entities.Workspace, currentID string, model entities.ModelSnapshot) (entities.Conversation, error) {
	if err := validateCreate(actor, workspace, model); err != nil {
		return entities.Conversation{}, err
	}
	return u.repository.ResetConversation(ctx, actor, workspace, currentID, model)
}

func (u *UseCase) UpdateConversationModel(ctx context.Context, actorID, workspaceID, conversationID string, model entities.ModelSnapshot) (entities.Conversation, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(workspaceID) == "" || !validUUID(conversationID) || !validModel(model) {
		return entities.Conversation{}, domainerrors.ErrInvalidInput
	}
	return u.repository.UpdateConversationModel(ctx, actorID, workspaceID, conversationID, model)
}

func (u *UseCase) ListMessages(ctx context.Context, actorID, workspaceID, conversationID string, limit int, cursor string) ([]entities.Message, string, *entities.Clarification, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(workspaceID) == "" || !validUUID(conversationID) {
		return nil, "", nil, domainerrors.ErrInvalidInput
	}
	if limit == 0 {
		limit = 50
	}
	before, err := decodeSequenceCursor(cursor)
	if err != nil {
		return nil, "", nil, domainerrors.ErrInvalidInput
	}
	items, next, err := u.repository.ListMessages(ctx, actorID, workspaceID, conversationID, limit, before)
	if err != nil {
		return nil, "", nil, err
	}
	var clarification *entities.Clarification
	if u.interactions != nil {
		clarification, err = u.interactions.GetOpenClarification(ctx, actorID, workspaceID, conversationID)
		if err != nil {
			return nil, "", nil, err
		}
	}
	return items, encodeSequenceCursor(next), clarification, nil
}
