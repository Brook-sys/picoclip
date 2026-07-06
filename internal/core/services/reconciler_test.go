package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

type testLogger struct{}

func (testLogger) Debug(msg string, args ...any) {}
func (testLogger) Info(msg string, args ...any)  {}
func (testLogger) Warn(msg string, args ...any)  {}
func (testLogger) Error(msg string, args ...any) {}

func TestReconcilerRecoversStaleLocksBeforeDispatchCycle(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 13, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}
	svc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	task, err := svc.Create(context.Background(), agent.ID, "t", "do")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	locked, err := svc.Checkout(context.Background(), task.ID, agent.ID, "run_1", nil)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	expired := clock.t.Add(-time.Minute)
	locked.ExecutionLockedAt = &expired
	locked.LockExpiresAt = &expired
	if err := st.Tasks().Update(context.Background(), locked); err != nil {
		t.Fatalf("update locked task: %v", err)
	}

	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	reconciler.Reconcile(context.Background())

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" || got.ExecutionLockedAt != nil || got.LockExpiresAt != nil {
		t.Fatalf("expected lock cleared, got run=%q agent=%q locked_at=%v expires=%v", got.CheckoutRunID, got.CheckedOutByAgentID, got.ExecutionLockedAt, got.LockExpiresAt)
	}
	if got.Status != domain.TaskStatusTodo || !got.NeedsRun {
		t.Fatalf("expected recovered task todo/needs_run, got status=%s needs_run=%v", got.Status, got.NeedsRun)
	}
}

func TestReconcilerDetectsStalledRuns(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 14, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc := NewTaskService(st, clock, idgen, bus)
	task, err := svc.Create(context.Background(), agent.ID, "t", "do")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	_, err = svc.Checkout(context.Background(), task.ID, agent.ID, "run_1", nil)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	last := clock.t.Add(-3 * time.Minute)
	run := domain.Run{
		ID:           "run_1",
		TaskID:       task.ID,
		AgentID:      agent.ID,
		Status:       domain.RunStatusRunning,
		Attempt:      0,
		StallTimeout: 60,
		LastOutputAt: &last,
		StartedAt:    clock.t.Add(-5 * time.Minute),
	}
	if err := st.Runs().Create(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	reconciler.Reconcile(context.Background())

	gotRun, _ := st.Runs().Get(context.Background(), "run_1")
	if gotRun.Status != domain.RunStatusTimeout {
		t.Fatalf("expected run timeout, got %s", gotRun.Status)
	}

	gotTask, _ := st.Tasks().Get(context.Background(), task.ID)
	if gotTask.CheckoutRunID != "" || gotTask.NeedsRun {
		t.Fatalf("expected task unlocked and waiting for retry wakeup, got run=%q needs=%v", gotTask.CheckoutRunID, gotTask.NeedsRun)
	}

	wakeups, _ := st.Wakeups().ListPending(context.Background(), clock.t.Add(time.Hour), 10)
	if len(wakeups) != 1 || wakeups[0].Reason != domain.WakeupReasonRetry {
		t.Fatalf("expected 1 retry wakeup, got %d", len(wakeups))
	}
	events, _ := st.Events().ListByTask(context.Background(), task.ID)
	if !hasEventType(events, domain.EventRunTimeout) {
		t.Fatalf("expected run timeout event, got %#v", events)
	}
}

type recordingCanceler struct{ canceled []string }

func (r *recordingCanceler) CancelRun(ctx context.Context, run domain.Run) error {
	r.canceled = append(r.canceled, run.ID)
	return nil
}

func TestReconcilerSchedulesRetryWakeupWithExponentialBackoffMetadata(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 14, 30, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	svc := NewTaskService(st, clock, idgen, bus)
	task, err := svc.Create(context.Background(), agent.ID, "t", "do")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	locked, err := svc.Checkout(context.Background(), task.ID, agent.ID, "run_1", nil)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	locked.Attempts = 4
	locked.MaxAttempts = 10
	if err := st.Tasks().Update(context.Background(), locked); err != nil {
		t.Fatalf("update task attempts: %v", err)
	}

	last := clock.t.Add(-3 * time.Minute)
	run := domain.Run{
		ID:           "run_1",
		TaskID:       task.ID,
		AgentID:      agent.ID,
		Status:       domain.RunStatusRunning,
		Attempt:      4,
		StallTimeout: 60,
		LastOutputAt: &last,
		StartedAt:    clock.t.Add(-5 * time.Minute),
	}
	if err := st.Runs().Create(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	reconciler.Reconcile(context.Background())

	wakeups, _ := st.Wakeups().ListPending(context.Background(), clock.t.Add(time.Hour), 10)
	if len(wakeups) != 1 {
		t.Fatalf("expected 1 retry wakeup, got %d", len(wakeups))
	}
	wantDue := clock.t.Add(4 * time.Minute)
	if !wakeups[0].DueAt.Equal(wantDue) {
		t.Fatalf("retry due_at=%s want %s", wakeups[0].DueAt, wantDue)
	}
	if wakeups[0].Payload["previous_run_id"] != run.ID || wakeups[0].Payload["attempt"] != "4" || wakeups[0].Payload["backoff_seconds"] != "240" || wakeups[0].Payload["retryable"] != "true" {
		t.Fatalf("unexpected retry payload: %#v", wakeups[0].Payload)
	}
}

func TestStalledRunRetryWaitsForBackoffWakeupBeforeDispatch(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 14, 45, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}

	agent := domain.Agent{ID: "agt_retry", Name: "retry", Type: "noop", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc := NewTaskService(st, clock, idgen, bus)
	task, err := svc.Create(context.Background(), agent.ID, "retry later", "do")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	locked, err := svc.Checkout(context.Background(), task.ID, agent.ID, "run_retry", nil)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	locked.Attempts = 2
	locked.MaxAttempts = 5
	if err := st.Tasks().Update(context.Background(), locked); err != nil {
		t.Fatalf("update locked task: %v", err)
	}
	last := clock.t.Add(-3 * time.Minute)
	if err := st.Runs().Create(context.Background(), domain.Run{ID: "run_retry", TaskID: task.ID, AgentID: agent.ID, Status: domain.RunStatusRunning, Attempt: 2, LastOutputAt: &last, StallTimeout: 60, StartedAt: clock.t.Add(-5 * time.Minute)}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	reconciler.Reconcile(context.Background())

	beforeDispatch, _, err := st.Tasks().ClaimNextRunnable(context.Background(), clock.t, 30*time.Minute)
	if !errors.Is(err, domain.ErrNoPendingTasks) {
		t.Fatalf("expected retry to wait for wakeup due, claimed=%#v err=%v", beforeDispatch, err)
	}

	wakeupDue := clock.t.Add(1 * time.Minute)
	_, _, err = st.Tasks().ClaimNextRunnable(context.Background(), wakeupDue, 30*time.Minute)
	if !errors.Is(err, domain.ErrNoPendingTasks) {
		t.Fatalf("expected dispatcher alone not to bypass pending wakeup, err=%v", err)
	}

	processed, err := NewWakeupService(st, fixedClock{t: wakeupDue}, idgen).ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("process wakeup: %v", err)
	}
	if processed != 1 {
		t.Fatalf("processed wakeups=%d want 1", processed)
	}
	claimed, _, err := st.Tasks().ClaimNextRunnable(context.Background(), wakeupDue, 30*time.Minute)
	if err != nil {
		t.Fatalf("expected retry task claimable after wakeup due: %v", err)
	}
	if claimed.ID != task.ID {
		t.Fatalf("claimed task=%s want %s", claimed.ID, task.ID)
	}
}

func TestReconcilerRecoversRunningRunWithMissingTask(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}
	last := clock.t.Add(-5 * time.Minute)
	run := domain.Run{
		ID:           "run_orphan",
		TaskID:       "missing_task",
		AgentID:      "agt_1",
		Status:       domain.RunStatusRunning,
		LastOutputAt: &last,
		StallTimeout: 3600,
		StartedAt:    clock.t.Add(-10 * time.Minute),
	}
	if err := st.Runs().Create(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	canceler := &recordingCanceler{}
	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	reconciler.SetCanceler(canceler)

	reconciler.Reconcile(context.Background())

	gotRun, _ := st.Runs().Get(context.Background(), run.ID)
	if gotRun.Status != domain.RunStatusTimeout || gotRun.FinishedAt == nil {
		t.Fatalf("expected orphan run timeout, got status=%s finished=%v", gotRun.Status, gotRun.FinishedAt)
	}
	if gotRun.Error == "" {
		t.Fatal("expected orphan run to record recovery error")
	}
	events, _ := st.Events().ListByTask(context.Background(), run.TaskID)
	if !hasEventType(events, domain.EventRunRecovered) {
		t.Fatalf("expected run recovered event, got %#v", events)
	}
	if len(canceler.canceled) != 1 || canceler.canceled[0] != run.ID {
		t.Fatalf("expected canceler called for orphan run, got %#v", canceler.canceled)
	}
}
