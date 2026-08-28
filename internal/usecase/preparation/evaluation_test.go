package preparation

import (
	"slices"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func TestDeterministicRoutingEvaluation(t *testing.T) {
	tests := []struct {
		name          string
		prompt        string
		intent        entities.TaskIntent
		source        entities.SourceMode
		expectedTypes []string
		mention       string
		reference     bool
	}{
		{name: "mutation command is unsupported", prompt: "Измени композицию sample-page и сохрани её", intent: entities.IntentUnsupported, source: entities.SourceNone},
		{name: "documentation code symbol", prompt: "Объясни, как работает defineComposition и для чего он нужен", intent: entities.IntentExplainDocumentation, source: entities.SourceDocumentation, mention: "definecomposition"},
		{name: "documentation inflected entity", prompt: "Объясни синтаксис фильтров", intent: entities.IntentExplainDocumentation, source: entities.SourceDocumentation, expectedTypes: []string{"filters"}, mention: "синтаксис фильтров"},
		{name: "documentation concept", prompt: "Расскажи, что такое проекция представления", intent: entities.IntentExplainDocumentation, source: entities.SourceDocumentation, expectedTypes: []string{"data-views"}, mention: "проекция представления"},
		{name: "exact identity", prompt: "Найди композицию sample-control-page", intent: entities.IntentFindEntity, source: entities.SourceDomain, expectedTypes: []string{"compositions"}, mention: "sample-control-page"},
		{name: "display name", prompt: "Покажи композицию Тестовая страница", intent: entities.IntentInspectEntity, source: entities.SourceDomain, expectedTypes: []string{"compositions"}, mention: "тестовая страница"},
		{name: "quoted display name", prompt: "Покажи композицию «Тестовая страница»", intent: entities.IntentInspectEntity, source: entities.SourceDomain, expectedTypes: []string{"compositions"}, mention: "тестовая страница"},
		{name: "list compositions", prompt: "Какие композиции у нас есть?", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"compositions"}},
		{name: "find component", prompt: "Найди компонент sample-card", intent: entities.IntentFindEntity, source: entities.SourceDomain, expectedTypes: []string{"components"}, mention: "sample-card"},
		{name: "inspect action", prompt: "Покажи действие Отправить форму", intent: entities.IntentInspectEntity, source: entities.SourceDomain, expectedTypes: []string{"actions"}, mention: "отправить форму"},
		{name: "list filters", prompt: "Перечисли фильтры", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"filters"}},
		{name: "list views genitive", prompt: "Покажи список представлений", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"data-views"}},
		{name: "list computations genitive", prompt: "Какие вычисления есть?", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"computations"}},
		{name: "find query", prompt: "Найди запрос sample-query", intent: entities.IntentFindEntity, source: entities.SourceDomain, expectedTypes: []string{"queries"}, mention: "sample-query"},
		{name: "find store", prompt: "Найди хранилище sample-store", intent: entities.IntentFindEntity, source: entities.SourceDomain, expectedTypes: []string{"stores"}, mention: "sample-store"},
		{name: "inspect converter", prompt: "Покажи конвертер Sample converter", intent: entities.IntentInspectEntity, source: entities.SourceDomain, expectedTypes: []string{"converters"}, mention: "sample"},
		{name: "inspect type", prompt: "Покажи тип Sample type", intent: entities.IntentInspectEntity, source: entities.SourceDomain, expectedTypes: []string{"types"}, mention: "sample"},
		{name: "list styles genitive", prompt: "Перечисли стили", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"styles"}},
		{name: "list navigations", prompt: "Какие навигации у нас есть?", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"navigations"}},
		{name: "list environments", prompt: "Перечисли окружения", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"environments"}},
		{name: "list projects", prompt: "Какие проекты у нас есть?", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"projects"}},
		{name: "documentation query", prompt: "Как использовать query в Endge?", intent: entities.IntentExplainDocumentation, source: entities.SourceDocumentation, expectedTypes: []string{"queries"}, mention: "query endge"},
		{name: "documentation store", prompt: "Что такое хранилище?", intent: entities.IntentExplainDocumentation, source: entities.SourceDocumentation, expectedTypes: []string{"stores"}, mention: "хранилище"},
		{name: "documentation computation", prompt: "Объясни возможности вычислений", intent: entities.IntentExplainDocumentation, source: entities.SourceDocumentation, expectedTypes: []string{"computations"}, mention: "возможности вычислений"},
		{name: "english documentation", prompt: "How to use defineQuery", intent: entities.IntentExplainDocumentation, source: entities.SourceDocumentation, mention: "definequery"},
		{name: "english find", prompt: "Find composition sample-page", intent: entities.IntentFindEntity, source: entities.SourceDomain, expectedTypes: []string{"compositions"}, mention: "sample-page"},
		{name: "english list", prompt: "List components", intent: entities.IntentListEntities, source: entities.SourceDomain, expectedTypes: []string{"components"}},
		{name: "explicit mixed", prompt: "Объясни композицию в рабочем пространстве", intent: entities.IntentExplainDocumentation, source: entities.SourceMixed, expectedTypes: []string{"compositions"}, mention: "композицию рабочем пространстве"},
		{name: "real reference", prompt: "Покажи предыдущую композицию", intent: entities.IntentInspectEntity, source: entities.SourceDomain, expectedTypes: []string{"compositions"}, mention: "предыдущую", reference: true},
	}

	correct := 0
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized := (Normalizer{}).Normalize(test.prompt)
			plan := deterministicPlan(normalized)
			if len(plan.Tasks) != 1 {
				t.Fatalf("tasks = %d, want 1: %#v", len(plan.Tasks), plan)
			}
			task := plan.Tasks[0]
			mention := ""
			if len(task.Mentions) > 0 {
				mention = task.Mentions[0]
			}
			if task.Intent != test.intent || task.SourceMode != test.source || !slices.Equal(task.ExpectedTypes, test.expectedTypes) || mention != test.mention || (len(normalized.ReferenceTokens) > 0) != test.reference {
				t.Fatalf("unexpected route: task=%#v normalized=%#v", task, normalized)
			}
			correct++
		})
	}
	if accuracy := float64(correct) / float64(len(tests)); accuracy < 0.95 {
		t.Fatalf("routing accuracy = %.2f, want >= 0.95", accuracy)
	}
}

func TestReferenceDetectionUsesWordBoundaries(t *testing.T) {
	for _, prompt := range []string{
		"Объясни синтаксис",
		"Покажи деталь",
		"Что умеет конфигуратор",
		"Объясни, как работает defineComposition и для чего он нужен",
		"Покажи композицию и расскажи, для чего она нужна",
	} {
		if references := (Normalizer{}).Normalize(prompt).ReferenceTokens; len(references) != 0 {
			t.Fatalf("%q produced false history references: %v", prompt, references)
		}
	}
}

func TestFolderScopeCreatesOrderedTasks(t *testing.T) {
	plan := deterministicPlan((Normalizer{}).Normalize("Перечисли композиции в папке «Примеры»"))
	if len(plan.Tasks) != 2 || plan.Tasks[0].ExpectedTypes[0] != "folders" || plan.Tasks[0].Mentions[0] != "примеры" {
		t.Fatalf("folder task was not planned first: %#v", plan)
	}
	child := plan.Tasks[1]
	if child.Intent != entities.IntentListEntities || !slices.Equal(child.ExpectedTypes, []string{"compositions"}) || len(child.DependsOn) != 1 || child.DependsOn[0] != plan.Tasks[0].ID {
		t.Fatalf("folder child task is invalid: %#v", child)
	}
}
