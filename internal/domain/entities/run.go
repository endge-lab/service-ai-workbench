package entities

// ProviderAccess is ephemeral secret material supplied by the trusted backend
// for one run. It must never be persisted, logged or added to debug artifacts.
type ProviderAccess struct {
	ConnectionID string `json:"connectionId"`
	BaseURL      string `json:"baseUrl"`
	Credential   string `json:"-"`
}

type RunInput struct {
	RequestID                                                  string
	Actor                                                      Actor
	Workspace                                                  Workspace
	ConversationID, Prompt                                     string
	Model                                                      ModelSnapshot
	Snapshot                                                   []byte
	Generation, HeadRevisionID, SnapshotSHA256                 string
	ProviderAccess                                             ProviderAccess
	InteractionID, ReplyToClarificationID, SelectedCandidateID string
}

type Run struct {
	ID, RequestID, UserMessageID string
	UserMessageSequence          int64
	InteractionID                string
}
