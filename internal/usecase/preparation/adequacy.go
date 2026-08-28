package preparation

import (
	"encoding/json"
	"sort"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type ContextAdequacyValidator struct{ maxChars int }

func NewContextAdequacyValidator(maxChars int) ContextAdequacyValidator {
	return ContextAdequacyValidator{maxChars: maxChars}
}

func (v ContextAdequacyValidator) Validate(prompt string, plan entities.TaskPlan, blocks []entities.RetrievedBlock, reservedChars int) ([]entities.RetrievedBlock, *entities.Clarification) {
	if clarification := validateAdequacy(prompt, plan, blocks); clarification != nil {
		return nil, clarification
	}
	available := v.maxChars - reservedChars
	if available < 0 {
		available = 0
	}
	fitted := fitBlocks(blocks, available)
	if !allMandatoryBlocksFit(blocks, fitted) {
		return nil, &entities.Clarification{TaskID: plan.Tasks[0].ID, Slot: "context_budget", Question: localized(prompt, "Обязательный контекст слишком велик. Уточните, какую часть нужно рассмотреть.", "The mandatory context is too large. Please narrow the part to inspect.")}
	}
	return fitted, nil
}

func fitBlocks(blocks []entities.RetrievedBlock, maxChars int) []entities.RetrievedBlock {
	ordered := append([]entities.RetrievedBlock(nil), blocks...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Mandatory != ordered[j].Mandatory {
			return ordered[i].Mandatory
		}
		return ordered[i].Score > ordered[j].Score
	})
	used := 0
	result := make([]entities.RetrievedBlock, 0, len(ordered))
	for _, block := range ordered {
		encoded, err := json.Marshal(block)
		if err != nil {
			continue
		}
		cost := len(encoded)
		if len(result) > 0 {
			cost++
		}
		if used+cost > maxChars {
			continue
		}
		used += cost
		result = append(result, block)
	}
	return result
}

func allMandatoryBlocksFit(all, fitted []entities.RetrievedBlock) bool {
	included := make(map[string]struct{}, len(fitted))
	for _, block := range fitted {
		included[block.SourceKind+"\x00"+block.SourceKey] = struct{}{}
	}
	for _, block := range all {
		if !block.Mandatory {
			continue
		}
		if _, exists := included[block.SourceKind+"\x00"+block.SourceKey]; !exists {
			return false
		}
	}
	return true
}
