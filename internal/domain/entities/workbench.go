package entities

import (
	"encoding/json"
	"time"
)

type Actor struct {
	ID          string
	Username    string
	DisplayName string
}

type Workspace struct {
	ID   string
	Name string
}

type ModelSnapshot struct {
	ProfileID       string
	ConnectionID    string
	Adapter         string
	ProviderModelID string
	DisplayName     string
}

type Conversation struct {
	ID           string
	ActorID      string
	WorkspaceID  string
	Model        ModelSnapshot
	Archived     bool
	MessageCount int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Message struct {
	ID             string
	ConversationID string
	Role           string
	Content        string
	Sequence       int64
	CreatedAt      time.Time
}

type RunInput struct {
	RequestID      string
	Actor          Actor
	Workspace      Workspace
	ConversationID string
	Prompt         string
	Model          ModelSnapshot
	Snapshot       []byte
	Generation     string
	HeadRevisionID string
	SnapshotSHA256 string
}

type Run struct {
	ID                  string
	RequestID           string
	UserMessageID       string
	UserMessageSequence int64
}

type KnowledgeSearchQuery struct {
	NormalizedPrompt string
	Terms            []string
	ExpandedTerms    []string
	Phrases          []string
}

type KnowledgeMatch struct {
	ChunkID      string
	DocumentPath string
	SourceFile   string
	Title        string
	Heading      string
	Content      string
	Score        int
	MatchedTerms []string
}

type KnowledgeRetrieval struct {
	Available bool
	BundleID  string
	Query     KnowledgeSearchQuery
	Matches   []KnowledgeMatch
	Error     string
}

type DomainContextMatch struct {
	DocumentType string
	Identity     string
	DisplayName  string
	MatchKind    string
	Score        int
	MatchedTerms []string
	RelatedTo    []string
	Snapshot     json.RawMessage
}

type DomainContext struct {
	Available             bool
	Kind                  string
	SchemaVersion         int
	DomainVersion         string
	Workspace             json.RawMessage
	InstalledIntegrations []json.RawMessage
	TotalDocuments        int
	Limit                 int
	Matches               []DomainContextMatch
	Error                 string
}

type ConversationContext struct {
	Limit          int
	BeforeSequence int64
	Messages       []Message
	Error          string
}

type PromptIntent struct {
	Kind    string
	Signals []string
}

type ContextDecision struct {
	Source string
	ID     string
	Score  int
	Chars  int
	Status string
	Reason string
}

type ContextSectionBudget struct {
	Name           string
	Priority       int
	BudgetChars    int
	UsedChars      int
	CandidateCount int
	IncludedCount  int
}

type ContextBudget struct {
	MaxChars             int
	SystemChars          int
	CurrentPromptChars   int
	ContextEnvelopeChars int
	HistoryChars         int
	DocumentationChars   int
	DomainChars          int
	TotalChars           int
	EstimatedTokens      int
}

type ContextPlan struct {
	Intent         PromptIntent
	SourcePriority []string
	Budget         ContextBudget
	Sections       []ContextSectionBudget
	Decisions      []ContextDecision
	Warnings       []string
}

type ModelMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ModelRequest struct {
	Model        ModelSnapshot  `json:"model"`
	SystemPrompt string         `json:"system"`
	Messages     []ModelMessage `json:"messages"`
}

type RunDebugRecord struct {
	RequestID      string
	RunID          string
	ConversationID string
	ActorID        string
	WorkspaceID    string
	Prompt         string
	Generation     string
	HeadRevisionID string
	SnapshotSHA256 string
	StartedAt      time.Time
	Knowledge      KnowledgeRetrieval
	Domain         DomainContext
	Conversation   ConversationContext
	ContextPlan    ContextPlan
	ModelRequest   ModelRequest
}
