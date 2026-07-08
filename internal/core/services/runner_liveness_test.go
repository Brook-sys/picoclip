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
	events, err := st.Events().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeEvent(t, events, domain.EventRuntimeTimeout, runs[0].ID, map[string]string{"runtime_id": "crush", "status": string(domain.RunStatusTimeout), "phase": "timeout_handled"})
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

func assertRuntimeEvent(t *testing.T, events []domain.Event, eventType domain.EventType, runID string, want map[string]string) {
	t.Helper()
	for _, event := range events {
		if event.Type != eventType || event.RunID != runID {
			continue
		}
		for key, value := range want {
			if event.Data[key] != value {
				t.Fatalf("event %s data[%s]=%q want %q in %#v", event.Type, key, event.Data[key], value, event)
			}
		}
		return
	}
	t.Fatalf("expected event %s for run %s, got %#v", eventType, runID, events)
}
