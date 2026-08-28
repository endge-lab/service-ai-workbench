package entities

import "encoding/json"

type KnowledgeSearchQuery struct {
	NormalizedPrompt              string
	Terms, ExpandedTerms, Phrases []string
}

type KnowledgeMatch struct {
	ChunkID, DocumentPath, SourceFile, Title, Heading, Content string
	Score                                                      int
	MatchedTerms                                               []string
}

type KnowledgeRetrieval struct {
	Available bool
	BundleID  string
	Query     KnowledgeSearchQuery
	Matches   []KnowledgeMatch
	Error     string
}

type DomainContextMatch struct {
	DocumentType, Identity, DisplayName, MatchKind string
	Score                                          int
	MatchedTerms, RelatedTo                        []string
	Snapshot                                       json.RawMessage
}

type DomainContext struct {
	Available             bool
	Kind                  string
	SchemaVersion         int
	DomainVersion         string
	Workspace             json.RawMessage
	InstalledIntegrations []json.RawMessage
	TotalDocuments, Limit int
	Matches               []DomainContextMatch
	Error                 string
}

type DomainSelectionInput struct {
	WorkspaceID, Generation, SnapshotSHA256 string
	Snapshot                                []byte
	Query                                   KnowledgeSearchQuery
	Limit                                   int
}
