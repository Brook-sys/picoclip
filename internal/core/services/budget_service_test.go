package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestBudgetServiceHardStopsAgent(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	svc := NewBudgetService(st, clock, idgen)

	agent := domain.Agent{ID: "agent_1", Name: "agent", Type: "noop", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	budget, err := svc.Create(context.Background(), domain.Budget{Scope: domain.BudgetScopeAgent, AgentID: agent.ID, LimitTokens: 100, HardStop: true})
	if err != nil {
		t.Fatalf("create budget: %v", err)
	}
	if budget.ID == "" || !budget.Enabled {
		t.Fatalf("expected budget id and enabled flag, got %#v", budget)
	}

	if err := st.Usage().Create(context.Background(), domain.UsageEvent{ID: "usage_1", AgentID: agent.ID, InputTokens: 60, OutputTokens: 40, CreatedAt: clock.t}); err != nil {
		t.Fatalf("create usage: %v", err)
	}

	stopped, stoppedBudget, usage, err := svc.IsHardStopped(context.Background(), "", agent.ID)
	if err != nil {
		t.Fatalf("hard stop check: %v", err)
	}
	if !stopped || stoppedBudget.ID != budget.ID || usage.TotalTokens != 100 {
		t.Fatalf("expected hard stop at 100 tokens, stopped=%v budget=%s usage=%d", stopped, stoppedBudget.ID, usage.TotalTokens)
	}
}

func TestBudgetServiceUsesBudgetSpecificScope(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 1, 12, 30, 0, 0, time.UTC)}
	idgen := &seqID{}
	svc := NewBudgetService(st, clock, idgen)

	taskA := domain.Task{ID: "task_a", WorkspaceID: "workspace_a", AgentID: "agent_a", Title: "a", Prompt: "a", Status: domain.TaskStatusDone, CreatedAt: clock.t, UpdatedAt: clock.t}
	taskB := domain.Task{ID: "task_b", WorkspaceID: "workspace_b", AgentID: "agent_b", Title: "b", Prompt: "b", Status: domain.TaskStatusDone, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), taskA); err != nil {
		t.Fatalf("create task a: %v", err)
	}
	if err := st.Tasks().Create(context.Background(), taskB); err != nil {
		t.Fatalf("create task b: %v", err)
	}
	_ = st.Usage().Create(context.Background(), domain.UsageEvent{ID: "usage_a", TaskID: taskA.ID, AgentID: taskA.AgentID, InputTokens: 90, CreatedAt: clock.t})
	_ = st.Usage().Create(context.Background(), domain.UsageEvent{ID: "usage_b", TaskID: taskB.ID, AgentID: taskB.AgentID, InputTokens: 90, CreatedAt: clock.t})

	budget, err := svc.Create(context.Background(), domain.Budget{Scope: domain.BudgetScopeWorkspace, WorkspaceID: taskA.WorkspaceID, LimitTokens: 100, HardStop: true})
	if err != nil {
		t.Fatalf("create budget: %v", err)
	}

	stopped, stoppedBudget, usage, err := svc.IsHardStopped(context.Background(), taskA.WorkspaceID, taskA.AgentID)
	if err != nil {
		t.Fatalf("hard stop check: %v", err)
	}
	if stopped || stoppedBudget.ID != "" || usage.TotalTokens != 90 {
		t.Fatalf("expected workspace budget not stopped at 90 tokens, stopped=%v budget=%s usage=%d", stopped, stoppedBudget.ID, usage.TotalTokens)
	}

	_ = st.Usage().Create(context.Background(), domain.UsageEvent{ID: "usage_a2", TaskID: taskA.ID, AgentID: taskA.AgentID, InputTokens: 10, CreatedAt: clock.t})
	stopped, stoppedBudget, usage, err = svc.IsHardStopped(context.Background(), taskA.WorkspaceID, taskA.AgentID)
	if err != nil {
		t.Fatalf("hard stop check after second usage: %v", err)
	}
	if !stopped || stoppedBudget.ID != budget.ID || usage.TotalTokens != 100 {
		t.Fatalf("expected workspace budget stopped at 100 tokens, stopped=%v budget=%s usage=%d", stopped, stoppedBudget.ID, usage.TotalTokens)
	}
}
