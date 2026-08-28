package preparation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type Planner struct {
	prompts ports.PromptCatalog
	models  ports.StructuredModelInvoker
}

func NewPlanner(prompts ports.PromptCatalog, models ports.StructuredModelInvoker) Planner {
	return Planner{prompts: prompts, models: models}
}

func (p Planner) Plan(ctx context.Context, normalized entities.NormalizedRequest, history []entities.Message, input entities.RunInput, budget *modelCallBudget, trace *entities.PreparationTrace) (entities.TaskPlan, error) {
	deterministic := deterministicPlan(normalized)
	if !needsPlanner(normalized, deterministic) {
		return deterministic, validatePlan(deterministic)
	}

	payload, err := json.Marshal(map[string]any{
		"request": normalized,
		"history": minimalHistory(history, 4),
		"registry": map[string]any{
			"intents":     []entities.TaskIntent{entities.IntentExplainDocumentation, entities.IntentFindEntity, entities.IntentInspectEntity, entities.IntentListEntities},
			"sourceModes": []entities.SourceMode{entities.SourceDocumentation, entities.SourceDomain, entities.SourceMixed, entities.SourceConversation, entities.SourceNone},
		},
	})
	if err != nil {
		return entities.TaskPlan{}, fmt.Errorf("encode planner input: %w", err)
	}
	raw, usages, err := invokeStructured(ctx, p.prompts, p.models, input, entities.PromptPlannerSystem, entities.PromptPlannerRequest, payload, budget)
	trace.PromptUsage = append(trace.PromptUsage, usages...)
	trace.ModelCalls = budget.used
	if err != nil {
		return deterministic, validatePlan(deterministic)
	}
	var planned entities.TaskPlan
	if err := json.Unmarshal(extractJSONObject(raw), &planned); err == nil {
		if err := validatePlan(planned); err == nil {
			return planned, nil
		}
	}
	repaired, repairUsage, repairErr := repairStructured(ctx, p.prompts, p.models, input, raw, "TaskPlan with an acyclic tasks array", budget)
	trace.PromptUsage = append(trace.PromptUsage, repairUsage...)
	trace.ModelCalls = budget.used
	if repairErr == nil && json.Unmarshal(extractJSONObject(repaired), &planned) == nil && validatePlan(planned) == nil {
		return planned, nil
	}
	return deterministic, validatePlan(deterministic)
}

func deterministicPlan(request entities.NormalizedRequest) entities.TaskPlan {
	text := request.NormalizedText
	parts := splitAtomicRequests(text)
	tasks := make([]entities.PlannedTask, 0, len(parts)+1)
	for _, part := range parts {
		intent, source := classifyIntent(part)
		mention := selectMention(request, part)
		task := entities.PlannedTask{
			ID:         fmt.Sprintf("task-%d", len(tasks)+1),
			Intent:     intent,
			SourceMode: source,
			Confidence: 0.9,
			Status:     "planned",
		}
		if mention != "" {
			task.Mentions = []string{mention}
		}
		if containsAny(part, "папк", "folder") {
			folderTaskID := fmt.Sprintf("task-%d", len(tasks)+1)
			tasks = append(tasks, entities.PlannedTask{
				ID: folderTaskID, Intent: entities.IntentFindEntity, SourceMode: entities.SourceDomain,
				Mentions: []string{mention}, ExpectedTypes: []string{"folders"}, Confidence: 0.85, Status: "planned",
			})
			task.ID = fmt.Sprintf("task-%d", len(tasks)+1)
			task.FolderMention = mention
			task.DependsOn = []string{folderTaskID}
		}
		tasks = append(tasks, task)
	}
	if len(tasks) == 0 {
		tasks = append(tasks, entities.PlannedTask{ID: "task-1", Intent: entities.IntentUnsupported, SourceMode: entities.SourceNone, Confidence: 1, Status: "unsupported"})
	}
	return entities.TaskPlan{Tasks: tasks}
}

func classifyIntent(text string) (entities.TaskIntent, entities.SourceMode) {
	documentation := containsAny(text, "документац", "синтаксис", "возможност", "как ", "documentation", "syntax", "how ")
	domain := containsAny(text, "домен", "workspace", "рабоч", "композиц", "сущност", "папк", "entity", "composition", "folder")
	mode := entities.SourceNone
	switch {
	case documentation && domain:
		mode = entities.SourceMixed
	case documentation:
		mode = entities.SourceDocumentation
	case domain:
		mode = entities.SourceDomain
	}
	switch {
	case containsAny(text, "объясни", "что такое", "как ", "explain", "how ") && documentation:
		return entities.IntentExplainDocumentation, mode
	case containsAny(text, "список", "перечисли", "какие", "list"):
		return entities.IntentListEntities, entities.SourceDomain
	case containsAny(text, "найди", "найти", "где", "find"):
		return entities.IntentFindEntity, entities.SourceDomain
	case containsAny(text, "покажи", "расскажи про", "проверь", "inspect", "show"):
		return entities.IntentInspectEntity, entities.SourceDomain
	case documentation:
		return entities.IntentExplainDocumentation, mode
	case domain:
		return entities.IntentInspectEntity, mode
	default:
		return entities.IntentUnsupported, entities.SourceNone
	}
}

func validatePlan(plan entities.TaskPlan) error {
	if len(plan.Tasks) == 0 {
		return fmt.Errorf("plan contains no tasks")
	}
	known := make(map[string]entities.PlannedTask, len(plan.Tasks))
	positions := make(map[string]int, len(plan.Tasks))
	for index, task := range plan.Tasks {
		if strings.TrimSpace(task.ID) == "" {
			return fmt.Errorf("task id is empty")
		}
		if _, exists := known[task.ID]; exists {
			return fmt.Errorf("duplicate task id %q", task.ID)
		}
		if !validIntent(task.Intent) || !validSource(task.SourceMode) {
			return fmt.Errorf("unsupported task contract")
		}
		known[task.ID] = task
		positions[task.ID] = index
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("task graph contains a cycle")
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range known[id].DependsOn {
			if _, exists := known[dependency]; !exists {
				return fmt.Errorf("unknown dependency %q", dependency)
			}
			if positions[dependency] >= positions[id] {
				return fmt.Errorf("dependency %q must precede task %q", dependency, id)
			}
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range known {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validIntent(intent entities.TaskIntent) bool {
	switch intent {
	case entities.IntentExplainDocumentation, entities.IntentFindEntity, entities.IntentInspectEntity, entities.IntentListEntities, entities.IntentUnsupported:
		return true
	default:
		return false
	}
}

func validSource(source entities.SourceMode) bool {
	switch source {
	case entities.SourceDocumentation, entities.SourceDomain, entities.SourceMixed, entities.SourceConversation, entities.SourceNone:
		return true
	default:
		return false
	}
}

func needsPlanner(request entities.NormalizedRequest, plan entities.TaskPlan) bool {
	return len(plan.Tasks) > 1 || len(request.ReferenceTokens) > 0 || strings.Contains(request.NormalizedText, " затем ") || strings.Contains(request.NormalizedText, " после этого ")
}

func splitAtomicRequests(text string) []string {
	text = strings.NewReplacer(" затем ", ". ", " после этого ", ". ", ";", ".").Replace(text)
	result := make([]string, 0)
	for _, part := range strings.Split(text, ".") {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func selectMention(request entities.NormalizedRequest, part string) string {
	for _, mention := range request.QuotedMentions {
		if strings.Contains(part, mention) {
			return mention
		}
	}
	for _, token := range request.IdentityLikeTokens {
		if strings.Contains(part, token) {
			return token
		}
	}
	return strings.TrimSpace(part)
}

func minimalHistory(messages []entities.Message, limit int) []entities.ModelMessage {
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	result := make([]entities.ModelMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, entities.ModelMessage{Role: message.Role, Content: message.Content})
	}
	return result
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
