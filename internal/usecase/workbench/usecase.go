package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	"github.com/google/uuid"
)

const (
	EventStarted = iota + 1
	EventContentDelta
	EventCompleted
	EventFailed
)

type Event struct {
	Type         int
	RunID        string
	MessageID    string
	Delta        string
	ErrorCode    string
	ErrorMessage string
	CreatedAt    time.Time
}

type UseCase struct {
	repository ports.ConversationRepository
	generators ports.GeneratorResolver
}

func NewUseCase(repository ports.ConversationRepository, generators ports.GeneratorResolver) *UseCase {
	return &UseCase{repository: repository, generators: generators}
}

func (u *UseCase) Capabilities() []string { return []string{"anthropic", "ollama"} }

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

func (u *UseCase) ListMessages(ctx context.Context, actorID, workspaceID, conversationID string, limit int, cursor string) ([]entities.Message, string, error) {
	if strings.TrimSpace(actorID) == "" || strings.TrimSpace(workspaceID) == "" || !validUUID(conversationID) {
		return nil, "", domainerrors.ErrInvalidInput
	}
	if limit == 0 {
		limit = 50
	}
	before, err := decodeSequenceCursor(cursor)
	if err != nil {
		return nil, "", domainerrors.ErrInvalidInput
	}
	items, next, err := u.repository.ListMessages(ctx, actorID, workspaceID, conversationID, limit, before)
	if err != nil {
		return nil, "", err
	}
	return items, encodeSequenceCursor(next), nil
}

func (u *UseCase) Run(ctx context.Context, input entities.RunInput, emit func(Event) error) (err error) {
	if !validRun(input) {
		return domainerrors.ErrInvalidInput
	}
	digest := sha256.Sum256(input.Snapshot)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), input.SnapshotSHA256) {
		return domainerrors.ErrInvalidInput
	}
	generator, ok := u.generators.Resolve(input.Model.Adapter)
	if !ok {
		return domainerrors.ErrInvalidInput
	}
	run, err := u.repository.StartRun(ctx, input)
	if err != nil {
		return err
	}
	fail := func(status, code, message string) {
		_ = u.repository.FailRun(context.WithoutCancel(ctx), run.ID, status, code, message)
	}
	if err := emit(Event{Type: EventStarted, RunID: run.ID, CreatedAt: time.Now().UTC()}); err != nil {
		fail("cancelled", "stream_cancelled", err.Error())
		return err
	}
	chunks, err := generator.Generate(ctx, input.Prompt, input.Model)
	if err != nil {
		fail("failed", "generation_failed", err.Error())
		_ = emit(Event{Type: EventFailed, RunID: run.ID, ErrorCode: "generation_failed", ErrorMessage: err.Error(), CreatedAt: time.Now().UTC()})
		return err
	}
	var content strings.Builder
	for _, chunk := range chunks {
		content.WriteString(chunk)
		if err := emit(Event{Type: EventContentDelta, RunID: run.ID, Delta: chunk, CreatedAt: time.Now().UTC()}); err != nil {
			fail("cancelled", "stream_cancelled", err.Error())
			return err
		}
	}
	message, err := u.repository.CompleteRun(ctx, run.ID, input.ConversationID, content.String())
	if err != nil {
		fail("failed", "persistence_failed", err.Error())
		return err
	}
	return emit(Event{Type: EventCompleted, RunID: run.ID, MessageID: message.ID, CreatedAt: time.Now().UTC()})
}

func validateCreate(actor entities.Actor, workspace entities.Workspace, model entities.ModelSnapshot) error {
	if strings.TrimSpace(actor.ID) == "" || strings.TrimSpace(workspace.ID) == "" || !validModel(model) {
		return domainerrors.ErrInvalidInput
	}
	return nil
}

func validModel(model entities.ModelSnapshot) bool {
	if !validUUID(model.ProfileID) || !validUUID(model.ConnectionID) || strings.TrimSpace(model.ProviderModelID) == "" || strings.TrimSpace(model.DisplayName) == "" {
		return false
	}
	return model.Adapter == "anthropic" || model.Adapter == "ollama"
}

func validRun(input entities.RunInput) bool {
	return validUUID(input.RequestID) && validUUID(input.ConversationID) && strings.TrimSpace(input.Prompt) != "" &&
		strings.TrimSpace(input.Actor.ID) != "" && strings.TrimSpace(input.Workspace.ID) != "" && validModel(input.Model) &&
		strings.TrimSpace(input.Generation) != "" && strings.TrimSpace(input.SnapshotSHA256) != ""
}

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func encodeTimeCursor(value *time.Time) string {
	if value == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value.UTC().Format(time.RFC3339Nano)))
}

func decodeTimeCursor(cursor string) (*time.Time, error) {
	if cursor == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, string(decoded))
	return &parsed, err
}

func encodeSequenceCursor(value *int64) string {
	if value == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(*value, 10)))
}

func decodeSequenceCursor(cursor string) (*int64, error) {
	if cursor == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	parsed, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
