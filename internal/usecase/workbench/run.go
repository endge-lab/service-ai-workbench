package workbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/modelcalls"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	preparationusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/preparation"
)

func (u *UseCase) Run(ctx context.Context, input entities.RunInput, emit func(Event) error) (err error) {
	if !validRun(input) {
		return domainerrors.ErrInvalidInput
	}
	ctx = modelcalls.WithBudget(ctx, u.maxPreparationModelCalls)
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
	startedAt := time.Now().UTC()
	if u.interaction == nil || u.preparation == nil || u.responseValidator == nil || u.interactions == nil {
		_ = u.repository.FailRun(context.WithoutCancel(ctx), run.ID, "failed", "preparation_unavailable", "preparation pipeline is unavailable")
		return errors.New("preparation pipeline is unavailable")
	}
	interaction, err := u.interaction.StartOrResume(ctx, input, run)
	if err != nil {
		_ = u.repository.FailRun(context.WithoutCancel(ctx), run.ID, "failed", "interaction_failed", err.Error())
		return err
	}
	fail := func(status, code, message string) {
		if interaction.ID != "" {
			_ = u.interactions.FailInteraction(context.WithoutCancel(ctx), run.ID, interaction.ID, status, code, message)
			return
		}
		_ = u.repository.FailRun(context.WithoutCancel(ctx), run.ID, status, code, message)
	}
	if err := emit(Event{Type: EventStarted, RunID: run.ID, InteractionID: interaction.ID, CreatedAt: startedAt}); err != nil {
		fail("cancelled", "stream_cancelled", err.Error())
		return err
	}
	conversationContext := entities.ConversationContext{
		Limit:          u.contextMessageLimit,
		BeforeSequence: run.UserMessageSequence,
		Messages:       []entities.Message{},
	}
	previousMessages, _, contextErr := u.repository.ListMessages(
		ctx,
		input.Actor.ID,
		input.Workspace.ID,
		input.ConversationID,
		u.contextMessageLimit,
		&run.UserMessageSequence,
	)
	if contextErr != nil {
		conversationContext.Error = contextErr.Error()
	} else {
		conversationContext.Messages = previousMessages
	}
	if interaction.Status == entities.InteractionCancelled {
		message, completeErr := u.repository.CompleteRun(ctx, run.ID, input.ConversationID, "Запрос отменён.")
		if completeErr != nil {
			return completeErr
		}
		_ = emit(Event{Type: EventContentDelta, RunID: run.ID, InteractionID: interaction.ID, Delta: "Запрос отменён.", CreatedAt: time.Now().UTC()})
		return emit(Event{Type: EventCompleted, RunID: run.ID, InteractionID: interaction.ID, MessageID: message.ID, CreatedAt: time.Now().UTC()})
	}
	prepared, retrieval, domainContext, err := u.preparation.Prepare(ctx, preparationusecase.Input{RunInput: input, Interaction: interaction, Conversation: conversationContext})
	if err != nil {
		fail("failed", "preparation_failed", err.Error())
		_ = emit(Event{Type: EventFailed, RunID: run.ID, InteractionID: interaction.ID, ErrorCode: "preparation_failed", ErrorMessage: "AI request preparation failed", CreatedAt: time.Now().UTC()})
		return err
	}
	interaction.Plan = prepared.Plan
	interaction.WorkspaceGeneration = input.Generation
	interaction.WorkspaceSnapshotSHA256 = input.SnapshotSHA256
	interaction.DocumentationVersion = retrieval.BundleID
	if prepared.Status == entities.PreparationNeedsClarification {
		clarification, _, clarificationErr := u.interactions.CreateClarification(ctx, run.ID, interaction, *prepared.Clarification)
		if clarificationErr != nil {
			fail("failed", "clarification_persistence_failed", clarificationErr.Error())
			return clarificationErr
		}
		u.recordDebug(ctx, input, run, interaction, &clarification, startedAt, retrieval, domainContext, conversationContext, prepared, entities.ResponseValidation{})
		return emit(Event{Type: EventClarificationRequired, RunID: run.ID, InteractionID: interaction.ID, Clarification: &clarification, CreatedAt: time.Now().UTC()})
	}
	interaction.Status = entities.InteractionReady
	interaction, err = u.interactions.SaveInteraction(ctx, interaction, interaction.PlanVersion)
	if err != nil {
		fail("failed", "interaction_persistence_failed", err.Error())
		return err
	}
	if prepared.Status == entities.PreparationUnsupported {
		u.recordDebug(ctx, input, run, interaction, nil, startedAt, retrieval, domainContext, conversationContext, prepared, entities.ResponseValidation{})
		if emitErr := emit(Event{Type: EventContentDelta, RunID: run.ID, InteractionID: interaction.ID, Delta: prepared.UnsupportedMessage, CreatedAt: time.Now().UTC()}); emitErr != nil {
			fail("cancelled", "stream_cancelled", "client stream closed")
			return emitErr
		}
		message, completeErr := u.interactions.CompleteInteractionRun(ctx, run.ID, input.ConversationID, interaction.ID, prepared.UnsupportedMessage)
		if completeErr != nil {
			fail("failed", "persistence_failed", completeErr.Error())
			return completeErr
		}
		return emit(Event{Type: EventCompleted, RunID: run.ID, InteractionID: interaction.ID, MessageID: message.ID, CreatedAt: time.Now().UTC()})
	}
	interaction.Status = entities.InteractionGenerating
	interaction, err = u.interactions.SaveInteraction(ctx, interaction, interaction.PlanVersion)
	if err != nil {
		fail("failed", "interaction_persistence_failed", err.Error())
		return err
	}
	bufferedContent, err := generateBuffered(ctx, generator, entities.GenerationRequest{
		ModelRequest:   *prepared.ModelRequest,
		ProviderAccess: input.ProviderAccess,
	}, u.responseMaxBytes)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fail("cancelled", "generation_cancelled", "generation was cancelled")
			return err
		}
		message := "AI provider request failed"
		code := "generation_failed"
		if errors.Is(err, errResponseBufferLimit) {
			message = "AI provider response is too large"
			code = "response_too_large"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			message = "AI provider request timed out"
			code = "generation_timeout"
		}
		fail("failed", code, message)
		u.recordDebug(ctx, input, run, interaction, nil, startedAt, retrieval, domainContext, conversationContext, prepared, entities.ResponseValidation{})
		_ = emit(Event{Type: EventFailed, RunID: run.ID, InteractionID: interaction.ID, ErrorCode: code, ErrorMessage: message, CreatedAt: time.Now().UTC()})
		return err
	}
	validation, repairUsage, err := u.responseValidator.Validate(ctx, input, prepared, []byte(bufferedContent))
	prepared.Trace.PromptUsage = append(prepared.Trace.PromptUsage, repairUsage...)
	if err != nil || !validation.Valid {
		fail("failed", "response_validation_failed", "AI response validation failed")
		u.recordDebug(ctx, input, run, interaction, nil, startedAt, retrieval, domainContext, conversationContext, prepared, validation)
		_ = emit(Event{Type: EventFailed, RunID: run.ID, InteractionID: interaction.ID, ErrorCode: "response_validation_failed", ErrorMessage: "AI response could not be verified", CreatedAt: time.Now().UTC()})
		if err != nil {
			return err
		}
		return errors.New("AI response validation failed")
	}
	validatedContent := preparationusecase.RenderValidatedAnswer(validation.Response)
	for _, chunk := range splitResponse(validatedContent, 512) {
		if emitErr := emit(Event{Type: EventContentDelta, RunID: run.ID, InteractionID: interaction.ID, Delta: chunk, CreatedAt: time.Now().UTC()}); emitErr != nil {
			fail("cancelled", "stream_cancelled", "client stream closed")
			return emitErr
		}
	}
	message, err := u.interactions.CompleteInteractionRun(ctx, run.ID, input.ConversationID, interaction.ID, validatedContent)
	if err != nil {
		fail("failed", "persistence_failed", err.Error())
		return err
	}
	u.recordDebug(ctx, input, run, interaction, nil, startedAt, retrieval, domainContext, conversationContext, prepared, validation)
	return emit(Event{Type: EventCompleted, RunID: run.ID, InteractionID: interaction.ID, MessageID: message.ID, CreatedAt: time.Now().UTC()})
}

func generateBuffered(ctx context.Context, generator ports.Generator, request entities.GenerationRequest, maxBytes int) (string, error) {
	var content strings.Builder
	err := generator.Generate(ctx, request, func(chunk string) error {
		if chunk == "" {
			return nil
		}
		if content.Len()+len(chunk) > maxBytes {
			return errResponseBufferLimit
		}
		content.WriteString(chunk)
		return nil
	})
	if err != nil {
		return "", err
	}
	return content.String(), nil
}

func (u *UseCase) recordDebug(ctx context.Context, input entities.RunInput, run entities.Run, interaction entities.Interaction, clarification *entities.Clarification, startedAt time.Time, knowledge entities.KnowledgeRetrieval, domain entities.DomainContext, conversation entities.ConversationContext, prepared entities.PreparationResult, response entities.ResponseValidation) {
	if u.debug == nil {
		return
	}
	modelRequest := entities.ModelRequest{}
	if prepared.ModelRequest != nil {
		modelRequest = *prepared.ModelRequest
	}
	prepared.Trace.ModelCalls = modelcalls.Used(ctx)
	prepared.Trace.PromptUsage = mergePromptUsage(prepared.Trace.PromptUsage, modelcalls.PromptUsage(ctx))
	u.debug.Record(ctx, entities.RunDebugRecord{
		RequestID: input.RequestID, RunID: run.ID, ConversationID: input.ConversationID, ActorID: input.Actor.ID,
		WorkspaceID: input.Workspace.ID, Prompt: input.Prompt, Generation: input.Generation,
		HeadRevisionID: input.HeadRevisionID, SnapshotSHA256: input.SnapshotSHA256, StartedAt: startedAt,
		Knowledge: knowledge, Domain: domain, Conversation: conversation, ModelRequest: modelRequest,
		Interaction: interaction, Clarification: clarification, Preparation: prepared.Trace, Response: response,
	})
}

func mergePromptUsage(current, additional []entities.PromptUsage) []entities.PromptUsage {
	seen := make(map[string]struct{}, len(current)+len(additional))
	result := make([]entities.PromptUsage, 0, len(current)+len(additional))
	for _, usage := range append(append([]entities.PromptUsage(nil), current...), additional...) {
		key := string(usage.ID) + "\x00" + usage.SHA256
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, usage)
	}
	return result
}

func splitResponse(value string, size int) []string {
	characters := []rune(value)
	result := make([]string, 0, (len(characters)+size-1)/size)
	for len(characters) > 0 {
		end := size
		if len(characters) < end {
			end = len(characters)
		}
		result = append(result, string(characters[:end]))
		characters = characters[end:]
	}
	return result
}
