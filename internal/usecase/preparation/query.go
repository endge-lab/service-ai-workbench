package preparation

import (
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

// domainSearchQuery builds a transport-neutral lexical query without touching
// the documentation retriever. Source routing therefore remains real: a
// domain-only task never accesses the knowledge bundle.
func domainSearchQuery(value string) entities.KnowledgeSearchQuery {
	normalized := normalizeText(value)
	terms := strings.Fields(normalized)
	phrases := make([]string, 0, len(terms))
	for index := 0; index+1 < len(terms); index++ {
		phrases = append(phrases, terms[index]+" "+terms[index+1])
	}
	return entities.KnowledgeSearchQuery{NormalizedPrompt: normalized, Terms: terms, Phrases: phrases}
}
