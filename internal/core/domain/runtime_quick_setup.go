package domain

import "time"

type RuntimeQuickSetupField struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
}

type RuntimeQuickSetupSchema struct {
	ProfileID   string                   `json:"profile_id"`
	Title       string                   `json:"title"`
	Description string                   `json:"description"`
	Fields      []RuntimeQuickSetupField `json:"fields"`
}

type RuntimeQuickSetupView struct {
	ProfileID        string            `json:"profile_id"`
	Values           map[string]string `json:"values"`
	SecretConfigured bool              `json:"secret_configured"`
	Revision         string            `json:"revision"`
	Configured       bool              `json:"configured"`
}

type RuntimeQuickSetupInput struct {
	ProfileID   string            `json:"profile_id"`
	Values      map[string]string `json:"values"`
	APIKey      string            `json:"-"`
	ClearAPIKey bool              `json:"clear_api_key"`
	Revision    string            `json:"revision"`
}

type RuntimeModelTestResult struct {
	Status    string        `json:"status"`
	Message   string        `json:"message"`
	Output    string        `json:"output,omitempty"`
	Latency   time.Duration `json:"latency"`
	CheckedAt time.Time     `json:"checked_at"`
}
