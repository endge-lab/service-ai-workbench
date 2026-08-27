package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WorkbenchRepository struct {
	pool *pgxpool.Pool
}

func NewWorkbenchRepository(pool *pgxpool.Pool) *WorkbenchRepository {
	return &WorkbenchRepository{pool: pool}
}

func (r *WorkbenchRepository) ListConversations(ctx context.Context, actorID, workspaceID string, includeArchived bool, limit int, before *time.Time) ([]entities.Conversation, *time.Time, error) {
	if limit < 1 || limit > 100 {
		return nil, nil, domainerrors.ErrInvalidInput
	}
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.actor_id, c.workspace_id, c.model_profile_id, c.connection_id,
		       c.adapter, c.provider_model_id, c.model_display_name, c.archived_at IS NOT NULL,
		       (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id), c.created_at, c.updated_at
		FROM conversations c
		WHERE c.actor_id = $1 AND c.workspace_id = $2
		  AND ($3 OR c.archived_at IS NULL)
		  AND ($4::timestamptz IS NULL OR c.updated_at < $4)
		ORDER BY c.updated_at DESC, c.id DESC
		LIMIT $5`, actorID, workspaceID, includeArchived, before, limit+1)
	if err != nil {
		return nil, nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	items := make([]entities.Conversation, 0, limit+1)
	for rows.Next() {
		item, err := scanConversation(rows)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate conversations: %w", err)
	}
	var next *time.Time
	if len(items) > limit {
		cursor := items[limit-1].UpdatedAt
		next = &cursor
		items = items[:limit]
	}
	return items, next, nil
}

func (r *WorkbenchRepository) CreateConversation(ctx context.Context, actor entities.Actor, workspace entities.Workspace, model entities.ModelSnapshot) (entities.Conversation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Conversation{}, fmt.Errorf("begin create conversation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := upsertProjections(ctx, tx, actor, workspace); err != nil {
		return entities.Conversation{}, err
	}
	conversation, err := insertConversation(ctx, tx, actor.ID, workspace.ID, model)
	if err != nil {
		return entities.Conversation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Conversation{}, fmt.Errorf("commit create conversation: %w", err)
	}
	return conversation, nil
}

func (r *WorkbenchRepository) ResetConversation(ctx context.Context, actor entities.Actor, workspace entities.Workspace, currentID string, model entities.ModelSnapshot) (entities.Conversation, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Conversation{}, fmt.Errorf("begin reset conversation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := upsertProjections(ctx, tx, actor, workspace); err != nil {
		return entities.Conversation{}, err
	}
	if currentID != "" {
		command, err := tx.Exec(ctx, `UPDATE conversations SET archived_at = now(), updated_at = now()
			WHERE id = $1 AND actor_id = $2 AND workspace_id = $3 AND archived_at IS NULL`, currentID, actor.ID, workspace.ID)
		if err != nil {
			return entities.Conversation{}, fmt.Errorf("archive current conversation: %w", err)
		}
		if command.RowsAffected() == 0 {
			return entities.Conversation{}, domainerrors.ErrNotFound
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE conversations SET archived_at = now(), updated_at = now()
			WHERE actor_id = $1 AND workspace_id = $2 AND archived_at IS NULL`, actor.ID, workspace.ID); err != nil {
			return entities.Conversation{}, fmt.Errorf("archive active conversation: %w", err)
		}
	}
	conversation, err := insertConversation(ctx, tx, actor.ID, workspace.ID, model)
	if err != nil {
		return entities.Conversation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Conversation{}, fmt.Errorf("commit reset conversation: %w", err)
	}
	return conversation, nil
}

func (r *WorkbenchRepository) UpdateConversationModel(ctx context.Context, actorID, workspaceID, conversationID string, model entities.ModelSnapshot) (entities.Conversation, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE conversations c
		SET model_profile_id = $4, connection_id = $5, adapter = $6,
		    provider_model_id = $7, model_display_name = $8, updated_at = now()
		WHERE c.id = $1 AND c.actor_id = $2 AND c.workspace_id = $3 AND c.archived_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.conversation_id = c.id)
		RETURNING c.id, c.actor_id, c.workspace_id, c.model_profile_id, c.connection_id,
		          c.adapter, c.provider_model_id, c.model_display_name, false, 0, c.created_at, c.updated_at`,
		conversationID, actorID, workspaceID, model.ProfileID, model.ConnectionID, model.Adapter, model.ProviderModelID, model.DisplayName)
	conversation, err := scanConversation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return entities.Conversation{}, domainerrors.ErrConflict
	}
	if err != nil {
		return entities.Conversation{}, fmt.Errorf("update conversation model: %w", err)
	}
	return conversation, nil
}

func (r *WorkbenchRepository) ListMessages(ctx context.Context, actorID, workspaceID, conversationID string, limit int, beforeSequence *int64) ([]entities.Message, *int64, error) {
	if limit < 1 || limit > 50 {
		return nil, nil, domainerrors.ErrInvalidInput
	}
	var owned bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversations WHERE id=$1 AND actor_id=$2 AND workspace_id=$3)`, conversationID, actorID, workspaceID).Scan(&owned); err != nil {
		return nil, nil, fmt.Errorf("verify conversation ownership: %w", err)
	}
	if !owned {
		return nil, nil, domainerrors.ErrNotFound
	}
	rows, err := r.pool.Query(ctx, `SELECT id, conversation_id, role, content, sequence, created_at
		FROM messages WHERE conversation_id=$1 AND ($2::bigint IS NULL OR sequence < $2)
		ORDER BY sequence DESC LIMIT $3`, conversationID, beforeSequence, limit+1)
	if err != nil {
		return nil, nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	items := make([]entities.Message, 0, limit+1)
	for rows.Next() {
		var item entities.Message
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.Role, &item.Content, &item.Sequence, &item.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan message: %w", err)
		}
		items = append(items, item)
	}
	var next *int64
	if len(items) > limit {
		cursor := items[limit-1].Sequence
		next = &cursor
		items = items[:limit]
	}
	slices.Reverse(items)
	return items, next, rows.Err()
}

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

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConversation(row rowScanner) (entities.Conversation, error) {
	var item entities.Conversation
	err := row.Scan(&item.ID, &item.ActorID, &item.WorkspaceID, &item.Model.ProfileID, &item.Model.ConnectionID,
		&item.Model.Adapter, &item.Model.ProviderModelID, &item.Model.DisplayName, &item.Archived,
		&item.MessageCount, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func insertConversation(ctx context.Context, tx pgx.Tx, actorID, workspaceID string, model entities.ModelSnapshot) (entities.Conversation, error) {
	row := tx.QueryRow(ctx, `INSERT INTO conversations (
		actor_id, workspace_id, model_profile_id, connection_id, adapter, provider_model_id, model_display_name
	) VALUES ($1,$2,$3,$4,$5,$6,$7)
	RETURNING id, actor_id, workspace_id, model_profile_id, connection_id, adapter,
	          provider_model_id, model_display_name, false, 0, created_at, updated_at`,
		actorID, workspaceID, model.ProfileID, model.ConnectionID, model.Adapter, model.ProviderModelID, model.DisplayName)
	conversation, err := scanConversation(row)
	if err != nil {
		return entities.Conversation{}, mapWriteError("insert conversation", err)
	}
	return conversation, nil
}

func upsertProjections(ctx context.Context, tx pgx.Tx, actor entities.Actor, workspace entities.Workspace) error {
	if _, err := tx.Exec(ctx, `INSERT INTO users (id,username,display_name) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO UPDATE SET username=excluded.username, display_name=excluded.display_name, last_seen_at=now()`,
		actor.ID, actor.Username, actor.DisplayName); err != nil {
		return fmt.Errorf("upsert user projection: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workspaces (id,name) VALUES ($1,$2)
		ON CONFLICT (id) DO UPDATE SET name=excluded.name, last_seen_at=now()`, workspace.ID, workspace.Name); err != nil {
		return fmt.Errorf("upsert workspace projection: %w", err)
	}
	return nil
}

func mapWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return domainerrors.ErrConflict
	}
	return fmt.Errorf("%s: %w", operation, err)
}
