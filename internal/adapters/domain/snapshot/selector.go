package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

const (
	workspaceSnapshotKind = "workspace-snapshot"
	supportedSchema       = 1
)

type portableSnapshot struct {
	Kind                  string                       `json:"kind"`
	SchemaVersion         int                          `json:"schemaVersion"`
	DomainVersion         string                       `json:"domainVersion"`
	Workspace             map[string]any               `json:"workspace"`
	Documents             map[string][]json.RawMessage `json:"documents"`
	InstalledIntegrations []json.RawMessage            `json:"installedIntegrations"`
}

type candidate struct {
	documentType string
	identity     string
	displayName  string
	description  string
	snapshot     json.RawMessage
	searchText   string
	searchTokens map[string]struct{}
}

type Selector struct {
	limit int
}

var _ ports.DomainContextSelector = (*Selector)(nil)

func NewSelector(cfg *config.Config) ports.DomainContextSelector {
	return &Selector{limit: cfg.Context.DomainMaxResults}
}

func (s *Selector) Select(_ context.Context, raw []byte, query entities.KnowledgeSearchQuery, limit int) entities.DomainContext {
	result := entities.DomainContext{Limit: limit}
	if result.Limit < 1 {
		result.Limit = s.limit
	}

	var source portableSnapshot
	if err := json.Unmarshal(raw, &source); err != nil {
		result.Error = fmt.Sprintf("decode workspace snapshot: %v", err)
		return result
	}
	result.Kind = source.Kind
	result.SchemaVersion = source.SchemaVersion
	result.DomainVersion = source.DomainVersion
	if source.Kind != workspaceSnapshotKind || source.SchemaVersion != supportedSchema {
		result.Error = fmt.Sprintf("unsupported workspace snapshot %q schema %d", source.Kind, source.SchemaVersion)
		return result
	}

	result.Workspace = marshalSanitized(source.Workspace)
	result.InstalledIntegrations = make([]json.RawMessage, 0, len(source.InstalledIntegrations))
	for _, integration := range source.InstalledIntegrations {
		result.InstalledIntegrations = append(result.InstalledIntegrations, sanitizeRaw(integration))
	}

	candidates := make([]candidate, 0)
	documentTypes := make([]string, 0, len(source.Documents))
	for documentType := range source.Documents {
		documentTypes = append(documentTypes, documentType)
	}
	sort.Strings(documentTypes)
	for _, documentType := range documentTypes {
		items := source.Documents[documentType]
		result.TotalDocuments += len(items)
		for _, item := range items {
			candidates = append(candidates, newCandidate(documentType, item))
		}
	}

	type scoredCandidate struct {
		candidate candidate
		score     int
		matched   []string
	}
	direct := make([]scoredCandidate, 0)
	for _, item := range candidates {
		score, matched := scoreCandidate(item, query)
		if score > 0 {
			direct = append(direct, scoredCandidate{candidate: item, score: score, matched: matched})
		}
	}
	sort.SliceStable(direct, func(left, right int) bool {
		if direct[left].score != direct[right].score {
			return direct[left].score > direct[right].score
		}
		if direct[left].candidate.documentType != direct[right].candidate.documentType {
			return direct[left].candidate.documentType < direct[right].candidate.documentType
		}
		return direct[left].candidate.identity < direct[right].candidate.identity
	})

	directKeys := make(map[string]struct{}, len(direct))
	for _, item := range direct {
		directKeys[candidateKey(item.candidate)] = struct{}{}
	}
	selected := make(map[string]struct{})
	identities := make(map[string]string)
	primaryLimit := result.Limit
	if primaryLimit > 1 {
		primaryLimit = (primaryLimit*2 + 2) / 3
	}
	for _, item := range direct {
		if len(result.Matches) >= primaryLimit {
			break
		}
		key := candidateKey(item.candidate)
		selected[key] = struct{}{}
		if reference := normalizeReference(item.candidate.identity); reference != "" {
			identities[reference] = key
		}
		result.Matches = append(result.Matches, toMatch(item.candidate, "direct", item.score, item.matched, nil))
	}

	if len(result.Matches) < result.Limit && len(identities) > 0 {
		related := make([]entities.DomainContextMatch, 0)
		for _, item := range candidates {
			if _, exists := directKeys[candidateKey(item)]; exists {
				continue
			}
			relatedTo := make([]string, 0)
			for identity, key := range identities {
				if _, exists := item.searchTokens[identity]; exists {
					relatedTo = append(relatedTo, key)
				}
			}
			if len(relatedTo) == 0 {
				continue
			}
			sort.Strings(relatedTo)
			related = append(related, toMatch(item, "related", len(relatedTo), nil, relatedTo))
		}
		sort.SliceStable(related, func(left, right int) bool {
			if related[left].Score != related[right].Score {
				return related[left].Score > related[right].Score
			}
			if related[left].DocumentType != related[right].DocumentType {
				return related[left].DocumentType < related[right].DocumentType
			}
			return related[left].Identity < related[right].Identity
		})
		remaining := result.Limit - len(result.Matches)
		if len(related) > remaining {
			related = related[:remaining]
		}
		result.Matches = append(result.Matches, related...)
	}
	if len(result.Matches) < result.Limit {
		for _, item := range direct {
			if len(result.Matches) >= result.Limit {
				break
			}
			key := candidateKey(item.candidate)
			if _, exists := selected[key]; exists {
				continue
			}
			selected[key] = struct{}{}
			result.Matches = append(result.Matches, toMatch(item.candidate, "direct", item.score, item.matched, nil))
		}
	}

	result.Available = true
	return result
}

func newCandidate(documentType string, raw json.RawMessage) candidate {
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	sanitized, ok := sanitize(value).(map[string]any)
	if !ok {
		sanitized = map[string]any{}
	}
	snapshot := marshalSanitized(sanitized)
	identity := stringValue(sanitized, "identity", "id")
	displayName := stringValue(sanitized, "displayName", "name", "title")
	description := stringValue(sanitized, "description")
	searchText := normalize(strings.Join([]string{documentType, identity, displayName, description, string(snapshot)}, "\n"))
	return candidate{
		documentType: documentType,
		identity:     identity,
		displayName:  displayName,
		description:  description,
		snapshot:     snapshot,
		searchText:   searchText,
		searchTokens: tokenSet(searchText),
	}
}

func scoreCandidate(item candidate, query entities.KnowledgeSearchQuery) (int, []string) {
	header := normalize(strings.Join([]string{item.documentType, item.identity, item.displayName, item.description}, " "))
	score := 0
	matched := make([]string, 0)
	for _, phrase := range query.Phrases {
		switch {
		case strings.Contains(header, phrase):
			score += 24
		case strings.Contains(item.searchText, phrase):
			score += 8
		default:
			continue
		}
		matched = append(matched, phrase)
	}
	for _, term := range slices.Concat(query.Terms, query.ExpandedTerms) {
		if _, exists := item.searchTokens[term]; !exists {
			continue
		}
		if strings.Contains(header, term) {
			score += 10
		} else {
			score += 2
		}
		matched = append(matched, term)
	}
	slices.Sort(matched)
	return score, slices.Compact(matched)
}

func toMatch(item candidate, kind string, score int, matched, relatedTo []string) entities.DomainContextMatch {
	return entities.DomainContextMatch{
		DocumentType: item.documentType,
		Identity:     item.identity,
		DisplayName:  item.displayName,
		MatchKind:    kind,
		Score:        score,
		MatchedTerms: matched,
		RelatedTo:    relatedTo,
		Snapshot:     item.snapshot,
	}
}

func candidateKey(item candidate) string {
	if item.identity != "" {
		return item.documentType + "/" + item.identity
	}
	return item.documentType + "/" + item.displayName
}

func stringValue(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
	}
	return ""
}

func normalizeReference(value string) string {
	value = normalize(value)
	if len([]rune(value)) < 3 || strings.Contains(value, " ") {
		return ""
	}
	return value
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func tokenSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(value, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' && character != '-' && character != '.'
	}) {
		result[token] = struct{}{}
	}
	return result
}

func sanitizeRaw(raw json.RawMessage) json.RawMessage {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return json.RawMessage(`null`)
	}
	return marshalSanitized(value)
}

func marshalSanitized(value any) json.RawMessage {
	encoded, err := json.Marshal(sanitize(value))
	if err != nil {
		return json.RawMessage(`null`)
	}
	return encoded
}

func sanitize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if sensitiveKey(key) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = sanitize(item)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, sanitize(item))
		}
		return result
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	for _, fragment := range []string{"password", "secret", "token", "credential", "authorization", "apikey", "privatekey", "accesskey"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
