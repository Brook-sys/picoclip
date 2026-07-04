package domain

import "time"

type UsageEvent struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	TaskID       string    `json:"task_id"`
	AgentID      string    `json:"agent_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CachedTokens int       `json:"cached_tokens"`
	CostMicros   int64     `json:"cost_micros"`
	CreatedAt    time.Time `json:"created_at"`
}
