package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"picoclip/internal/core/domain"
)

type MessageRepository struct {
	db *sql.DB
}

func (r *MessageRepository) Create(ctx context.Context, message domain.Message) error {
	query := `
		INSERT INTO messages (id, task_id, from_id, to_id, role, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, query,
		message.ID, message.TaskID, message.FromID, message.ToID, string(message.Role), message.Body, message.CreatedAt,
	)
	return err
}

func (r *MessageRepository) ListByTask(ctx context.Context, taskID string) ([]domain.Message, error) {
	query := `
		SELECT id, task_id, from_id, to_id, role, body, created_at
		FROM messages WHERE task_id = ? ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func scanMessage(row scanner) (domain.Message, error) {
	var m domain.Message
	var roleStr string

	err := row.Scan(&m.ID, &m.TaskID, &m.FromID, &m.ToID, &roleStr, &m.Body, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Message{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Message{}, err
	}
	m.Role = domain.MessageRole(roleStr)
	return m, nil
}

func (r *MessageRepository) DeleteByTask(ctx context.Context, taskID string) error {
	q := getQueryer(ctx, r.db)
	_, err := q.ExecContext(ctx, `DELETE FROM messages WHERE task_id = ?`, taskID)
	return err
}
