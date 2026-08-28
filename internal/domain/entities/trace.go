package entities

import "time"

type RunDebugRecord struct {
	RequestID, RunID, ConversationID, ActorID, WorkspaceID, Prompt string
	Generation, HeadRevisionID, SnapshotSHA256                     string
	StartedAt                                                      time.Time
	Knowledge                                                      KnowledgeRetrieval
	Domain                                                         DomainContext
	Conversation                                                   ConversationContext
	ModelRequest                                                   ModelRequest
	Interaction                                                    Interaction
	Clarification                                                  *Clarification
	Preparation                                                    PreparationTrace
	Response                                                       ResponseValidation
}
