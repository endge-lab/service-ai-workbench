package entities

import (
	"encoding/json"
	"testing"
)

func TestStructuredSchemasAreValidJSONObjects(t *testing.T) {
	schemas := map[string]json.RawMessage{
		"final-answer":             FinalAnswerSchema,
		"task-plan":                TaskPlanSchema,
		"query-expansion":          QueryExpansionSchema,
		"reranker":                 RerankerSchema,
		"clarification-classifier": ClarificationClassifierSchema,
	}
	for name, schema := range schemas {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if !json.Valid(schema) || json.Unmarshal(schema, &value) != nil || value["type"] != "object" {
				t.Fatalf("schema is not a JSON object: %s", schema)
			}
		})
	}
}
