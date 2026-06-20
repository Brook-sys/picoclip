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
	ID         string     `json:"id"`
	TaskID     string     `json:"task_id"`
	AgentID    string     `json:"agent_id"`
	DriverType string     `json:"driver_type"`
	Status     RunStatus  `json:"status"`
	Attempt    int        `json:"attempt"`
	Input      string     `json:"input"`
	Output     string     `json:"output"`
	Error      string     `json:"error"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}
