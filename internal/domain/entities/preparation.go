package entities

import "encoding/json"

type TaskIntent string

const (
	IntentExplainDocumentation TaskIntent = "explain_documentation"
	IntentFindEntity           TaskIntent = "find_entity"
	IntentInspectEntity        TaskIntent = "inspect_entity"
	IntentListEntities         TaskIntent = "list_entities"
	IntentUnsupported          TaskIntent = "unsupported"
)

type SourceMode string

const (
	SourceDocumentation SourceMode = "documentation"
	SourceDomain        SourceMode = "domain"
	SourceMixed         SourceMode = "mixed"
	SourceConversation  SourceMode = "conversation"
	SourceNone          SourceMode = "none"
)

type NormalizedRequest struct {
	OriginalText       string   `json:"originalText"`
	NormalizedText     string   `json:"normalizedText"`
	QuotedMentions     []string `json:"quotedMentions"`
	IdentityLikeTokens []string `json:"identityLikeTokens"`
	CommandTokens      []string `json:"commandTokens"`
	ReferenceTokens    []string `json:"referenceTokens"`
}

type ResolvedEntity struct {
	DocumentType string          `json:"documentType"`
	Identity     string          `json:"identity"`
	DisplayName  string          `json:"displayName"`
	Snapshot     json.RawMessage `json:"snapshot"`
	SnapshotHash string          `json:"snapshotSha256"`
}

type PlannedTask struct {
	ID               string                   `json:"id"`
	Intent           TaskIntent               `json:"intent"`
	SourceMode       SourceMode               `json:"sourceMode"`
	Mentions         []string                 `json:"mentions"`
	ExpectedTypes    []string                 `json:"expectedTypes"`
	RequestedAspects []string                 `json:"requestedAspects"`
	DependsOn        []string                 `json:"dependsOn"`
	FolderMention    string                   `json:"folderMention,omitempty"`
	Confidence       float64                  `json:"confidence"`
	Status           string                   `json:"status"`
	ResolvedEntity   *ResolvedEntity          `json:"resolvedEntity,omitempty"`
	Candidates       []ClarificationCandidate `json:"candidates,omitempty"`
	UnresolvedSlot   string                   `json:"unresolvedSlot,omitempty"`
}

type TaskPlan struct {
	Tasks []PlannedTask `json:"tasks"`
}

type RetrievedBlock struct {
	SourceKind string   `json:"sourceKind"`
	SourceKey  string   `json:"sourceKey"`
	TaskIDs    []string `json:"taskIds"`
	Score      float64  `json:"score"`
	Mandatory  bool     `json:"mandatory"`
	Content    string   `json:"content"`
}

type PreparationStatus string

const (
	PreparationReady              PreparationStatus = "ready"
	PreparationNeedsClarification PreparationStatus = "needs_clarification"
	PreparationUnsupported        PreparationStatus = "unsupported"
)

type PromptUsage struct {
	ID      PromptID `json:"id"`
	Version string   `json:"version"`
	SHA256  string   `json:"sha256"`
}

type PreparationTrace struct {
	Normalized  NormalizedRequest `json:"normalized"`
	Plan        TaskPlan          `json:"plan"`
	Routing     map[string]string `json:"routing"`
	Blocks      []RetrievedBlock  `json:"blocks"`
	Warnings    []string          `json:"warnings"`
	PromptUsage []PromptUsage     `json:"promptUsage"`
	ModelCalls  int               `json:"modelCalls"`
}

type PreparationResult struct {
	Status             PreparationStatus
	Plan               TaskPlan
	ModelRequest       *ModelRequest
	Clarification      *Clarification
	Trace              PreparationTrace
	UnsupportedMessage string
}

type StructuredResponse struct {
	Answer                 string           `json:"answer"`
	EntityCitations        []EntityCitation `json:"entityCitations"`
	DocumentationCitations []string         `json:"documentationCitations"`
	Limitations            []string         `json:"limitations"`
}

type EntityCitation struct {
	DocumentType string `json:"documentType"`
	Identity     string `json:"identity"`
}

type ResponseValidation struct {
	Valid    bool               `json:"valid"`
	Repaired bool               `json:"repaired"`
	Errors   []string           `json:"errors"`
	Response StructuredResponse `json:"response"`
}
