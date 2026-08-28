package preparation

import (
	"encoding/json"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func TestFinalAnswerSchemaClosesCitationSets(t *testing.T) {
	plan := entities.TaskPlan{Tasks: []entities.PlannedTask{{
		ID: "task-1",
		ResolvedEntity: &entities.ResolvedEntity{
			DocumentType: "compositions",
			Identity:     "sample-page",
		},
	}}}
	blocks := []entities.RetrievedBlock{{SourceKind: "documentation", SourceKey: "reference/composition"}}
	raw, err := finalAnswerSchema(plan, blocks)
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	entityItems := properties["entityCitations"].(map[string]any)["items"].(map[string]any)
	entityOption := entityItems["oneOf"].([]any)[0].(map[string]any)
	entityProperties := entityOption["properties"].(map[string]any)
	if entityProperties["documentType"].(map[string]any)["const"] != "compositions" || entityProperties["identity"].(map[string]any)["const"] != "sample-page" {
		t.Fatalf("entity citation is not closed over resolved context: %s", raw)
	}
	docItems := properties["documentationCitations"].(map[string]any)["items"].(map[string]any)
	if docItems["enum"].([]any)[0] != "reference/composition" {
		t.Fatalf("documentation citations are not closed over selected blocks: %s", raw)
	}
}

func TestFinalAnswerSchemaRequiresEmptyCitationsWithoutContext(t *testing.T) {
	raw, err := finalAnswerSchema(entities.TaskPlan{}, nil)
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	for _, name := range []string{"entityCitations", "documentationCitations"} {
		if properties[name].(map[string]any)["maxItems"] != float64(0) {
			t.Fatalf("%s must be empty without context: %s", name, raw)
		}
	}
}

func TestListDomainBlockClosesSchemaOverReturnedItems(t *testing.T) {
	blocks := []entities.RetrievedBlock{{
		SourceKind: "domain",
		SourceKey:  "list/task-1",
		Content:    `{"items":[{"documentType":"compositions","identity":"sample-one"},{"documentType":"compositions","identity":"sample-two"}]}`,
	}}
	raw, err := finalAnswerSchema(entities.TaskPlan{}, blocks)
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	properties := schema["properties"].(map[string]any)
	entityItems := properties["entityCitations"].(map[string]any)["items"].(map[string]any)
	if len(entityItems["oneOf"].([]any)) != 2 {
		t.Fatalf("list items were not exposed as closed citation candidates: %s", raw)
	}
}
