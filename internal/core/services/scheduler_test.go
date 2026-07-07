package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestSchedulerReconcilesDueWakeupBeforeDispatch(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 7, 21, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}

	agent := domain.Agent{ID: "agent_scheduler_due", Name: "scheduler", Type: "noop", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task := domain.Task{ID: "task_scheduler_due", AgentID: agent.ID, Title: "due wakeup", Prompt: "run", Status: domain.TaskStatusBlocked, NeedsRun: false, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := st.Wakeups().Create(context.Background(), domain.WakeupRequest{ID: "wakeup_due", TaskID: task.ID, AgentID: agent.ID, Reason: domain.WakeupReasonRetry, Status: domain.WakeupStatusPending, Priority: 1, DueAt: clock.t.Add(-time.Second), CreatedAt: clock.t, UpdatedAt: clock.t}); err != nil {
		t.Fatalf("create due wakeup: %v", err)
	}

	runner := NewRunner(st, clock, idgen, bus, nil, NoopMemoryProvider{}, logger, Config{TaskTimeout: time.Minute})
	dispatcher := NewDispatcher(st, runner, logger, 1)
	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	scheduler := NewScheduler(time.Hour, dispatcher, reconciler, logger)

	scheduler.runOnce(context.Background())
	dispatcher.Wait()

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.TaskStatusTodo || got.NeedsRun || got.CheckoutRunID != "" {
		t.Fatalf("expected due wakeup to be reconciled and dispatched in same scheduler tick, got %#v", got)
	}
	runs, err := st.Runs().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != domain.RunStatusSucceeded {
		t.Fatalf("expected one successful run after scheduler tick, got %#v", runs)
	}
}

func TestSchedulerDoesNotDispatchTaskWaitingForFutureWakeup(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 7, 21, 15, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}

	agent := domain.Agent{ID: "agent_scheduler_future", Name: "scheduler", Type: "noop", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task := domain.Task{ID: "task_scheduler_future", AgentID: agent.ID, Title: "future wakeup", Prompt: "wait", Status: domain.TaskStatusBlocked, NeedsRun: false, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := st.Wakeups().Create(context.Background(), domain.WakeupRequest{ID: "wakeup_future", TaskID: task.ID, AgentID: agent.ID, Reason: domain.WakeupReasonRetry, Status: domain.WakeupStatusPending, Priority: 1, DueAt: clock.t.Add(time.Minute), CreatedAt: clock.t, UpdatedAt: clock.t}); err != nil {
		t.Fatalf("create future wakeup: %v", err)
	}

	runner := NewRunner(st, clock, idgen, bus, nil, NoopMemoryProvider{}, logger, Config{TaskTimeout: time.Minute})
	dispatcher := NewDispatcher(st, runner, logger, 1)
	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	scheduler := NewScheduler(time.Hour, dispatcher, reconciler, logger)

	scheduler.runOnce(context.Background())
	dispatcher.Wait()

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.TaskStatusBlocked || got.NeedsRun || got.CheckoutRunID != "" {
		t.Fatalf("expected future wakeup not to be dispatched early, got %#v", got)
	}
	runs, err := st.Runs().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no run before wakeup is due, got %#v", runs)
	}
}
