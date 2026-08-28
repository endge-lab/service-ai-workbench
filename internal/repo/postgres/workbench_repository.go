package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ ports.ConversationRepository = (*WorkbenchRepository)(nil)

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
			WHERE id = $1 AND actor_id = $2 AND workspace_id = $3 AND archived_at IS NULL
			  AND NOT EXISTS (SELECT 1 FROM runs r WHERE r.conversation_id=conversations.id AND r.status='running')`, currentID, actor.ID, workspace.ID)
		if err != nil {
			return entities.Conversation{}, fmt.Errorf("archive current conversation: %w", err)
		}
		if command.RowsAffected() == 0 {
			var exists, running bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM conversations WHERE id=$1 AND actor_id=$2 AND workspace_id=$3),
				EXISTS(SELECT 1 FROM runs WHERE conversation_id=$1 AND status='running')`, currentID, actor.ID, workspace.ID).Scan(&exists, &running); err != nil {
				return entities.Conversation{}, fmt.Errorf("inspect conversation reset conflict: %w", err)
			}
			if exists && running {
				return entities.Conversation{}, domainerrors.ErrConflict
			}
			return entities.Conversation{}, domainerrors.ErrNotFound
		}
	} else {
		if _, err := tx.Exec(ctx, `UPDATE conversations SET archived_at = now(), updated_at = now()
			WHERE actor_id = $1 AND workspace_id = $2 AND archived_at IS NULL
			  AND NOT EXISTS (SELECT 1 FROM runs r WHERE r.conversation_id=conversations.id AND r.status='running')`, actor.ID, workspace.ID); err != nil {
			return entities.Conversation{}, fmt.Errorf("archive active conversation: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE clarifications cl SET status='superseded', resolved_at=now()
		FROM interactions i, conversations c
		WHERE cl.interaction_id=i.id AND i.conversation_id=c.id AND cl.status='open'
		  AND c.actor_id=$1 AND c.workspace_id=$2 AND ($3::text='' OR c.id::text=$3)`, actor.ID, workspace.ID, currentID); err != nil {
		return entities.Conversation{}, fmt.Errorf("supersede reset clarifications: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE interactions i SET status='superseded', updated_at=now(), completed_at=now()
		FROM conversations c
		WHERE i.conversation_id=c.id AND i.status IN ('planning','resolving','awaiting_clarification','ready','generating')
		  AND c.actor_id=$1 AND c.workspace_id=$2 AND ($3::text='' OR c.id::text=$3)`, actor.ID, workspace.ID, currentID); err != nil {
		return entities.Conversation{}, fmt.Errorf("supersede reset interactions: %w", err)
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
