package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	"github.com/jackc/pgx/v5"
)

var _ ports.InteractionRepository = (*WorkbenchRepository)(nil)

func (r *WorkbenchRepository) GetActiveInteraction(ctx context.Context, actorID, workspaceID, conversationID string) (*entities.Interaction, error) {
	return r.getInteraction(ctx, actorID, workspaceID, conversationID, "", true)
}

func (r *WorkbenchRepository) GetInteraction(ctx context.Context, actorID, workspaceID, conversationID, interactionID string) (*entities.Interaction, error) {
	return r.getInteraction(ctx, actorID, workspaceID, conversationID, interactionID, false)
}

func (r *WorkbenchRepository) getInteraction(ctx context.Context, actorID, workspaceID, conversationID, interactionID string, active bool) (*entities.Interaction, error) {
	query := `SELECT i.id, i.conversation_id, i.root_message_id, i.status, i.plan, i.plan_version,
		i.workspace_generation, i.workspace_snapshot_sha256, i.documentation_version,
		i.created_at, i.updated_at, i.completed_at
		FROM interactions i JOIN conversations c ON c.id=i.conversation_id
		WHERE c.actor_id=$1 AND c.workspace_id=$2 AND c.id=$3`
	args := []any{actorID, workspaceID, conversationID}
	if active {
		query += ` AND i.status IN ('planning','resolving','awaiting_clarification','ready','generating') ORDER BY i.created_at DESC LIMIT 1`
	} else {
		query += ` AND i.id=$4`
		args = append(args, interactionID)
	}
	interaction, err := scanInteraction(r.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get interaction: %w", err)
	}
	return &interaction, nil
}

func (r *WorkbenchRepository) CreateInteraction(ctx context.Context, runID, conversationID, rootMessageID, generation, snapshotSHA256, documentationVersion string) (entities.Interaction, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Interaction{}, fmt.Errorf("begin create interaction: %w", err)
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `INSERT INTO interactions (
		conversation_id, root_message_id, status, workspace_generation, workspace_snapshot_sha256, documentation_version
	) VALUES ($1,$2,'planning',$3,$4,$5)
	RETURNING id, conversation_id, root_message_id, status, plan, plan_version,
		workspace_generation, workspace_snapshot_sha256, documentation_version, created_at, updated_at, completed_at`,
		conversationID, rootMessageID, generation, snapshotSHA256, documentationVersion)
	interaction, err := scanInteraction(row)
	if err != nil {
		return entities.Interaction{}, mapWriteError("create interaction", err)
	}
	command, err := tx.Exec(ctx, `UPDATE runs SET interaction_id=$2 WHERE id=$1 AND status='running'`, runID, interaction.ID)
	if err != nil || command.RowsAffected() != 1 {
		return entities.Interaction{}, domainerrors.ErrConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Interaction{}, fmt.Errorf("commit create interaction: %w", err)
	}
	return interaction, nil
}

func (r *WorkbenchRepository) AttachRunInteraction(ctx context.Context, runID, interactionID string) error {
	command, err := r.pool.Exec(ctx, `UPDATE runs SET interaction_id=$2 WHERE id=$1 AND status='running'`, runID, interactionID)
	if err != nil {
		return fmt.Errorf("attach run interaction: %w", err)
	}
	if command.RowsAffected() != 1 {
		return domainerrors.ErrConflict
	}
	return nil
}

func (r *WorkbenchRepository) SupersedeInteraction(ctx context.Context, interactionID string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE interactions SET status='superseded', updated_at=now(), completed_at=now()
		WHERE id=$1 AND status IN ('planning','resolving','awaiting_clarification','ready','generating')`, interactionID); err != nil {
		return fmt.Errorf("supersede interaction: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE clarifications SET status='superseded', resolved_at=now()
		WHERE interaction_id=$1 AND status='open'`, interactionID); err != nil {
		return fmt.Errorf("supersede clarification: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *WorkbenchRepository) SaveInteraction(ctx context.Context, value entities.Interaction, expectedVersion int) (entities.Interaction, error) {
	plan, err := json.Marshal(value.Plan)
	if err != nil {
		return entities.Interaction{}, fmt.Errorf("encode interaction plan: %w", err)
	}
	row := r.pool.QueryRow(ctx, `UPDATE interactions SET status=$3, plan=$4, plan_version=plan_version+1,
		workspace_generation=$5, workspace_snapshot_sha256=$6, documentation_version=$7, updated_at=now(),
		completed_at=CASE WHEN $3 IN ('completed','failed','cancelled','superseded') THEN now() ELSE NULL END
		WHERE id=$1 AND plan_version=$2
		RETURNING id, conversation_id, root_message_id, status, plan, plan_version,
		workspace_generation, workspace_snapshot_sha256, documentation_version, created_at, updated_at, completed_at`,
		value.ID, expectedVersion, value.Status, plan, value.WorkspaceGeneration, value.WorkspaceSnapshotSHA256, value.DocumentationVersion)
	updated, err := scanInteraction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return entities.Interaction{}, domainerrors.ErrConflict
	}
	if err != nil {
		return entities.Interaction{}, fmt.Errorf("save interaction: %w", err)
	}
	return updated, nil
}

func (r *WorkbenchRepository) CompleteInteractionRun(ctx context.Context, runID, conversationID, interactionID, content string) (entities.Message, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Message{}, fmt.Errorf("begin complete interaction run: %w", err)
	}
	defer tx.Rollback(ctx)
	var message entities.Message
	if err := tx.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content) VALUES ($1,'assistant',$2)
		RETURNING id, conversation_id, role, content, sequence, created_at`, conversationID, content).
		Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.Sequence, &message.CreatedAt); err != nil {
		return entities.Message{}, fmt.Errorf("insert assistant message: %w", err)
	}
	if command, err := tx.Exec(ctx, `UPDATE runs SET status='completed', completed_at=now() WHERE id=$1 AND status='running'`, runID); err != nil || command.RowsAffected() != 1 {
		return entities.Message{}, domainerrors.ErrConflict
	}
	if command, err := tx.Exec(ctx, `UPDATE interactions SET status='completed', completed_at=now(), updated_at=now()
		WHERE id=$1 AND status IN ('ready','generating','resolving')`, interactionID); err != nil || command.RowsAffected() != 1 {
		return entities.Message{}, domainerrors.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, conversationID); err != nil {
		return entities.Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Message{}, fmt.Errorf("commit complete interaction run: %w", err)
	}
	return message, nil
}

func (r *WorkbenchRepository) FailInteraction(ctx context.Context, runID, interactionID, runStatus, code, message string) error {
	if runStatus != "failed" && runStatus != "cancelled" {
		runStatus = "failed"
	}
	interactionStatus := runStatus
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE runs SET status=$2, error_code=$3, error_message=$4, completed_at=now()
		WHERE id=$1 AND status='running'`, runID, runStatus, code, message); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE interactions SET status=$2, updated_at=now(), completed_at=now()
		WHERE id=$1 AND status IN ('planning','resolving','ready','generating')`, interactionID, interactionStatus); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanInteraction(row rowScanner) (entities.Interaction, error) {
	var value entities.Interaction
	var plan []byte
	if err := row.Scan(&value.ID, &value.ConversationID, &value.RootMessageID, &value.Status, &plan, &value.PlanVersion,
		&value.WorkspaceGeneration, &value.WorkspaceSnapshotSHA256, &value.DocumentationVersion,
		&value.CreatedAt, &value.UpdatedAt, &value.CompletedAt); err != nil {
		return value, err
	}
	if len(plan) > 0 {
		if err := json.Unmarshal(plan, &value.Plan); err != nil {
			return value, fmt.Errorf("decode interaction plan: %w", err)
		}
	}
	return value, nil
}
