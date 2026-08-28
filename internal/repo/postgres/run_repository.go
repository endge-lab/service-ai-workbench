package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/jackc/pgx/v5"
)

func (r *WorkbenchRepository) StartRun(ctx context.Context, input entities.RunInput) (entities.Run, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Run{}, fmt.Errorf("begin run: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := upsertProjections(ctx, tx, input.Actor, input.Workspace); err != nil {
		return entities.Run{}, err
	}
	var stored entities.ModelSnapshot
	if err := tx.QueryRow(ctx, `SELECT model_profile_id, connection_id, adapter, provider_model_id, model_display_name
		FROM conversations WHERE id=$1 AND actor_id=$2 AND workspace_id=$3 AND archived_at IS NULL FOR UPDATE`,
		input.ConversationID, input.Actor.ID, input.Workspace.ID).Scan(&stored.ProfileID, &stored.ConnectionID, &stored.Adapter, &stored.ProviderModelID, &stored.DisplayName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return entities.Run{}, domainerrors.ErrNotFound
		}
		return entities.Run{}, fmt.Errorf("lock conversation: %w", err)
	}
	if stored != input.Model {
		return entities.Run{}, domainerrors.ErrConflict
	}
	var run entities.Run
	err = tx.QueryRow(ctx, `INSERT INTO runs (
		request_id, conversation_id, actor_id, adapter, model_profile_id, provider_model_id,
		status, snapshot_generation, snapshot_head_revision_id, snapshot_sha256, snapshot_size
	) VALUES ($1,$2,$3,$4,$5,$6,'running',$7,$8,$9,$10) RETURNING id, request_id`,
		input.RequestID, input.ConversationID, input.Actor.ID, input.Model.Adapter, input.Model.ProfileID,
		input.Model.ProviderModelID, input.Generation, input.HeadRevisionID, input.SnapshotSHA256, len(input.Snapshot)).Scan(&run.ID, &run.RequestID)
	if err != nil {
		return entities.Run{}, mapWriteError("insert run", err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content) VALUES ($1,'user',$2)
		RETURNING id, sequence`, input.ConversationID, input.Prompt).Scan(&run.UserMessageID, &run.UserMessageSequence); err != nil {
		return entities.Run{}, fmt.Errorf("insert user message: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, input.ConversationID); err != nil {
		return entities.Run{}, fmt.Errorf("touch conversation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Run{}, fmt.Errorf("commit run start: %w", err)
	}
	return run, nil
}

func (r *WorkbenchRepository) CompleteRun(ctx context.Context, runID, conversationID, content string) (entities.Message, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Message{}, fmt.Errorf("begin complete run: %w", err)
	}
	defer tx.Rollback(ctx)
	var message entities.Message
	err = tx.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content) VALUES ($1,'assistant',$2)
		RETURNING id, conversation_id, role, content, sequence, created_at`, conversationID, content).
		Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.Sequence, &message.CreatedAt)
	if err != nil {
		return entities.Message{}, fmt.Errorf("insert assistant message: %w", err)
	}
	command, err := tx.Exec(ctx, `UPDATE runs SET status='completed', completed_at=now() WHERE id=$1 AND status='running'`, runID)
	if err != nil || command.RowsAffected() != 1 {
		return entities.Message{}, domainerrors.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, conversationID); err != nil {
		return entities.Message{}, fmt.Errorf("touch completed conversation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Message{}, fmt.Errorf("commit completed run: %w", err)
	}
	return message, nil
}

func (r *WorkbenchRepository) FailRun(ctx context.Context, runID, runStatus, code, message string) error {
	command, err := r.pool.Exec(ctx, `UPDATE runs SET status=$2, error_code=$3, error_message=$4, completed_at=now()
		WHERE id=$1 AND status='running'`, runID, runStatus, code, message)
	if err != nil {
		return fmt.Errorf("fail run: %w", err)
	}
	if command.RowsAffected() != 1 {
		return domainerrors.ErrConflict
	}
	return nil
}
