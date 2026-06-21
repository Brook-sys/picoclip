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
