package domain

import "time"

type TaskStatus string

const (
	TaskStatusBacklog    TaskStatus = "backlog"
	TaskStatusTodo       TaskStatus = "todo"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusInReview   TaskStatus = "in_review"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusDone       TaskStatus = "done"
	TaskStatusCancelled  TaskStatus = "cancelled"
)

type Task struct {
	ID                  string     `json:"id"`
	ParentID            string     `json:"parent_id,omitempty"`
	WorkspaceID         string     `json:"workspace_id,omitempty"`
	AgentID             string     `json:"agent_id"`
	Title               string     `json:"title"`
	Prompt              string     `json:"prompt"`
	Status              TaskStatus `json:"status"`
	Priority            int        `json:"priority"`
	Attempts            int        `json:"attempts"`
	MaxAttempts         int        `json:"max_attempts"`
	NeedsRun            bool       `json:"needs_run"`
	CheckoutRunID       string     `json:"checkout_run_id,omitempty"`
	CheckedOutByAgentID string     `json:"checked_out_by_agent_id,omitempty"`
	CancelReason        string     `json:"cancel_reason,omitempty"`
	InputTokens         int        `json:"input_tokens"`
	OutputTokens        int        `json:"output_tokens"`
	TotalTokens         int        `json:"total_tokens"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	CancelledAt         *time.Time `json:"cancelled_at,omitempty"`
}
