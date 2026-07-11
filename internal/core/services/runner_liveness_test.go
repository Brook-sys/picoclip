package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type livenessRuntimeAdapter struct {
	id domain.RuntimeID
}

func (a livenessRuntimeAdapter) ID() domain.RuntimeID                        { return a.id }
func (a livenessRuntimeAdapter) Name() string                                { return "liveness" }
func (a livenessRuntimeAdapter) Kind() domain.RuntimeKind                    { return "fake" }
func (a livenessRuntimeAdapter) SupportedInstallModes() []domain.InstallMode { return nil }
func (a livenessRuntimeAdapter) ListVersions(context.Context, int) ([]domain.RuntimeVersion, error) {
	return nil, nil
}
func (a livenessRuntimeAdapter) Install(context.Context, domain.InstallMode, string, string) (domain.RuntimeState, error) {
	return domain.RuntimeState{}, nil
}
func (a livenessRuntimeAdapter) Resolve(context.Context, domain.RuntimeState) error { return nil }
func (a livenessRuntimeAdapter) Health(context.Context, domain.RuntimeState) domain.RuntimeHealth {
	return domain.RuntimeHealth{Status: "ok"}
}
func (a livenessRuntimeAdapter) ReadConfig(context.Context, domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	return nil, nil
}
func (a livenessRuntimeAdapter) WriteConfig(context.Context, domain.RuntimeState, string, []byte) error {
	return nil
}
func (a livenessRuntimeAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	if input.OnStart != nil {
		input.OnStart(4242)
	}
	if input.OnOutput != nil {
		input.OnOutput([]byte("first chunk"), nil)
		input.OnOutput([]byte("second chunk"), []byte("stderr chunk"))
	}
	return ports.RuntimeExecutionResult{Output: "done"}, nil
}
func (a livenessRuntimeAdapter) Cancel(context.Context, domain.RuntimeState, domain.Run) error {
	return nil
}

func TestRunnerPersistsStructuredRuntimeLivenessEvents(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 3, 10, 0, 0, time.UTC)}
	idgen := &seqID{}
	runtimes := NewRuntimeManager(st, t.TempDir(), clock)
	runtimes.Register(livenessRuntimeAdapter{id: "crush"})
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Mode: domain.InstallModeExisting, Enabled: true, SettingsJSON: "{}", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	agent := domain.Agent{ID: "agent_liveness", Name: "agent", Type: "crush", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(ctx, agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_liveness", AgentID: agent.ID, Title: "liveness", Prompt: "run", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, runtimes, NoopMemoryProvider{}, testLogger{}, Config{TaskTimeout: time.Minute})
	runner.Run(ctx, task)

	runs, err := st.Runs().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run, got %#v", runs)
	}
	events, err := st.Events().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeEvent(t, events, domain.EventRuntimeStarted, runs[0].ID, map[string]string{"runtime_id": "crush", "phase": "started"})
	assertRuntimeEvent(t, events, domain.EventRuntimeProcessStarted, runs[0].ID, map[string]string{"pid": "4242", "phase": "process_started"})
	assertRuntimeEvent(t, events, domain.EventRuntimeHeartbeat, runs[0].ID, map[string]string{"stdout_bytes": "11", "stderr_bytes": "0", "phase": "output_heartbeat"})
	assertRuntimeEvent(t, events, domain.EventRuntimeCompleted, runs[0].ID, map[string]string{"runtime_id": "crush", "status": string(domain.RunStatusSucceeded), "phase": "completed"})
}

func TestRunnerPersistsRuntimeTimeoutEvent(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 3, 20, 0, 0, time.UTC)}
	idgen := &seqID{}
	runtimes := NewRuntimeManager(st, t.TempDir(), clock)
	runtimes.Register(fakeRuntimeAdapter{id: "crush", err: context.DeadlineExceeded})
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Mode: domain.InstallModeExisting, Enabled: true, SettingsJSON: "{}", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	agent := domain.Agent{ID: "agent_timeout", Name: "agent", Type: "crush", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(ctx, agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_timeout", AgentID: agent.ID, Title: "timeout", Prompt: "run", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, runtimes, NoopMemoryProvider{}, testLogger{}, Config{TaskTimeout: time.Nanosecond})
	runner.Run(ctx, task)

	runs, err := st.Runs().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domain.RunStatusTimeout {
		t.Fatalf("expected timeout run, got %#v", runs)
	}
	gotTask, err := st.Tasks().Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTask.Status != domain.TaskStatusInProgress || gotTask.NeedsRun {
		t.Fatalf("expected timed-out one-shot task to wait for retry wakeup, got status=%s needs_run=%v", gotTask.Status, gotTask.NeedsRun)
	}
	wakeups, err := st.Wakeups().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected one retry wakeup, got %#v", wakeups)
	}
	if wakeups[0].Reason != domain.WakeupReasonRetry || wakeups[0].Payload["previous_run_id"] != runs[0].ID || wakeups[0].Payload["reason"] != "runtime_timeout" {
		t.Fatalf("expected runtime timeout retry wakeup for run %s, got %#v", runs[0].ID, wakeups[0])
	}
	if got := int(wakeups[0].DueAt.Sub(clock.t).Seconds()); got != 30 {
		t.Fatalf("expected first retry due after 30s, got %ds", got)
	}
	events, err := st.Events().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeEvent(t, events, domain.EventRuntimeTimeout, runs[0].ID, map[string]string{"runtime_id": "crush", "status": string(domain.RunStatusTimeout), "phase": "timeout_handled", "retryable": "true", "reason": "runtime_timeout"})
	assertEventData(t, events, domain.EventRetryScheduled, runs[0].ID, map[string]string{"retryable": "true", "classification": "retryable", "reason": "runtime_timeout", "backoff_seconds": "30"})
}

func TestRunnerClassifiesRuntimeUnavailableAsNonRetryable(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 3, 25, 0, 0, time.UTC)}
	idgen := &seqID{}
	runtimes := NewRuntimeManager(st, t.TempDir(), clock)
	agent := domain.Agent{ID: "agent_missing_runtime", Name: "agent", Type: "crush", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(ctx, agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_missing_runtime", AgentID: agent.ID, Title: "missing runtime", Prompt: "run", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, runtimes, NoopMemoryProvider{}, testLogger{}, Config{TaskTimeout: time.Minute, MaxAttempts: 3})
	runner.Run(ctx, task)

	gotTask, err := st.Tasks().Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTask.Status != domain.TaskStatusBlocked || gotTask.NeedsRun {
		t.Fatalf("expected non-retryable failure to block task without retry, got status=%s needs_run=%v", gotTask.Status, gotTask.NeedsRun)
	}
	events, err := st.Events().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEventData(t, events, domain.EventDriverMissing, "", map[string]string{"retryable": "false", "classification": "non_retryable", "reason": "runtime_unavailable"})
	assertEventData(t, events, domain.EventTaskFailed, "", map[string]string{"retryable": "false", "classification": "non_retryable", "reason": "runtime_unavailable"})
}

func TestRunnerRateLimitOneShot(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 3, 20, 0, 0, time.UTC)}
	idgen := &seqID{}
	runtimes := NewRuntimeManager(st, t.TempDir(), clock)
	// Return a rate limit error (429)
	runtimes.Register(fakeRuntimeAdapter{id: "crush", err: errors.New("rate limit: 429 too many requests")})
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Mode: domain.InstallModeExisting, Enabled: true, SettingsJSON: "{}", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	agent := domain.Agent{ID: "agent_ratelimit", Name: "agent", Type: "crush", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(ctx, agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_ratelimit", AgentID: agent.ID, Title: "ratelimit one shot", Prompt: "run", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, runtimes, NoopMemoryProvider{}, testLogger{}, Config{TaskTimeout: time.Minute, MaxAttempts: 3})
	runner.Run(ctx, task)

	runs, err := st.Runs().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domain.RunStatusFailed {
		t.Fatalf("expected failed run, got %#v", runs)
	}
	gotTask, err := st.Tasks().Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTask.Status != domain.TaskStatusInProgress || gotTask.NeedsRun {
		t.Fatalf("expected one-shot task to wait for retry wakeup, got status=%s needs_run=%v", gotTask.Status, gotTask.NeedsRun)
	}
	wakeups, err := st.Wakeups().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wakeups) != 1 {
		t.Fatalf("expected one retry wakeup, got %#v", wakeups)
	}
	if wakeups[0].Reason != domain.WakeupReasonRetry || wakeups[0].Payload["previous_run_id"] != runs[0].ID || wakeups[0].Payload["reason"] != "rate_limit" {
		t.Fatalf("expected rate limit retry wakeup for run %s, got %#v", runs[0].ID, wakeups[0])
	}
	if got := int(wakeups[0].DueAt.Sub(clock.t).Seconds()); got != 30 {
		t.Fatalf("expected first retry due after 30s, got %ds", got)
	}
	events, err := st.Events().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEventData(t, events, domain.EventRetryScheduled, runs[0].ID, map[string]string{"retryable": "true", "classification": "retryable", "reason": "rate_limit", "backoff_seconds": "30"})
}

func TestRunnerRateLimitContinuous(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 3, 20, 0, 0, time.UTC)}
	idgen := &seqID{}
	runtimes := NewRuntimeManager(st, t.TempDir(), clock)
	// Return a rate limit error (429)
	runtimes.Register(fakeRuntimeAdapter{id: "crush", err: errors.New("429 too many requests")})
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Mode: domain.InstallModeExisting, Enabled: true, SettingsJSON: "{}", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	agent := domain.Agent{ID: "agent_ratelimit_cont", Name: "agent", Type: "crush", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(ctx, agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{
		ID:        "task_ratelimit_cont",
		AgentID:   agent.ID,
		Title:     "ratelimit continuous",
		Prompt:    "run",
		Status:    domain.TaskStatusTodo,
		Mode:      domain.TaskModeContinuous,
		NeedsRun:  true,
		CreatedAt: clock.t,
		UpdatedAt: clock.t,
	}
	if err := st.Tasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, runtimes, NoopMemoryProvider{}, testLogger{}, Config{TaskTimeout: time.Minute, MaxAttempts: 3})
	runner.Run(ctx, task)

	runs, err := st.Runs().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != domain.RunStatusFailed {
		t.Fatalf("expected failed run, got %#v", runs)
	}
	gotTask, err := st.Tasks().Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTask.Status != domain.TaskStatusWaitingNextCycle || gotTask.NeedsRun {
		t.Fatalf("expected continuous task to remain in waiting next cycle, got status=%s needs_run=%v", gotTask.Status, gotTask.NeedsRun)
	}
	if gotTask.LoopNextRunAt == nil {
		t.Fatal("expected LoopNextRunAt to be set")
	}
	// First retry backoff is 30s
	wantNext := clock.t.Add(30 * time.Second)
	if !gotTask.LoopNextRunAt.Equal(wantNext) {
		t.Fatalf("expected LoopNextRunAt = %v, got %v", wantNext, gotTask.LoopNextRunAt)
	}

	// Verify retry.scheduled event was recorded for continuous task backoff
	events, err := st.Events().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEventData(t, events, domain.EventRetryScheduled, runs[0].ID, map[string]string{"retryable": "true", "classification": "retryable", "reason": "rate_limit", "backoff_seconds": "30"})

	// Reset agent response to success
	runtimes.Register(fakeRuntimeAdapter{id: "crush", out: "success"})

	// Prepare for next loop run: wake task and clear LoopNextRunAt
	gotTask.Status = domain.TaskStatusTodo
	gotTask.NeedsRun = true
	gotTask.LoopNextRunAt = nil
	if err := st.Tasks().Update(ctx, gotTask); err != nil {
		t.Fatal(err)
	}

	// Run again (should succeed)
	runner.Run(ctx, gotTask)

	// Attempts should be reset to 0
	gotTask2, err := st.Tasks().Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotTask2.Attempts != 0 {
		t.Fatalf("expected attempts to reset to 0 on successful continuous task run, got %d", gotTask2.Attempts)
	}
}

func TestReconcilerPersistsRuntimeCancellationEventsForStalledRun(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 3, 30, 0, 0, time.UTC)}
	idgen := &seqID{}
	agent := domain.Agent{ID: "agent_stall", Name: "agent", Type: "crush", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "task_stall", AgentID: agent.ID, Title: "stall", Prompt: "run", Status: domain.TaskStatusInProgress, CheckoutRunID: "run_stall", CheckedOutByAgentID: agent.ID, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	last := clock.t.Add(-3 * time.Minute)
	run := domain.Run{ID: "run_stall", TaskID: task.ID, AgentID: agent.ID, DriverType: "crush", Status: domain.RunStatusRunning, Attempt: 1, LastOutputAt: &last, StallTimeout: 60, StartedAt: clock.t.Add(-5 * time.Minute)}
	if err := st.Runs().Create(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	reconciler := NewReconciler(st, clock, noopBus{}, idgen, testLogger{})
	reconciler.SetCanceler(&recordingRunCanceler{err: errors.New("cancel failed")})

	reconciler.Reconcile(context.Background())

	events, err := st.Events().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeEvent(t, events, domain.EventRuntimeStalled, run.ID, map[string]string{"runtime_id": "crush", "phase": "stall_detected"})
	assertRuntimeEvent(t, events, domain.EventRuntimeCancelRequested, run.ID, map[string]string{"runtime_id": "crush", "phase": "cancel_requested", "reason": "run_stalled"})
	assertRuntimeEvent(t, events, domain.EventRuntimeCancelFailed, run.ID, map[string]string{"runtime_id": "crush", "phase": "cancel_failed"})
}

func TestRunnerRateLimitContinuousDoesNotFailOnMaxAttempts(t *testing.T) {
	ctx := context.Background()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 3, 20, 0, 0, time.UTC)}
	idgen := &seqID{}
	runtimes := NewRuntimeManager(st, t.TempDir(), clock)
	// Return a rate limit error (429)
	runtimes.Register(fakeRuntimeAdapter{id: "crush", err: errors.New("429 too many requests")})
	if err := st.Runtimes().Save(ctx, domain.RuntimeState{ID: "runtime_crush", RuntimeID: "crush", Mode: domain.InstallModeExisting, Enabled: true, SettingsJSON: "{}", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	agent := domain.Agent{ID: "agent_ratelimit_cont_max", Name: "agent", Type: "crush", Enabled: true, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(ctx, agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{
		ID:          "task_ratelimit_cont_max",
		AgentID:     agent.ID,
		Title:       "ratelimit continuous max",
		Prompt:      "run",
		Status:      domain.TaskStatusTodo,
		Mode:        domain.TaskModeContinuous,
		NeedsRun:    true,
		MaxAttempts: 3,
		CreatedAt:   clock.t,
		UpdatedAt:   clock.t,
	}
	if err := st.Tasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(st, clock, idgen, noopBus{}, runtimes, NoopMemoryProvider{}, testLogger{}, Config{TaskTimeout: time.Minute, MaxAttempts: 3})

	// Run 4 times (each run should schedule a retry/backoff, and because it's a rate limit error,
	// it should NEVER block the task even if attempts (4) > MaxAttempts (3))
	for i := 1; i <= 4; i++ {
		gotTask, err := st.Tasks().Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		// Wake the task for the next run
		gotTask.Status = domain.TaskStatusTodo
		gotTask.NeedsRun = true
		gotTask.LoopNextRunAt = nil
		if err := st.Tasks().Update(ctx, gotTask); err != nil {
			t.Fatal(err)
		}

		runner.Run(ctx, gotTask)

		// Verify task status is waiting next cycle, NOT blocked
		afterRunTask, err := st.Tasks().Get(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if afterRunTask.Status != domain.TaskStatusWaitingNextCycle {
			t.Fatalf("run %d: expected task to remain waiting_next_cycle, got status=%s", i, afterRunTask.Status)
		}
		if afterRunTask.Attempts != i {
			t.Fatalf("run %d: expected task attempts to be %d, got %d", i, i, afterRunTask.Attempts)
		}
	}
}

func assertRuntimeEvent(t *testing.T, events []domain.Event, eventType domain.EventType, runID string, want map[string]string) {
	t.Helper()
	assertEventData(t, events, eventType, runID, want)
}

func assertEventData(t *testing.T, events []domain.Event, eventType domain.EventType, runID string, want map[string]string) {
	t.Helper()
	var candidates []domain.Event
	for _, event := range events {
		if event.Type != eventType || event.RunID != runID {
			continue
		}
		candidates = append(candidates, event)
		matched := true
		for key, value := range want {
			if event.Data[key] != value {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("expected event %s for run %s with data %#v, candidates=%#v events=%#v", eventType, runID, want, candidates, events)
}
