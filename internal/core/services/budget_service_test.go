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

func TestBudgetServiceUsageForWorkspaceAggregatesLedgerEvents(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 1, 12, 45, 0, 0, time.UTC)}
	svc := NewBudgetService(st, clock, &seqID{})

	workspaceTaskA := domain.Task{ID: "task_workspace_a", WorkspaceID: "workspace_a", AgentID: "agent_a", Title: "a", Prompt: "a", Status: domain.TaskStatusDone, CreatedAt: clock.t, UpdatedAt: clock.t}
	workspaceTaskB := domain.Task{ID: "task_workspace_b", WorkspaceID: "workspace_a", AgentID: "agent_b", Title: "b", Prompt: "b", Status: domain.TaskStatusDone, CreatedAt: clock.t, UpdatedAt: clock.t}
	otherWorkspaceTask := domain.Task{ID: "task_other_workspace", WorkspaceID: "workspace_b", AgentID: "agent_c", Title: "c", Prompt: "c", Status: domain.TaskStatusDone, CreatedAt: clock.t, UpdatedAt: clock.t}
	for _, task := range []domain.Task{workspaceTaskA, workspaceTaskB, otherWorkspaceTask} {
		if err := st.Tasks().Create(context.Background(), task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	for _, event := range []domain.UsageEvent{
		{ID: "usage_workspace_a", TaskID: workspaceTaskA.ID, InputTokens: 7, OutputTokens: 3, CachedTokens: 2, CreatedAt: clock.t},
		{ID: "usage_workspace_b", TaskID: workspaceTaskB.ID, InputTokens: 5, OutputTokens: 4, CachedTokens: 1, CreatedAt: clock.t},
		{ID: "usage_other_workspace", TaskID: otherWorkspaceTask.ID, InputTokens: 100, OutputTokens: 100, CachedTokens: 100, CreatedAt: clock.t},
	} {
		if err := st.Usage().Create(context.Background(), event); err != nil {
			t.Fatalf("create usage event %s: %v", event.ID, err)
		}
	}

	usage, err := svc.UsageForWorkspace(context.Background(), "workspace_a")
	if err != nil {
		t.Fatalf("usage for workspace: %v", err)
	}
	if usage.InputTokens != 12 || usage.OutputTokens != 7 || usage.CachedTokens != 3 || usage.TotalTokens != 22 || usage.Runs != 2 {
		t.Fatalf("unexpected workspace usage: %#v", usage)
	}
}

func TestBudgetServiceWorkspaceUsageIsStrictButHardStopIgnoresOrphanedUsage(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 1, 12, 50, 0, 0, time.UTC)}
	svc := NewBudgetService(st, clock, &seqID{})

	task := domain.Task{ID: "task_workspace_valid", WorkspaceID: "workspace_a", AgentID: "agent_a", Title: "valid", Prompt: "valid", Status: domain.TaskStatusDone, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := st.Usage().Create(context.Background(), domain.UsageEvent{ID: "usage_valid", TaskID: task.ID, AgentID: task.AgentID, InputTokens: 10, CreatedAt: clock.t}); err != nil {
		t.Fatalf("create valid usage: %v", err)
	}
	if err := st.Usage().Create(context.Background(), domain.UsageEvent{ID: "usage_orphan", TaskID: "missing_task", AgentID: task.AgentID, InputTokens: 999, CreatedAt: clock.t}); err != nil {
		t.Fatalf("create orphan usage: %v", err)
	}

	if _, err := svc.UsageForWorkspace(context.Background(), task.WorkspaceID); err == nil {
		t.Fatal("workspace usage should fail when the ledger references a missing task")
	}

	_, _, usage, err := svc.IsHardStopped(context.Background(), task.WorkspaceID, task.AgentID)
	if err != nil {
		t.Fatalf("hard stop should preserve tolerant handling of orphaned usage: %v", err)
	}
	if usage.TotalTokens != 10 {
		t.Fatalf("hard stop should ignore orphaned usage, got %#v", usage)
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
