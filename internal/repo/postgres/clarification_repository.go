package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
	"github.com/jackc/pgx/v5"
)

func (r *WorkbenchRepository) GetOpenClarification(ctx context.Context, actorID, workspaceID, conversationID string) (*entities.Clarification, error) {
	row := r.pool.QueryRow(ctx, `SELECT cl.id, cl.interaction_id, cl.task_id, cl.slot, qm.content,
		cl.question_message_id, COALESCE(cl.answer_message_id::text,''), cl.candidate_snapshot,
		cl.status, cl.plan_version, cl.created_at
		FROM clarifications cl
		JOIN interactions i ON i.id=cl.interaction_id
		JOIN conversations c ON c.id=i.conversation_id
		JOIN messages qm ON qm.id=cl.question_message_id
		WHERE c.actor_id=$1 AND c.workspace_id=$2 AND c.id=$3 AND cl.status='open'
		ORDER BY cl.created_at DESC LIMIT 1`, actorID, workspaceID, conversationID)
	clarification, err := scanClarification(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get open clarification: %w", err)
	}
	return &clarification, nil
}

func (r *WorkbenchRepository) CreateClarification(ctx context.Context, runID string, interaction entities.Interaction, value entities.Clarification) (entities.Clarification, entities.Message, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Clarification{}, entities.Message{}, fmt.Errorf("begin clarification: %w", err)
	}
	defer tx.Rollback(ctx)
	plan, err := json.Marshal(interaction.Plan)
	if err != nil {
		return entities.Clarification{}, entities.Message{}, err
	}
	storedCandidates := make([]storedClarificationCandidate, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		storedCandidates = append(storedCandidates, storedClarificationCandidate{
			CandidateID: candidate.CandidateID, DocumentType: candidate.DocumentType, Identity: candidate.Identity,
			DisplayName: candidate.DisplayName, Snapshot: candidate.Snapshot,
		})
	}
	candidates, err := json.Marshal(storedCandidates)
	if err != nil {
		return entities.Clarification{}, entities.Message{}, err
	}
	var message entities.Message
	if err := tx.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content)
		VALUES ($1,'assistant',$2) RETURNING id, conversation_id, role, content, sequence, created_at`,
		interaction.ConversationID, value.Question).Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.Sequence, &message.CreatedAt); err != nil {
		return entities.Clarification{}, entities.Message{}, fmt.Errorf("insert clarification message: %w", err)
	}
	row := tx.QueryRow(ctx, `INSERT INTO clarifications (
		interaction_id, task_id, slot, question_message_id, candidate_snapshot, status, plan_version
	) VALUES ($1,$2,$3,$4,$5,'open',$6)
	RETURNING id, interaction_id, task_id, slot, $7::text, question_message_id,
		'', candidate_snapshot, status, plan_version, created_at`, interaction.ID, value.TaskID, value.Slot,
		message.ID, candidates, interaction.PlanVersion+1, value.Question)
	created, err := scanClarification(row)
	if err != nil {
		return entities.Clarification{}, entities.Message{}, mapWriteError("insert clarification", err)
	}
	command, err := tx.Exec(ctx, `UPDATE interactions SET status='awaiting_clarification', plan=$2,
		plan_version=plan_version+1, updated_at=now() WHERE id=$1 AND plan_version=$3`, interaction.ID, plan, interaction.PlanVersion)
	if err != nil || command.RowsAffected() != 1 {
		return entities.Clarification{}, entities.Message{}, domainerrors.ErrConflict
	}
	command, err = tx.Exec(ctx, `UPDATE runs SET status='clarification_required', completed_at=now() WHERE id=$1 AND status='running'`, runID)
	if err != nil || command.RowsAffected() != 1 {
		return entities.Clarification{}, entities.Message{}, domainerrors.ErrConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE conversations SET updated_at=now() WHERE id=$1`, interaction.ConversationID); err != nil {
		return entities.Clarification{}, entities.Message{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Clarification{}, entities.Message{}, fmt.Errorf("commit clarification: %w", err)
	}
	return created, message, nil
}

func (r *WorkbenchRepository) ApplyClarification(ctx context.Context, answer entities.ClarificationAnswer, value entities.Interaction, expectedVersion int) (entities.Interaction, error) {
	if answer.Status != "answered" && answer.Status != "cancelled" {
		return entities.Interaction{}, domainerrors.ErrInvalidInput
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return entities.Interaction{}, fmt.Errorf("begin clarification patch: %w", err)
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `UPDATE clarifications SET answer_message_id=$2, status=$5, resolved_at=now()
		WHERE id=$1 AND interaction_id=$3 AND status='open' AND plan_version=$4`, answer.ClarificationID,
		answer.UserMessageID, answer.InteractionID, answer.BasePlanVersion, answer.Status)
	if err != nil || command.RowsAffected() != 1 {
		return entities.Interaction{}, domainerrors.ErrConflict
	}
	plan, err := json.Marshal(value.Plan)
	if err != nil {
		return entities.Interaction{}, fmt.Errorf("encode clarification plan: %w", err)
	}
	row := tx.QueryRow(ctx, `UPDATE interactions SET status=$3, plan=$4, plan_version=plan_version+1,
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
		return entities.Interaction{}, fmt.Errorf("apply clarification plan: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return entities.Interaction{}, fmt.Errorf("commit clarification patch: %w", err)
	}
	return updated, nil
}

func scanClarification(row rowScanner) (entities.Clarification, error) {
	var value entities.Clarification
	var candidates []byte
	if err := row.Scan(&value.ID, &value.InteractionID, &value.TaskID, &value.Slot, &value.Question,
		&value.QuestionMessageID, &value.AnswerMessageID, &candidates, &value.Status, &value.PlanVersion, &value.CreatedAt); err != nil {
		return value, err
	}
	var stored []storedClarificationCandidate
	if err := json.Unmarshal(candidates, &stored); err != nil {
		return value, fmt.Errorf("decode clarification candidates: %w", err)
	}
	value.Candidates = make([]entities.ClarificationCandidate, 0, len(stored))
	for _, candidate := range stored {
		value.Candidates = append(value.Candidates, entities.ClarificationCandidate{
			CandidateID: candidate.CandidateID, DocumentType: candidate.DocumentType, Identity: candidate.Identity,
			DisplayName: candidate.DisplayName, Snapshot: candidate.Snapshot,
		})
	}
	return value, nil
}

type storedClarificationCandidate struct {
	CandidateID  string          `json:"candidateId"`
	DocumentType string          `json:"documentType"`
	Identity     string          `json:"identity"`
	DisplayName  string          `json:"displayName"`
	Snapshot     json.RawMessage `json:"snapshot"`
}
