package bundle

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	"go.uber.org/zap"
	"golang.org/x/text/unicode/norm"
)

const bundleSchemaVersion = "endge-knowledge/v1"

type manifest struct {
	SchemaVersion   string `json:"schemaVersion"`
	BundleID        string `json:"bundleId"`
	DocumentsFile   string `json:"documentsFile"`
	DocumentsSHA256 string `json:"documentsSha256"`
}

type chunk struct {
	ID           string `json:"id"`
	DocumentPath string `json:"documentPath"`
	SourceFile   string `json:"sourceFile"`
	Title        string `json:"title"`
	Heading      string `json:"heading"`
	Ordinal      int    `json:"ordinal"`
	Content      string `json:"content"`
	searchText   string
	searchTokens map[string]struct{}
}

type Retriever struct {
	bundleID string
	chunks   []chunk
	loadErr  string
	limit    int
}

var _ ports.KnowledgeRetriever = (*Retriever)(nil)

func NewRetriever(cfg *config.Config, logger *zap.Logger) ports.KnowledgeRetriever {
	retriever := &Retriever{limit: cfg.Knowledge.MaxResults}
	if cfg.Knowledge.BundlePath == "" {
		retriever.loadErr = "AI_KNOWLEDGE_BUNDLE_PATH is not configured"
		return retriever
	}
	if err := retriever.load(cfg.Knowledge.BundlePath); err != nil {
		retriever.loadErr = err.Error()
		logger.Warn("knowledge bundle is unavailable", zap.Error(err), zap.String("path", cfg.Knowledge.BundlePath))
		return retriever
	}
	logger.Info("knowledge bundle loaded", zap.String("bundle_id", retriever.bundleID), zap.Int("chunks", len(retriever.chunks)))
	return retriever
}

func (r *Retriever) Retrieve(_ context.Context, prompt string, limit int) entities.KnowledgeRetrieval {
	query := buildQuery(prompt)
	result := entities.KnowledgeRetrieval{
		Available: r.loadErr == "" && len(r.chunks) > 0,
		BundleID:  r.bundleID,
		Query:     query,
		Error:     r.loadErr,
	}
	if !result.Available {
		return result
	}

	type scoredMatch struct {
		match   entities.KnowledgeMatch
		ordinal int
	}
	matches := make([]scoredMatch, 0)
	for _, candidate := range r.chunks {
		score, matchedTerms := scoreChunk(candidate, query)
		if score == 0 {
			continue
		}
		matches = append(matches, scoredMatch{
			match: entities.KnowledgeMatch{
				ChunkID:      candidate.ID,
				DocumentPath: candidate.DocumentPath,
				SourceFile:   candidate.SourceFile,
				Title:        candidate.Title,
				Heading:      candidate.Heading,
				Content:      candidate.Content,
				Score:        score,
				MatchedTerms: matchedTerms,
			},
			ordinal: candidate.Ordinal,
		})
	}

	sort.SliceStable(matches, func(left, right int) bool {
		if matches[left].match.Score != matches[right].match.Score {
			return matches[left].match.Score > matches[right].match.Score
		}
		if matches[left].match.DocumentPath != matches[right].match.DocumentPath {
			return matches[left].match.DocumentPath < matches[right].match.DocumentPath
		}
		return matches[left].ordinal < matches[right].ordinal
	})
	if limit < 1 {
		limit = r.limit
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	result.Matches = make([]entities.KnowledgeMatch, 0, len(matches))
	for _, match := range matches {
		result.Matches = append(result.Matches, match.match)
	}
	if len(result.Matches) > 0 {
		result.BestScore = result.Matches[0].Score
		result.Coverage = queryCoverage(r.chunks, result.Matches[0].ChunkID, query)
	}
	return result
}

func (r *Retriever) load(bundlePath string) error {
	stat, err := os.Stat(bundlePath)
	if err != nil {
		return fmt.Errorf("stat knowledge bundle: %w", err)
	}
	bundleDirectory := bundlePath
	manifestPath := filepath.Join(bundleDirectory, "manifest.json")
	if !stat.IsDir() {
		bundleDirectory = filepath.Dir(bundlePath)
		manifestPath = filepath.Join(bundleDirectory, "manifest.json")
	}

	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read knowledge manifest: %w", err)
	}
	var bundleManifest manifest
	if err := json.Unmarshal(manifestBytes, &bundleManifest); err != nil {
		return fmt.Errorf("decode knowledge manifest: %w", err)
	}
	if bundleManifest.SchemaVersion != bundleSchemaVersion {
		return fmt.Errorf("unsupported knowledge schema %q", bundleManifest.SchemaVersion)
	}
	if strings.TrimSpace(bundleManifest.BundleID) == "" || strings.TrimSpace(bundleManifest.DocumentsFile) == "" {
		return fmt.Errorf("knowledge manifest is incomplete")
	}

	documentsPath := filepath.Join(bundleDirectory, filepath.Clean(bundleManifest.DocumentsFile))
	if filepath.Dir(documentsPath) != filepath.Clean(bundleDirectory) {
		return fmt.Errorf("knowledge documents file must be inside the bundle directory")
	}
	documentsBytes, err := os.ReadFile(documentsPath)
	if err != nil {
		return fmt.Errorf("read knowledge documents: %w", err)
	}
	digest := sha256.Sum256(documentsBytes)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), bundleManifest.DocumentsSHA256) {
		return fmt.Errorf("knowledge documents checksum mismatch")
	}

	scanner := bufio.NewScanner(strings.NewReader(string(documentsBytes)))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	loaded := make([]chunk, 0)
	line := 0
	for scanner.Scan() {
		line++
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var item chunk
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return fmt.Errorf("decode knowledge chunk at line %d: %w", line, err)
		}
		if item.ID == "" || item.DocumentPath == "" || item.Content == "" {
			return fmt.Errorf("knowledge chunk at line %d is incomplete", line)
		}
		item.searchText = normalize(strings.Join([]string{item.Title, item.Heading, item.Content}, "\n"))
		item.searchTokens = tokenSet(item.searchText)
		loaded = append(loaded, item)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan knowledge documents: %w", err)
	}
	if len(loaded) == 0 {
		return fmt.Errorf("knowledge bundle contains no chunks")
	}
	r.bundleID = bundleManifest.BundleID
	r.chunks = loaded
	return nil
}

func buildQuery(prompt string) entities.KnowledgeSearchQuery {
	normalized := normalize(prompt)
	terms := meaningfulTokens(normalized)
	expanded := make([]string, 0)
	seen := make(map[string]struct{}, len(terms))
	for _, term := range terms {
		seen[term] = struct{}{}
	}
	for _, term := range terms {
		if stem := stemToken(term); stem != term {
			if _, exists := seen[stem]; !exists {
				seen[stem] = struct{}{}
				expanded = append(expanded, stem)
			}
		}
		for _, alias := range aliases[term] {
			if _, exists := seen[alias]; exists {
				continue
			}
			seen[alias] = struct{}{}
			expanded = append(expanded, alias)
		}
	}
	sort.Strings(expanded)

	phrases := make([]string, 0)
	for index := 0; index+1 < len(terms); index++ {
		phrases = append(phrases, terms[index]+" "+terms[index+1])
	}
	return entities.KnowledgeSearchQuery{
		NormalizedPrompt: normalized,
		Terms:            terms,
		ExpandedTerms:    expanded,
		Phrases:          phrases,
	}
}

func scoreChunk(candidate chunk, query entities.KnowledgeSearchQuery) (int, []string) {
	title := normalize(candidate.Title)
	heading := normalize(candidate.Heading)
	score := 0
	matched := make([]string, 0)

	for _, phrase := range query.Phrases {
		switch {
		case strings.Contains(title, phrase):
			score += 40
		case strings.Contains(heading, phrase):
			score += 25
		case strings.Contains(candidate.searchText, phrase):
			score += 10
		default:
			continue
		}
		matched = append(matched, phrase)
	}

	allTerms := slices.Concat(query.Terms, query.ExpandedTerms)
	for _, term := range allTerms {
		if concept, ok := documentedConcept(term); ok {
			switch {
			case title == concept && heading == concept:
				score += 120
			case title == concept:
				score += 70
			}
		}
		if _, exists := candidate.searchTokens[term]; !exists {
			continue
		}
		switch {
		case title == term:
			score += 30
		case strings.Contains(title, term):
			score += 24
		case strings.Contains(heading, term):
			score += 12
		default:
			score += 3
		}
		if strings.HasPrefix(term, "define") && len(term) > len("define") {
			score += 30
		}
		if isTechnicalAnchor(term) {
			// Exact API identifiers are stronger evidence than generic prose words.
			// This keeps a query such as filterView on its reference page instead of
			// letting broad expansion terms (documentation, usage, example) dominate.
			score += 50
		}
		matched = append(matched, term)
	}
	slices.Sort(matched)
	matched = slices.Compact(matched)
	return score, matched
}

func isTechnicalAnchor(term string) bool {
	if len(term) < 6 {
		return false
	}
	for _, character := range term {
		if character > unicode.MaxASCII || (!unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' && character != '-') {
			return false
		}
	}
	switch term {
	case "documentation", "example", "endge", "request", "response", "system", "usage":
		return false
	default:
		return true
	}
}

func documentedConcept(term string) (string, bool) {
	if !strings.HasPrefix(term, "define") || len(term) == len("define") {
		return "", false
	}
	return strings.TrimPrefix(term, "define"), true
}

func normalize(value string) string {
	value = norm.NFKC.String(value)
	value = strings.Map(func(character rune) rune {
		switch character {
		case 'Ё', 'ё':
			return 'е'
		default:
			return unicode.ToLower(character)
		}
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func meaningfulTokens(value string) []string {
	result := make([]string, 0)
	seen := map[string]struct{}{}
	for _, token := range tokenize(value) {
		if len([]rune(token)) < 2 {
			continue
		}
		if _, ignored := stopWords[token]; ignored {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}
	return result
}

func tokenSet(value string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, token := range tokenize(value) {
		result[token] = struct{}{}
		result[stemToken(token)] = struct{}{}
	}
	return result
}

func queryCoverage(chunks []chunk, chunkID string, query entities.KnowledgeSearchQuery) float64 {
	if len(query.Terms) == 0 {
		return 1
	}
	for _, item := range chunks {
		if item.ID != chunkID {
			continue
		}
		matched := 0
		for _, term := range query.Terms {
			if _, exists := item.searchTokens[term]; exists {
				matched++
				continue
			}
			if _, exists := item.searchTokens[stemToken(term)]; exists {
				matched++
			}
		}
		return float64(matched) / float64(len(query.Terms))
	}
	return 0
}

func stemToken(token string) string {
	characters := []rune(token)
	if len(characters) < 5 {
		return token
	}
	for _, suffix := range []string{
		"иями", "ями", "ами", "ение", "ения", "ений", "ского", "ному", "ого", "ему", "ому",
		"иях", "ах", "ях", "ию", "ью", "ия", "ья", "ии", "ий", "ый", "ой", "ая", "яя", "ое", "ее",
		"ов", "ев", "ей", "ам", "ям", "ом", "ем", "ы", "и", "а", "я", "у", "ю", "е",
	} {
		if strings.HasSuffix(token, suffix) && len([]rune(token))-len([]rune(suffix)) >= 4 {
			return string(characters[:len(characters)-len([]rune(suffix))])
		}
	}
	for _, suffix := range []string{"ing", "ed", "es", "s"} {
		if strings.HasSuffix(token, suffix) && len(token)-len(suffix) >= 4 {
			return strings.TrimSuffix(token, suffix)
		}
	}
	return token
}

func tokenize(value string) []string {
	return strings.FieldsFunc(value, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' && character != '-'
	})
}

var stopWords = map[string]struct{}{
	"а": {}, "без": {}, "бы": {}, "в": {}, "во": {}, "вот": {}, "для": {}, "до": {}, "его": {}, "если": {},
	"же": {}, "и": {}, "из": {}, "или": {}, "как": {}, "к": {}, "ли": {}, "мне": {}, "мы": {}, "на": {},
	"не": {}, "но": {}, "о": {}, "он": {}, "она": {}, "они": {}, "от": {}, "по": {}, "при": {}, "с": {},
	"со": {}, "так": {}, "то": {}, "у": {}, "что": {}, "это": {}, "этот": {}, "я": {}, "объясни": {},
	"покажи": {}, "найди": {}, "перечисли": {}, "расскажи": {},
	"a": {}, "an": {}, "and": {}, "for": {}, "how": {}, "in": {}, "is": {}, "of": {}, "on": {}, "or": {},
	"the": {}, "to": {}, "with": {},
}

var aliases = map[string][]string{
	"вычисление": {"computation"},
	"вычисления": {"computation"},
	"действие":   {"action"},
	"действия":   {"action"},
	"запрос":     {"query"},
	"компонент":  {"component"},
	"мок":        {"mock"},
	"обновить":   {"update"},
	"обновление": {"update"},
	"таблица":    {"table"},
	"тип":        {"type"},
	"фильтр":     {"filter"},
	"фильтра":    {"filter"},
	"фильтров":   {"filter"},
	"хранилище":  {"store"},
}
