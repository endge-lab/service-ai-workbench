package preparation

import (
	"encoding/json"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func allowedEntityCitations(plan entities.TaskPlan, blocks []entities.RetrievedBlock) []entities.EntityCitation {
	result := make([]entities.EntityCitation, 0)
	seen := map[string]struct{}{}
	appendCitation := func(documentType, identity string) {
		if documentType == "" || identity == "" {
			return
		}
		key := documentType + "\x00" + identity
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		result = append(result, entities.EntityCitation{DocumentType: documentType, Identity: identity})
	}
	for _, task := range plan.Tasks {
		if task.ResolvedEntity != nil {
			appendCitation(task.ResolvedEntity.DocumentType, task.ResolvedEntity.Identity)
		}
	}
	for _, block := range blocks {
		if block.SourceKind != "domain" {
			continue
		}
		var payload struct {
			DocumentType string `json:"documentType"`
			Identity     string `json:"identity"`
			Items        []struct {
				DocumentType string `json:"documentType"`
				Identity     string `json:"identity"`
			} `json:"items"`
		}
		if json.Unmarshal([]byte(block.Content), &payload) != nil {
			continue
		}
		appendCitation(payload.DocumentType, payload.Identity)
		for _, item := range payload.Items {
			appendCitation(item.DocumentType, item.Identity)
		}
	}
	return result
}
