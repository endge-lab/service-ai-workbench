package workbench

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/google/uuid"
)

func validateCreate(actor entities.Actor, workspace entities.Workspace, model entities.ModelSnapshot) error {
	if strings.TrimSpace(actor.ID) == "" || strings.TrimSpace(workspace.ID) == "" || !validModel(model) {
		return domainerrors.ErrInvalidInput
	}
	return nil
}

func validModel(model entities.ModelSnapshot) bool {
	if !validUUID(model.ProfileID) || !validUUID(model.ConnectionID) || strings.TrimSpace(model.ProviderModelID) == "" || strings.TrimSpace(model.DisplayName) == "" {
		return false
	}
	return model.Adapter == "anthropic" || model.Adapter == "ollama"
}

func validRun(input entities.RunInput) bool {
	linkageValid := validOptionalUUID(input.InteractionID) && validOptionalUUID(input.ReplyToClarificationID)
	if input.SelectedCandidateID != "" && (input.InteractionID == "" || input.ReplyToClarificationID == "") {
		linkageValid = false
	}
	if input.ReplyToClarificationID != "" && input.InteractionID == "" {
		linkageValid = false
	}
	return linkageValid && validUUID(input.RequestID) && validUUID(input.ConversationID) && strings.TrimSpace(input.Prompt) != "" &&
		strings.TrimSpace(input.Actor.ID) != "" && strings.TrimSpace(input.Workspace.ID) != "" && validModel(input.Model) &&
		strings.TrimSpace(input.Generation) != "" && strings.TrimSpace(input.SnapshotSHA256) != "" &&
		input.ProviderAccess.ConnectionID == input.Model.ConnectionID &&
		(input.Model.Adapter != "ollama" || strings.TrimSpace(input.ProviderAccess.BaseURL) != "")
}

func validOptionalUUID(value string) bool { return value == "" || validUUID(value) }

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func encodeTimeCursor(value *time.Time) string {
	if value == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value.UTC().Format(time.RFC3339Nano)))
}

func decodeTimeCursor(cursor string) (*time.Time, error) {
	if cursor == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, string(decoded))
	return &parsed, err
}

func encodeSequenceCursor(value *int64) string {
	if value == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(*value, 10)))
}

func decodeSequenceCursor(cursor string) (*int64, error) {
	if cursor == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	parsed, err := strconv.ParseInt(string(decoded), 10, 64)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
