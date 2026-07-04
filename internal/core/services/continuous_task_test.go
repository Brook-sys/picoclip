package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestRunnerSchedulesNextContinuousCycle(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	agent := domain.Agent{ID: "agent_cont", Name: "agent", Type: "noop", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_cont", AgentID: agent.ID, Title: "continuous", Prompt: "do", Status: domain.TaskStatusTodo, Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60, MaxAttempts: 0, NeedsRun: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, nil, NoopMemoryProvider{}, testLogger{}, Config{})
	runner.Run(context.Background(), task)

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusWaitingNextCycle || got.NeedsRun {
		t.Fatalf("expected waiting next cycle without needs_run, got status=%s needs=%v", got.Status, got.NeedsRun)
	}
	if got.LoopRunCount != 1 {
		t.Fatalf("expected loop count 1, got %d", got.LoopRunCount)
	}
	if got.LoopNextRunAt == nil || !got.LoopNextRunAt.Equal(clock.t.Add(60*time.Second)) {
		t.Fatalf("expected next run at +60s, got %v", got.LoopNextRunAt)
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" {
		t.Fatalf("expected checkout cleared, got run=%q agent=%q", got.CheckoutRunID, got.CheckedOutByAgentID)
	}
}

func TestReconcilerActivatesDueContinuousTask(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 4, 11, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	next := clock.t.Add(-time.Second)
	task := domain.Task{ID: "task_due", AgentID: "agent_due", Title: "continuous", Prompt: "do", Status: domain.TaskStatusWaitingNextCycle, Mode: domain.TaskModeContinuous, LoopDelaySeconds: 60, LoopRunCount: 1, LoopNextRunAt: &next, NeedsRun: false, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	reconciler := NewReconciler(st, clock, noopBus{}, idgen, testLogger{})
	reconciler.Reconcile(context.Background())

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusTodo || !got.NeedsRun {
		t.Fatalf("expected todo/needs_run, got status=%s needs=%v", got.Status, got.NeedsRun)
	}
	if got.LoopNextRunAt != nil {
		t.Fatalf("expected next run cleared, got %v", got.LoopNextRunAt)
	}
}
