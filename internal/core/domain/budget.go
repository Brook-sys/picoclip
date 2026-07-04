package domain

import "time"

type BudgetScope string

const (
	BudgetScopeGlobal    BudgetScope = "global"
	BudgetScopeWorkspace BudgetScope = "workspace"
	BudgetScopeAgent     BudgetScope = "agent"
)

type Budget struct {
	ID              string      `json:"id"`
	Scope           BudgetScope `json:"scope"`
	WorkspaceID     string      `json:"workspace_id,omitempty"`
	AgentID         string      `json:"agent_id,omitempty"`
	LimitTokens     int         `json:"limit_tokens"`
	LimitRuns       int         `json:"limit_runs"`
	LimitCostMicros int64       `json:"limit_cost_micros"`
	HardStop        bool        `json:"hard_stop"`
	Enabled         bool        `json:"enabled"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type BudgetUsage struct {
	InputTokens  int   `json:"input_tokens"`
	OutputTokens int   `json:"output_tokens"`
	CachedTokens int   `json:"cached_tokens"`
	TotalTokens  int   `json:"total_tokens"`
	Runs         int   `json:"runs"`
	CostMicros   int64 `json:"cost_micros"`
}

func (b Budget) AppliesTo(workspaceID string, agentID string) bool {
	if !b.Enabled {
		return false
	}
	switch b.Scope {
	case BudgetScopeGlobal:
		return true
	case BudgetScopeWorkspace:
		return b.WorkspaceID != "" && b.WorkspaceID == workspaceID
	case BudgetScopeAgent:
		return b.AgentID != "" && b.AgentID == agentID
	default:
		return false
	}
}

func (b Budget) Exceeded(usage BudgetUsage) bool {
	if !b.Enabled {
		return false
	}
	if b.LimitTokens > 0 && usage.TotalTokens >= b.LimitTokens {
		return true
	}
	if b.LimitRuns > 0 && usage.Runs >= b.LimitRuns {
		return true
	}
	if b.LimitCostMicros > 0 && usage.CostMicros >= b.LimitCostMicros {
		return true
	}
	return false
}
