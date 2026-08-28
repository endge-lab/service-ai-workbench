package files

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
	return &Recorder{enabled: cfg.Debug.Enabled, outputPath: cfg.Debug.OutputPath, logger: logger}
}

func (r *Recorder) Record(_ context.Context, record entities.RunDebugRecord) {
	if !r.enabled {
		return
	}
	if err := r.write(record); err != nil {
		r.logger.Warn("write AI preparation trace", zap.Error(err), zap.String("request_id", record.RequestID))
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

	metadata := map[string]any{
		"createdAt": record.StartedAt.UTC(), "requestId": record.RequestID, "runId": record.RunID,
		"conversationId": record.ConversationID, "actorId": record.ActorID, "workspaceId": record.WorkspaceID,
		"workspaceGeneration": record.Generation, "workspaceHeadRevisionId": record.HeadRevisionID,
		"workspaceSnapshotSha256": record.SnapshotSHA256, "knowledgeBundleId": record.Knowledge.BundleID,
	}
	stages := []debugStage{
		{"00-metadata.json", "completed", metadata},
		{"01-original-request.json", "completed", map[string]any{"text": record.Prompt}},
		{"02-normalization.json", statusFor(record.Preparation.Normalized.NormalizedText != ""), record.Preparation.Normalized},
		{"03-plan.json", statusFor(len(record.Preparation.Plan.Tasks) > 0), record.Preparation.Plan},
		{"04-routing.json", statusFor(len(record.Preparation.Routing) > 0), record.Preparation.Routing},
		{"05-documentation.json", statusFor(record.Knowledge.Available || record.Knowledge.Error != ""), record.Knowledge},
		{"06-domain-resolution.json", statusFor(record.Domain.Available || record.Domain.Error != ""), record.Domain},
		{"07-interaction-clarification.json", statusFor(record.Interaction.ID != ""), map[string]any{"interaction": record.Interaction, "clarification": record.Clarification}},
		{"08-conversation-context.json", statusFor(len(record.Conversation.Messages) > 0), record.Conversation},
		{"09-adequacy-context-plan.json", statusFor(len(record.Preparation.Blocks) > 0), map[string]any{"blocks": record.Preparation.Blocks, "warnings": record.Preparation.Warnings}},
		{"10-prompt-manifest.json", statusFor(len(record.Preparation.PromptUsage) > 0), map[string]any{"prompts": record.Preparation.PromptUsage, "modelCalls": record.Preparation.ModelCalls}},
		{"11-model-request.json", statusFor(len(record.ModelRequest.Messages) > 0), sanitizeModelRequest(record.ModelRequest)},
		{"12-response-validation.json", statusFor(record.Response.Valid || len(record.Response.Errors) > 0), record.Response},
	}
	for _, stage := range stages {
		encoded, err := json.MarshalIndent(map[string]any{"status": stage.status, "data": stage.data}, "", "  ")
		if err != nil {
			return fmt.Errorf("encode %s: %w", stage.name, err)
		}
		if err := writeProtectedFile(directory, stage.name, append(encoded, '\n')); err != nil {
			return err
		}
	}
	return nil
}

type debugStage struct {
	name   string
	status string
	data   any
}

func statusFor(completed bool) string {
	if completed {
		return "completed"
	}
	return "skipped"
}

func sanitizeModelRequest(request entities.ModelRequest) any {
	return struct {
		Model    entities.ModelSnapshot  `json:"model"`
		System   string                  `json:"system"`
		Messages []entities.ModelMessage `json:"messages"`
	}{Model: request.Model, System: request.SystemPrompt, Messages: request.Messages}
}

func writeProtectedFile(directory, name string, content []byte) error {
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return os.Chmod(path, 0o600)
}
