package preparation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
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
	raw, usages, err := invokeStructured(ctx, p.prompts, p.models, input, entities.PromptPlannerSystem, entities.PromptPlannerRequest, payload, entities.TaskPlanSchema, budget)
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
	repaired, repairUsage, repairErr := repairStructured(ctx, p.prompts, p.models, input, raw, entities.TaskPlanSchema, []string{"invalid task plan"}, budget)
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
		expectedTypes := expectedEntityTypes(part)
		mention := selectMention(request, part, intent, expectedTypes)
		if intent == entities.IntentUnsupported {
			expectedTypes = nil
			mention = ""
		}
		task := entities.PlannedTask{
			ID:            fmt.Sprintf("task-%d", len(tasks)+1),
			Intent:        intent,
			SourceMode:    source,
			ExpectedTypes: expectedTypes,
			Confidence:    0.95,
			Status:        "planned",
		}
		if mention != "" {
			task.Mentions = []string{mention}
		}
		if slices.Contains(expectedTypes, "folders") && len(expectedTypes) > 1 {
			folderMention := selectFolderMention(request, part)
			folderTaskID := fmt.Sprintf("task-%d", len(tasks)+1)
			tasks = append(tasks, entities.PlannedTask{
				ID: folderTaskID, Intent: entities.IntentFindEntity, SourceMode: entities.SourceDomain,
				Mentions: []string{folderMention}, ExpectedTypes: []string{"folders"}, Confidence: 0.95, Status: "planned",
			})
			task.ID = fmt.Sprintf("task-%d", len(tasks)+1)
			task.FolderMention = folderMention
			task.DependsOn = []string{folderTaskID}
			task.ExpectedTypes = withoutType(expectedTypes, "folders")
			if task.Intent == entities.IntentListEntities {
				task.Mentions = nil
			}
		}
		tasks = append(tasks, task)
	}
	if len(tasks) == 0 {
		tasks = append(tasks, entities.PlannedTask{ID: "task-1", Intent: entities.IntentUnsupported, SourceMode: entities.SourceNone, Confidence: 1, Status: "unsupported"})
	}
	return entities.TaskPlan{Tasks: tasks}
}

func classifyIntent(text string) (entities.TaskIntent, entities.SourceMode) {
	tokens := lexicalTokens(normalizeText(text))
	if containsMutationCommand(tokens) && !containsAnyPhrase(tokens,
		"как", "как изменить", "как создать", "как удалить", "объясни", "документация", "документации", "синтаксис",
		"how", "how to", "explain", "documentation", "syntax") {
		return entities.IntentUnsupported, entities.SourceNone
	}
	types := expectedEntityTypes(text)
	hasTypes := len(types) > 0
	documentation := containsAnyPhrase(tokens,
		"документация", "документации", "синтаксис", "возможности", "пример", "как работает", "как использовать",
		"что такое", "documentation", "syntax", "how does", "how to", "example")
	explicitDomain := containsAnyPhrase(tokens, "домен", "домене", "workspace", "рабочее пространство", "рабочем пространстве", "у нас")
	list := containsAnyPhrase(tokens, "список", "перечисли", "какие", "list")
	find := containsAnyPhrase(tokens, "найди", "найти", "где", "find")
	inspect := containsAnyPhrase(tokens, "покажи", "показать", "расскажи про", "проверь", "inspect", "show")
	explain := containsAnyPhrase(tokens, "объясни", "объяснить", "что такое", "как работает", "как использовать", "explain", "how")
	switch {
	case list && hasTypes:
		return entities.IntentListEntities, entities.SourceDomain
	case find && hasTypes:
		return entities.IntentFindEntity, entities.SourceDomain
	case inspect && hasTypes:
		return entities.IntentInspectEntity, entities.SourceDomain
	case explain && explicitDomain && hasTypes:
		return entities.IntentExplainDocumentation, entities.SourceMixed
	case explain || documentation:
		return entities.IntentExplainDocumentation, entities.SourceDocumentation
	case explicitDomain && hasTypes:
		return entities.IntentInspectEntity, entities.SourceDomain
	case hasTypes:
		return entities.IntentInspectEntity, entities.SourceDomain
	default:
		return entities.IntentUnsupported, entities.SourceNone
	}
}

func containsMutationCommand(tokens []string) bool {
	commands := map[string]struct{}{
		"добавь": {}, "измени": {}, "изменить": {}, "примени": {}, "создай": {}, "сохрани": {}, "удали": {},
		"add": {}, "apply": {}, "create": {}, "delete": {}, "edit": {}, "save": {}, "update": {},
	}
	for _, token := range tokens {
		if _, exists := commands[token]; exists {
			return true
		}
	}
	return false
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
		for _, expectedType := range task.ExpectedTypes {
			if !knownEntityType(expectedType) {
				return fmt.Errorf("unsupported entity type %q", expectedType)
			}
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

func selectMention(request entities.NormalizedRequest, part string, intent entities.TaskIntent, expectedTypes []string) string {
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
	tokens := lexicalTokens(part)
	if intent == entities.IntentExplainDocumentation {
		for _, token := range tokens {
			if strings.HasPrefix(token, "define") && len(token) > len("define") {
				return token
			}
		}
	}
	ignored := commandAndFillerTokens()
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if _, skip := ignored[token]; skip {
			continue
		}
		if intent != entities.IntentExplainDocumentation && isEntityAliasToken(token, expectedTypes) {
			continue
		}
		result = append(result, token)
	}
	return strings.TrimSpace(strings.Join(result, " "))
}

func selectFolderMention(request entities.NormalizedRequest, part string) string {
	for _, mention := range request.QuotedMentions {
		if strings.Contains(part, mention) {
			return mention
		}
	}
	tokens := lexicalTokens(part)
	for index, token := range tokens {
		if token != "папка" && token != "папке" && token != "папки" && token != "folder" {
			continue
		}
		if index+1 < len(tokens) {
			return strings.Join(tokens[index+1:], " ")
		}
	}
	return ""
}

func commandAndFillerTokens() map[string]struct{} {
	values := []string{
		"найди", "найти", "покажи", "показать", "объясни", "объяснить", "перечисли", "список", "какие",
		"расскажи", "про", "проверь", "что", "такое", "как", "работает", "использовать", "мне", "пожалуйста",
		"у", "нас", "есть", "в", "во", "из", "на", "find", "show", "explain", "list", "inspect", "how", "does", "to",
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func containsAnyPhrase(tokens []string, phrases ...string) bool {
	for _, phrase := range phrases {
		if containsTokenSequence(tokens, strings.Fields(phrase)) {
			return true
		}
	}
	return false
}

func withoutType(values []string, excluded string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != excluded {
			result = append(result, value)
		}
	}
	return result
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
