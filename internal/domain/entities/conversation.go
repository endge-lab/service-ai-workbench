package entities

import "time"

type Actor struct {
	ID          string
	Username    string
	DisplayName string
}

type Workspace struct{ ID, Name string }

type ModelSnapshot struct {
	ProfileID, ConnectionID, Adapter, ProviderModelID, DisplayName string
}

type Conversation struct {
	ID, ActorID, WorkspaceID string
	Model                    ModelSnapshot
	Archived                 bool
	MessageCount             int64
	CreatedAt, UpdatedAt     time.Time
}

type Message struct {
	ID, ConversationID, Role, Content string
	Sequence                          int64
	CreatedAt                         time.Time
}

type ConversationContext struct {
	Limit             int
	BeforeSequence    int64
	Messages          []Message
	Error             string
	OpenClarification *Clarification
}
