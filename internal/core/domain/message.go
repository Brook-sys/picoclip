package domain

import "time"

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAgent     MessageRole = "agent"
	MessageRoleSystem    MessageRole = "system"
	MessageRoleDelegated MessageRole = "delegated"
)

type Message struct {
	ID        string      `json:"id"`
	TaskID    string      `json:"task_id"`
	FromID    string      `json:"from_id,omitempty"`
	ToID      string      `json:"to_id,omitempty"`
	Role      MessageRole `json:"role"`
	Body      string      `json:"body"`
	CreatedAt time.Time   `json:"created_at"`
}
