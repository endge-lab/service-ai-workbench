package files

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	"go.uber.org/zap"
)

type Recorder struct {
	enabled    bool
	outputPath string
	logger     *zap.Logger
}

var _ ports.RunDebugRecorder = (*Recorder)(nil)

func NewRecorder(cfg *config.Config, logger *zap.Logger) ports.RunDebugRecorder {
	return &Recorder{
		enabled:    cfg.Debug.Enabled,
		outputPath: cfg.Debug.OutputPath,
		logger:     logger,
	}
}

func (r *Recorder) Record(_ context.Context, record entities.RunDebugRecord) {
	if !r.enabled {
		return
	}
	if err := r.write(record); err != nil {
		r.logger.Warn("write AI debug record", zap.Error(err), zap.String("request_id", record.RequestID))
	}
}

func (r *Recorder) write(record entities.RunDebugRecord) error {
	timestamp := record.StartedAt.UTC().Format("20060102T150405.000000000Z")
	directory := filepath.Join(r.outputPath, record.ConversationID, timestamp+"_"+record.RequestID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create debug directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("protect debug directory: %w", err)
	}

	metadata := struct {
		CreatedAt       time.Time `json:"created_at"`
		RequestID       string    `json:"request_id"`
		RunID           string    `json:"run_id"`
		ConversationID  string    `json:"conversation_id"`
		ActorID         string    `json:"actor_id"`
		WorkspaceID     string    `json:"workspace_id"`
		Generation      string    `json:"workspace_generation"`
		HeadRevisionID  string    `json:"workspace_head_revision_id,omitempty"`
		SnapshotSHA256  string    `json:"workspace_snapshot_sha256"`
		KnowledgeBundle string    `json:"knowledge_bundle_id,omitempty"`
	}{
		CreatedAt:       record.StartedAt.UTC(),
		RequestID:       record.RequestID,
		RunID:           record.RunID,
		ConversationID:  record.ConversationID,
		ActorID:         record.ActorID,
		WorkspaceID:     record.WorkspaceID,
		Generation:      record.Generation,
		HeadRevisionID:  record.HeadRevisionID,
		SnapshotSHA256:  record.SnapshotSHA256,
		KnowledgeBundle: record.Knowledge.BundleID,
	}
	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode debug metadata: %w", err)
	}
	if err := writeProtectedFile(directory, "00-метаданные.json", append(metadataJSON, '\n')); err != nil {
		return err
	}
	if err := writeProtectedFile(directory, "01-промпт.txt", []byte(record.Prompt+"\n")); err != nil {
		return err
	}

	query := struct {
		NormalizedPrompt string   `json:"нормализованный_промпт"`
		Terms            []string `json:"термины"`
		ExpandedTerms    []string `json:"добавленные_термины"`
		Phrases          []string `json:"поисковые_фразы"`
	}{
		NormalizedPrompt: record.Knowledge.Query.NormalizedPrompt,
		Terms:            record.Knowledge.Query.Terms,
		ExpandedTerms:    record.Knowledge.Query.ExpandedTerms,
		Phrases:          record.Knowledge.Query.Phrases,
	}
	queryJSON, err := json.MarshalIndent(query, "", "  ")
	if err != nil {
		return fmt.Errorf("encode debug query: %w", err)
	}
	if err := writeProtectedFile(directory, "02-поисковые-выражения.json", append(queryJSON, '\n')); err != nil {
		return err
	}

	documentation := renderDocumentation(record.Knowledge)
	if err := writeProtectedFile(directory, "03-извлеченная-документация.md", []byte(documentation)); err != nil {
		return err
	}
	domainJSON, err := json.MarshalIndent(renderDomainContext(record.Domain), "", "  ")
	if err != nil {
		return fmt.Errorf("encode domain context: %w", err)
	}
	if err := writeProtectedFile(directory, "04-контекст-домена.json", append(domainJSON, '\n')); err != nil {
		return err
	}
	conversationJSON, err := json.MarshalIndent(renderConversationContext(record.Conversation), "", "  ")
	if err != nil {
		return fmt.Errorf("encode conversation context: %w", err)
	}
	if err := writeProtectedFile(directory, "05-контекст-диалога.json", append(conversationJSON, '\n')); err != nil {
		return err
	}
	contextPlanJSON, err := json.MarshalIndent(renderContextPlan(record.ContextPlan), "", "  ")
	if err != nil {
		return fmt.Errorf("encode context plan: %w", err)
	}
	if err := writeProtectedFile(directory, "06-план-контекста.json", append(contextPlanJSON, '\n')); err != nil {
		return err
	}
	modelRequestJSON, err := json.MarshalIndent(renderModelRequest(record.ModelRequest), "", "  ")
	if err != nil {
		return fmt.Errorf("encode model request: %w", err)
	}
	if err := writeProtectedFile(directory, "07-запрос-к-модели.json", append(modelRequestJSON, '\n')); err != nil {
		return err
	}
	return nil
}

type contextPlanDebug struct {
	Intent struct {
		Kind    string   `json:"тип"`
		Signals []string `json:"сигналы"`
	} `json:"интент"`
	SourcePriority []string                        `json:"приоритет_источников"`
	Budget         contextBudgetDebug              `json:"бюджет"`
	Sections       []contextSectionBudgetDebug     `json:"секции"`
	Decisions      []contextDecisionDebug          `json:"решения"`
}

type contextBudgetDebug struct {
	MaxChars             int `json:"максимум_символов"`
	SystemChars          int `json:"системный_промпт"`
	CurrentPromptChars   int `json:"текущий_запрос"`
	ContextEnvelopeChars int `json:"резерв_структуры"`
	HistoryChars         int `json:"история"`
	DocumentationChars   int `json:"документация"`
	DomainChars          int `json:"домен"`
	TotalChars           int `json:"итого_символов"`
	EstimatedTokens      int `json:"примерно_токенов"`
}

type contextSectionBudgetDebug struct {
	Name           string `json:"секция"`
	Priority       int    `json:"порядок_распределения"`
	BudgetChars    int    `json:"бюджет_символов"`
	UsedChars      int    `json:"использовано_символов"`
	CandidateCount int    `json:"кандидатов"`
	IncludedCount  int    `json:"включено"`
}

type contextDecisionDebug struct {
	Source string `json:"источник"`
	ID     string `json:"id"`
	Score  int    `json:"score,omitempty"`
	Chars  int    `json:"символов"`
	Status string `json:"статус"`
	Reason string `json:"причина"`
}

func renderContextPlan(plan entities.ContextPlan) contextPlanDebug {
	result := contextPlanDebug{
		SourcePriority: plan.SourcePriority,
		Budget: contextBudgetDebug{
			MaxChars:             plan.Budget.MaxChars,
			SystemChars:          plan.Budget.SystemChars,
			CurrentPromptChars:   plan.Budget.CurrentPromptChars,
			ContextEnvelopeChars: plan.Budget.ContextEnvelopeChars,
			HistoryChars:         plan.Budget.HistoryChars,
			DocumentationChars:   plan.Budget.DocumentationChars,
			DomainChars:          plan.Budget.DomainChars,
			TotalChars:           plan.Budget.TotalChars,
			EstimatedTokens:      plan.Budget.EstimatedTokens,
		},
		Sections:  make([]contextSectionBudgetDebug, 0, len(plan.Sections)),
		Decisions: make([]contextDecisionDebug, 0, len(plan.Decisions)),
	}
	result.Intent.Kind = plan.Intent.Kind
	result.Intent.Signals = plan.Intent.Signals
	for _, section := range plan.Sections {
		result.Sections = append(result.Sections, contextSectionBudgetDebug{
			Name:           section.Name,
			Priority:       section.Priority,
			BudgetChars:    section.BudgetChars,
			UsedChars:      section.UsedChars,
			CandidateCount: section.CandidateCount,
			IncludedCount:  section.IncludedCount,
		})
	}
	for _, decision := range plan.Decisions {
		result.Decisions = append(result.Decisions, contextDecisionDebug{
			Source: decision.Source,
			ID:     decision.ID,
			Score:  decision.Score,
			Chars:  decision.Chars,
			Status: decision.Status,
			Reason: decision.Reason,
		})
	}
	return result
}

type modelRequestDebug struct {
	Model struct {
		Adapter         string `json:"adapter"`
		ProviderModelID string `json:"provider_model_id"`
		DisplayName     string `json:"display_name"`
	} `json:"model"`
	System   string                  `json:"system"`
	Messages []entities.ModelMessage `json:"messages"`
}

func renderModelRequest(request entities.ModelRequest) modelRequestDebug {
	result := modelRequestDebug{
		System:   request.SystemPrompt,
		Messages: request.Messages,
	}
	result.Model.Adapter = request.Model.Adapter
	result.Model.ProviderModelID = request.Model.ProviderModelID
	result.Model.DisplayName = request.Model.DisplayName
	return result
}

type domainDebugMatch struct {
	DocumentType string          `json:"тип_документа"`
	Identity     string          `json:"identity,omitempty"`
	DisplayName  string          `json:"название,omitempty"`
	MatchKind    string          `json:"тип_совпадения"`
	Score        int             `json:"score"`
	MatchedTerms []string        `json:"совпавшие_термины,omitempty"`
	RelatedTo    []string        `json:"связан_с,omitempty"`
	Snapshot     json.RawMessage `json:"снимок"`
}

type domainDebugContext struct {
	Available             bool               `json:"доступен"`
	Kind                  string             `json:"тип_снимка,omitempty"`
	SchemaVersion         int                `json:"версия_схемы,omitempty"`
	DomainVersion         string             `json:"версия_домена,omitempty"`
	TotalDocuments        int                `json:"всего_документов"`
	Limit                 int                `json:"лимит"`
	Workspace             json.RawMessage    `json:"рабочее_пространство,omitempty"`
	InstalledIntegrations []json.RawMessage  `json:"установленные_интеграции"`
	Matches               []domainDebugMatch `json:"выбранные_документы"`
	Error                 string             `json:"ошибка,omitempty"`
}

func renderDomainContext(context entities.DomainContext) domainDebugContext {
	matches := make([]domainDebugMatch, 0, len(context.Matches))
	for _, match := range context.Matches {
		matches = append(matches, domainDebugMatch{
			DocumentType: match.DocumentType,
			Identity:     match.Identity,
			DisplayName:  match.DisplayName,
			MatchKind:    match.MatchKind,
			Score:        match.Score,
			MatchedTerms: match.MatchedTerms,
			RelatedTo:    match.RelatedTo,
			Snapshot:     match.Snapshot,
		})
	}
	integrations := context.InstalledIntegrations
	if integrations == nil {
		integrations = []json.RawMessage{}
	}
	return domainDebugContext{
		Available:             context.Available,
		Kind:                  context.Kind,
		SchemaVersion:         context.SchemaVersion,
		DomainVersion:         context.DomainVersion,
		TotalDocuments:        context.TotalDocuments,
		Limit:                 context.Limit,
		Workspace:             context.Workspace,
		InstalledIntegrations: integrations,
		Matches:               matches,
		Error:                 context.Error,
	}
}

type conversationDebugMessage struct {
	Sequence  int64     `json:"sequence"`
	Role      string    `json:"роль"`
	Content   string    `json:"сообщение"`
	CreatedAt time.Time `json:"создано"`
}

type conversationDebugContext struct {
	Selection               string                     `json:"отбор"`
	Limit                   int                        `json:"лимит"`
	BeforeSequence          int64                      `json:"до_sequence"`
	CurrentPromptSeparately bool                       `json:"текущий_промпт_добавляется_отдельно"`
	Messages                []conversationDebugMessage `json:"предыдущие_сообщения"`
	Error                   string                     `json:"ошибка,omitempty"`
}

func renderConversationContext(context entities.ConversationContext) conversationDebugContext {
	messages := make([]conversationDebugMessage, 0, len(context.Messages))
	for _, message := range context.Messages {
		messages = append(messages, conversationDebugMessage{
			Sequence:  message.Sequence,
			Role:      message.Role,
			Content:   message.Content,
			CreatedAt: message.CreatedAt.UTC(),
		})
	}
	return conversationDebugContext{
		Selection:               "latest_before_current_prompt",
		Limit:                   context.Limit,
		BeforeSequence:          context.BeforeSequence,
		CurrentPromptSeparately: true,
		Messages:                messages,
		Error:                   context.Error,
	}
}

func renderDocumentation(retrieval entities.KnowledgeRetrieval) string {
	var output strings.Builder
	output.WriteString("# Извлечённая документация\n\n")
	if !retrieval.Available {
		output.WriteString("Документация недоступна.\n")
		if retrieval.Error != "" {
			output.WriteString("\nПричина: `")
			output.WriteString(strings.ReplaceAll(retrieval.Error, "`", "'"))
			output.WriteString("`\n")
		}
		return output.String()
	}
	output.WriteString("Bundle: `")
	output.WriteString(retrieval.BundleID)
	output.WriteString("`\n\n")
	if len(retrieval.Matches) == 0 {
		output.WriteString("Подходящие фрагменты не найдены.\n")
		return output.String()
	}

	for index, match := range retrieval.Matches {
		fmt.Fprintf(&output, "## %d. %s\n\n", index+1, match.Title)
		fmt.Fprintf(&output, "- Источник: `%s`\n", match.DocumentPath)
		fmt.Fprintf(&output, "- Раздел: %s\n", match.Heading)
		fmt.Fprintf(&output, "- Score: %d\n", match.Score)
		fmt.Fprintf(&output, "- Совпадения: `%s`\n\n", strings.Join(match.MatchedTerms, "`, `"))
		output.WriteString(match.Content)
		output.WriteString("\n\n")
	}
	return output.String()
}

func writeProtectedFile(directory, name string, content []byte) error {
	target := filepath.Join(directory, name)
	temporary := filepath.Join(directory, "."+name+".tmp")
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		return fmt.Errorf("write debug file %s: %w", name, err)
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("protect debug file %s: %w", name, err)
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish debug file %s: %w", name, err)
	}
	return nil
}
