package preparation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

// Resolver is the only component allowed to turn domain candidates into a
// resolved entity or a closed clarification candidate set.
type Resolver struct {
	prompts            ports.PromptCatalog
	models             ports.StructuredModelInvoker
	maxCandidates      int
	rerankerConfidence float64
}

func NewResolver(prompts ports.PromptCatalog, models ports.StructuredModelInvoker, maxCandidates int, rerankerConfidence float64) Resolver {
	return Resolver{prompts: prompts, models: models, maxCandidates: maxCandidates, rerankerConfidence: rerankerConfidence}
}

func (r Resolver) Resolve(ctx context.Context, input entities.RunInput, task *entities.PlannedTask, domain entities.DomainContext, budget *modelCallBudget, trace *entities.PreparationTrace) (*entities.Clarification, error) {
	if task.Intent == entities.IntentListEntities {
		task.Status = "resolved"
		return nil, nil
	}
	mention := normalizeText(strings.Join(task.Mentions, " "))
	if task.ResolvedEntity != nil {
		mention = normalizeText(task.ResolvedEntity.Identity)
	}
	matches := filterExpectedTypes(domain.Matches, task.ExpectedTypes)
	exact := exactMatches(matches, mention)
	if len(exact) == 1 {
		setResolved(task, exact[0], input.SnapshotSHA256)
		return nil, nil
	}
	if len(exact) > 1 {
		return clarificationFor(task, exact, localized(input.Prompt, "Найдено несколько точных совпадений. Что вы имели в виду?", "Several exact matches were found. Which one did you mean?"), r.maxCandidates), nil
	}
	if len(matches) == 0 {
		return clarificationFor(task, nil, localized(input.Prompt, "Не удалось однозначно найти сущность. Уточните название или identity.", "The entity could not be resolved. Please clarify its name or identity."), r.maxCandidates), nil
	}
	if len(matches) > r.maxCandidates {
		matches = matches[:r.maxCandidates]
	}
	payload, err := json.Marshal(map[string]any{"mention": mention, "candidates": publicCandidates(matches)})
	if err != nil {
		return nil, err
	}
	raw, usages, err := invokeStructured(ctx, r.prompts, r.models, input, entities.PromptRerankerSystem, entities.PromptRerankerRequest, payload, entities.RerankerSchema, budget)
	trace.PromptUsage = append(trace.PromptUsage, usages...)
	trace.ModelCalls = budget.used
	if err == nil {
		var ranked struct {
			SelectedCandidateID   string  `json:"selectedCandidateId"`
			Confidence            float64 `json:"confidence"`
			RequiresClarification bool    `json:"requiresClarification"`
		}
		if json.Unmarshal(extractJSONObject(raw), &ranked) == nil && !ranked.RequiresClarification && ranked.Confidence >= r.rerankerConfidence {
			for _, match := range matches {
				if candidateID(match) == ranked.SelectedCandidateID {
					setResolved(task, match, input.SnapshotSHA256)
					return nil, nil
				}
			}
		}
	}
	return clarificationFor(task, matches, localized(input.Prompt, "Уточните, о какой сущности идёт речь.", "Please clarify which entity you mean."), r.maxCandidates), nil
}
