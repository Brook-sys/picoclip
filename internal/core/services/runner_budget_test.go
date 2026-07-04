package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestRunnerBlocksTaskWhenBudgetHardStopExceeded(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 1, 13, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}

	agent := domain.Agent{ID: "agent_1", Name: "agent", Type: "noop", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	budgetSvc := NewBudgetService(st, clock, idgen)
	budget, err := budgetSvc.Create(context.Background(), domain.Budget{Scope: domain.BudgetScopeAgent, AgentID: agent.ID, LimitTokens: 10, HardStop: true})
	if err != nil {
		t.Fatalf("create budget: %v", err)
	}
	if err := st.Usage().Create(context.Background(), domain.UsageEvent{ID: "usage_1", AgentID: agent.ID, InputTokens: 10, CreatedAt: clock.t}); err != nil {
		t.Fatalf("create usage: %v", err)
	}

	task := domain.Task{ID: "task_1", AgentID: agent.ID, Title: "t", Prompt: "do", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	runner := NewRunner(st, clock, idgen, bus, nil, nil, logger, Config{})
	runner.Run(context.Background(), task)

	gotTask, _ := st.Tasks().Get(context.Background(), task.ID)
	if gotTask.Status != domain.TaskStatusBlocked || gotTask.NeedsRun {
		t.Fatalf("expected blocked task with needs_run=false, got status=%s needs=%v", gotTask.Status, gotTask.NeedsRun)
	}
	runs, _ := st.Runs().ListByTask(context.Background(), task.ID)
	if len(runs) != 0 {
		t.Fatalf("expected no run created, got %d", len(runs))
	}
	messages, _ := st.Messages().ListByTask(context.Background(), task.ID)
	if len(messages) != 1 || messages[0].Role != domain.MessageRoleSystem {
		t.Fatalf("expected system budget message, got %#v", messages)
	}
	if messages[0].Body == "" || budget.ID == "" {
		t.Fatalf("expected budget message and id")
	}
}

func TestRunnerRecordTokenUsageCreatesLedgerEventOnce(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 1, 14, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	runner := NewRunner(st, clock, idgen, noopBus{}, nil, nil, testLogger{}, Config{})

	agent := domain.Agent{ID: "agent_usage", Name: "usage", Type: "noop", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_usage", AgentID: agent.ID, Title: "usage", Prompt: "do", Status: domain.TaskStatusTodo, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	finished := clock.t.Add(time.Second)
	run := domain.Run{ID: "run_usage", TaskID: task.ID, AgentID: agent.ID, DriverType: "noop", InputTokens: 12, OutputTokens: 8, TotalTokens: 20, FinishedAt: &finished}

	runner.recordTokenUsage(context.Background(), run)
	runner.recordTokenUsage(context.Background(), run)

	events, err := st.Usage().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one usage event, got %d", len(events))
	}
	if events[0].RunID != run.ID || events[0].InputTokens != 12 || events[0].OutputTokens != 8 {
		t.Fatalf("unexpected usage event: %#v", events[0])
	}
	gotTask, _ := st.Tasks().Get(context.Background(), task.ID)
	if gotTask.TotalTokens != 20 {
		t.Fatalf("expected task total 20, got %d", gotTask.TotalTokens)
	}
	gotAgent, _ := st.Agents().Get(context.Background(), agent.ID)
	if gotAgent.TotalTokens != 20 {
		t.Fatalf("expected agent total 20, got %d", gotAgent.TotalTokens)
	}
}
