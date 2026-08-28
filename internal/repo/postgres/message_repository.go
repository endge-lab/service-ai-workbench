package postgres

import (
	"context"
	"fmt"
	"slices"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	domainerrors "github.com/endge-lab/service-ai-workbench/internal/domain/errors"
)

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
