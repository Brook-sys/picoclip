package domain

import "time"

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
	RunStatusTimeout   RunStatus = "timeout"
)

type Run struct {
	ID           string     `json:"id"`
	TaskID       string     `json:"task_id"`
	AgentID      string     `json:"agent_id"`
	DriverType   string     `json:"driver_type"`
	Status       RunStatus  `json:"status"`
	Attempt      int        `json:"attempt"`
	Input        string     `json:"input"`
	Output       string     `json:"output"`
	Error        string     `json:"error"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	TotalTokens  int        `json:"total_tokens"`
	ProcessID    int        `json:"process_id,omitempty"`
	LastOutputAt *time.Time `json:"last_output_at,omitempty"`
	StallTimeout int        `json:"stall_timeout,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}
