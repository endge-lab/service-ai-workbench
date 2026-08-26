package entities

import "time"

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
	ID        string
	RequestID string
}
