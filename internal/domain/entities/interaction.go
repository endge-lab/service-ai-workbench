package entities

import (
	"encoding/json"
	"time"
)

type InteractionStatus string

const (
	InteractionPlanning              InteractionStatus = "planning"
	InteractionResolving             InteractionStatus = "resolving"
	InteractionAwaitingClarification InteractionStatus = "awaiting_clarification"
	InteractionReady                 InteractionStatus = "ready"
	InteractionGenerating            InteractionStatus = "generating"
	InteractionCompleted             InteractionStatus = "completed"
	InteractionFailed                InteractionStatus = "failed"
	InteractionCancelled             InteractionStatus = "cancelled"
	InteractionSuperseded            InteractionStatus = "superseded"
)

type Interaction struct {
	ID                      string
	ConversationID          string
	RootMessageID           string
	Status                  InteractionStatus
	Plan                    TaskPlan
	PlanVersion             int
	WorkspaceGeneration     string
	WorkspaceSnapshotSHA256 string
	DocumentationVersion    string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	CompletedAt             *time.Time
}

type ClarificationCandidate struct {
	CandidateID  string          `json:"candidateId"`
	DocumentType string          `json:"documentType"`
	Identity     string          `json:"identity"`
	DisplayName  string          `json:"displayName"`
	Snapshot     json.RawMessage `json:"-"`
}

type Clarification struct {
	ID                string                   `json:"id"`
	InteractionID     string                   `json:"interactionId"`
	TaskID            string                   `json:"taskId"`
	Slot              string                   `json:"slot"`
	Question          string                   `json:"question"`
	QuestionMessageID string                   `json:"-"`
	AnswerMessageID   string                   `json:"-"`
	Candidates        []ClarificationCandidate `json:"candidates"`
	Status            string                   `json:"-"`
	PlanVersion       int                      `json:"planVersion"`
	CreatedAt         time.Time                `json:"-"`
}

type ClarificationAnswer struct {
	InteractionID       string
	ClarificationID     string
	SelectedCandidateID string
	Text                string
	UserMessageID       string
	BasePlanVersion     int
	Status              string
}
