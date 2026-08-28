package preparation

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"golang.org/x/text/unicode/norm"
)

var (
	quotedMentionPattern = regexp.MustCompile(`["«“]([^"»”]+)["»”]`)
	identityPattern      = regexp.MustCompile(`(?i)\b(?:[a-z][a-z0-9_.]*[-_:][a-z0-9_.:-]+|[0-9a-f]{8}-[0-9a-f-]{27,})\b`)
)

type Normalizer struct{}

func (Normalizer) Normalize(text string) entities.NormalizedRequest {
	canonical := normalizeText(text)
	result := entities.NormalizedRequest{
		OriginalText:       text,
		NormalizedText:     canonical,
		QuotedMentions:     uniqueMatches(quotedMentionPattern, text, true),
		IdentityLikeTokens: uniqueMatches(identityPattern, canonical, false),
		CommandTokens:      collectSignals(canonical, commandSignals),
		ReferenceTokens:    collectSignals(canonical, referenceSignals),
	}
	return result
}

func normalizeText(value string) string {
	value = norm.NFKC.String(value)
	value = strings.Map(func(character rune) rune {
		switch character {
		case 'Ё', 'ё':
			return 'е'
		case '\u00a0':
			return ' '
		default:
			return unicode.ToLower(character)
		}
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func uniqueMatches(pattern *regexp.Regexp, value string, subgroup bool) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, match := range pattern.FindAllStringSubmatch(value, -1) {
		index := 0
		if subgroup && len(match) > 1 {
			index = 1
		}
		normalized := normalizeText(match[index])
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	slices.Sort(result)
	return result
}

func collectSignals(value string, registry []string) []string {
	result := make([]string, 0)
	for _, signal := range registry {
		if strings.Contains(value, signal) {
			result = append(result, signal)
		}
	}
	return result
}

var commandSignals = []string{
	"найди", "найти", "покажи", "показать", "объясни", "объяснить", "перечисли", "список",
	"find", "show", "explain", "list", "inspect",
}

var referenceSignals = []string{
	"этот", "эта", "это", "тот", "та", "выше", "предыдущ", "последн", "он ", "она ",
	"this", "that", "previous", "above", "last",
}
