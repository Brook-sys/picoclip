package domain

import "time"

type DiagnosticsReport struct {
	StorageType   string            `json:"storage_type"`
	DatabasePath  string            `json:"database_path"`
	WorkspacePath string            `json:"workspace_path"`
	RuntimePath   string            `json:"runtime_path"`
	LogLevel      string            `json:"log_level"`
	DebugMode     bool              `json:"debug_mode"`
	Checks        []DiagnosticCheck `json:"checks"`
	GeneratedAt   time.Time         `json:"generated_at"`
}

type RecoveryLivenessDiagnostics struct {
	GeneratedAt time.Time                         `json:"generated_at"`
	Counts      map[string]int                    `json:"counts"`
	Items       []RecoveryLivenessDiagnosticsItem `json:"items"`
}

type RecoveryLivenessDiagnosticsItem struct {
	Kind      string    `json:"kind"`
	TaskID    string    `json:"task_id,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	RunID     string    `json:"run_id,omitempty"`
	WakeupID  string    `json:"wakeup_id,omitempty"`
	EventID   string    `json:"event_id,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	DueAt     time.Time `json:"due_at,omitempty"`
	Status    string    `json:"status,omitempty"`
}
