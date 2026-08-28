package interaction

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/modelcalls"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type Coordinator struct {
	repository ports.InteractionRepository
	prompts    ports.PromptCatalog
	models     ports.StructuredModelInvoker
}

func NewCoordinator(repository ports.InteractionRepository, prompts ports.PromptCatalog, models ports.StructuredModelInvoker) *Coordinator {
	return &Coordinator{repository: repository, prompts: prompts, models: models}
}

func (c *Coordinator) StartOrResume(ctx context.Context, input entities.RunInput, run entities.Run) (entities.Interaction, error) {
	if strings.TrimSpace(input.ReplyToClarificationID) == "" {
		return c.startNew(ctx, input, run)
	}
	if strings.TrimSpace(input.InteractionID) == "" {
		return entities.Interaction{}, domainerrors.ErrConflict
	}
	current, err := c.repository.GetInteraction(ctx, input.Actor.ID, input.Workspace.ID, input.ConversationID, input.InteractionID)
	if err != nil || current == nil {
		if err != nil {
			return entities.Interaction{}, err
		}
		return entities.Interaction{}, domainerrors.ErrConflict
	}
	clarification, err := c.repository.GetOpenClarification(ctx, input.Actor.ID, input.Workspace.ID, input.ConversationID)
	if err != nil {
		return entities.Interaction{}, err
	}
	if clarification == nil || clarification.ID != input.ReplyToClarificationID || clarification.InteractionID != current.ID {
		return entities.Interaction{}, domainerrors.ErrConflict
	}
	if input.SelectedCandidateID == "" {
		if selected, ok := candidateFromFreeText(input.Prompt, clarification.Candidates); ok {
			input.SelectedCandidateID = selected
		}
	}

	kind, err := c.classify(ctx, input, *clarification)
	if err != nil {
		return entities.Interaction{}, err
	}
	if kind == "new_request" {
		if err := c.repository.SupersedeInteraction(ctx, current.ID); err != nil {
			return entities.Interaction{}, err
		}
		return c.createAndAttach(ctx, input, run)
	}
	if kind == "cancel" {
		current.Status = entities.InteractionCancelled
		updated, err := c.repository.ApplyClarification(ctx, entities.ClarificationAnswer{
			InteractionID: current.ID, ClarificationID: clarification.ID, UserMessageID: run.UserMessageID,
			BasePlanVersion: clarification.PlanVersion, Status: "cancelled",
		}, *current, current.PlanVersion)
		if err != nil {
			return entities.Interaction{}, err
		}
		if err := c.repository.AttachRunInteraction(ctx, run.ID, updated.ID); err != nil {
			return entities.Interaction{}, err
		}
		return updated, nil
	}

	applyClarification(&current.Plan, *clarification, input)
	current.Status = entities.InteractionResolving
	updated, err := c.repository.ApplyClarification(ctx, entities.ClarificationAnswer{
		InteractionID: current.ID, ClarificationID: clarification.ID, SelectedCandidateID: input.SelectedCandidateID,
		Text: input.Prompt, UserMessageID: run.UserMessageID, BasePlanVersion: clarification.PlanVersion, Status: "answered",
	}, *current, current.PlanVersion)
	if err != nil {
		return entities.Interaction{}, err
	}
	if err := c.repository.AttachRunInteraction(ctx, run.ID, updated.ID); err != nil {
		return entities.Interaction{}, err
	}
	return updated, nil
}

func candidateFromFreeText(answer string, candidates []entities.ClarificationCandidate) (string, bool) {
	normalizedAnswer := normalizeCandidateText(answer)
	if normalizedAnswer == "" {
		return "", false
	}
	selected := ""
	for _, candidate := range candidates {
		matched := false
		for _, value := range []string{candidate.Identity, candidate.DisplayName} {
			normalizedValue := normalizeCandidateText(value)
			if len([]rune(normalizedValue)) >= 4 && (normalizedAnswer == normalizedValue || strings.Contains(normalizedAnswer, normalizedValue)) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if selected != "" && selected != candidate.CandidateID {
			return "", false
		}
		selected = candidate.CandidateID
	}
	return selected, selected != ""
}

func normalizeCandidateText(value string) string {
	value = strings.Map(func(character rune) rune {
		switch character {
		case 'Ё', 'ё':
			return 'е'
		case '-', '_':
			return ' '
		default:
			if unicode.IsLetter(character) || unicode.IsDigit(character) {
				return unicode.ToLower(character)
			}
			return ' '
		}
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func (c *Coordinator) startNew(ctx context.Context, input entities.RunInput, run entities.Run) (entities.Interaction, error) {
	active, err := c.repository.GetActiveInteraction(ctx, input.Actor.ID, input.Workspace.ID, input.ConversationID)
	if err != nil {
		return entities.Interaction{}, err
	}
	if active != nil {
		if err := c.repository.SupersedeInteraction(ctx, active.ID); err != nil {
			return entities.Interaction{}, err
		}
	}
	return c.createAndAttach(ctx, input, run)
}

func (c *Coordinator) createAndAttach(ctx context.Context, input entities.RunInput, run entities.Run) (entities.Interaction, error) {
	interaction, err := c.repository.CreateInteraction(ctx, run.ID, input.ConversationID, run.UserMessageID, input.Generation, input.SnapshotSHA256, "")
	if err != nil {
		return entities.Interaction{}, err
	}
	return interaction, nil
}

func (c *Coordinator) classify(ctx context.Context, input entities.RunInput, clarification entities.Clarification) (string, error) {
	if input.SelectedCandidateID != "" {
		for _, candidate := range clarification.Candidates {
			if candidate.CandidateID == input.SelectedCandidateID {
				return "answer", nil
			}
		}
		return "", domainerrors.ErrConflict
	}
	normalized := strings.ToLower(strings.TrimSpace(input.Prompt))
	if normalized == "отмена" || normalized == "cancel" || normalized == "отмени" {
		return "cancel", nil
	}
	if strings.HasPrefix(normalized, "новый вопрос") || strings.HasPrefix(normalized, "другой вопрос") {
		return "new_request", nil
	}
	payload, _ := json.Marshal(map[string]any{"question": clarification.Question, "candidates": clarification.Candidates, "answer": input.Prompt})
	system, err := c.prompts.Render(ctx, entities.PromptClarificationClassifierSystem, nil)
	if err != nil {
		return "", err
	}
	request, err := c.prompts.Render(ctx, entities.PromptClarificationClassifierRequest, map[string]string{"PayloadJSON": string(payload)})
	if err != nil {
		return "", err
	}
	modelcalls.RecordPromptUsage(ctx,
		entities.PromptUsage{ID: system.ID, Version: system.Version, SHA256: system.SHA256},
		entities.PromptUsage{ID: request.ID, Version: request.Version, SHA256: request.SHA256},
	)
	if err := modelcalls.Consume(ctx); err != nil {
		return "unclear", err
	}
	raw, err := c.models.Invoke(ctx, ports.StructuredModelRequest{Model: input.Model, ProviderAccess: input.ProviderAccess, SystemPrompt: system.Content, UserPrompt: request.Content, ResponseFormat: entities.ClarificationClassifierSchema})
	if err != nil {
		return "answer", nil
	}
	var result struct {
		Kind       string  `json:"kind"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal(extractJSON(raw), &result); err != nil || result.Confidence < 0.8 {
		return "answer", nil
	}
	switch result.Kind {
	case "answer", "correction", "new_request", "cancel", "unclear":
		return result.Kind, nil
	default:
		return "answer", nil
	}
}

func applyClarification(plan *entities.TaskPlan, clarification entities.Clarification, input entities.RunInput) {
	changedTaskID := ""
	for index := range plan.Tasks {
		task := &plan.Tasks[index]
		if task.ID != clarification.TaskID {
			continue
		}
		changedTaskID = task.ID
		if input.SelectedCandidateID != "" {
			for _, candidate := range clarification.Candidates {
				if candidate.CandidateID == input.SelectedCandidateID {
					task.ResolvedEntity = &entities.ResolvedEntity{
						DocumentType: candidate.DocumentType,
						Identity:     candidate.Identity,
						DisplayName:  candidate.DisplayName,
						Snapshot:     candidate.Snapshot,
					}
					task.Status = "resolved"
					task.Candidates = nil
					task.UnresolvedSlot = ""
					invalidateDependents(plan, changedTaskID)
					return
				}
			}
		}
		task.Mentions = []string{input.Prompt}
		task.Status = "planned"
		task.Candidates = nil
		task.UnresolvedSlot = ""
		invalidateDependents(plan, changedTaskID)
		return
	}
}

func invalidateDependents(plan *entities.TaskPlan, changedTaskID string) {
	invalidated := map[string]bool{changedTaskID: true}
	for changed := true; changed; {
		changed = false
		for index := range plan.Tasks {
			task := &plan.Tasks[index]
			if invalidated[task.ID] {
				continue
			}
			for _, dependency := range task.DependsOn {
				if !invalidated[dependency] {
					continue
				}
				invalidated[task.ID] = true
				task.Status = "planned"
				task.ResolvedEntity = nil
				task.Candidates = nil
				task.UnresolvedSlot = ""
				changed = true
				break
			}
		}
	}
}

func extractJSON(raw []byte) []byte {
	value := strings.TrimSpace(string(raw))
	if start := strings.Index(value, "{"); start >= 0 {
		if end := strings.LastIndex(value, "}"); end >= start {
			return []byte(value[start : end+1])
		}
	}
	return raw
}
