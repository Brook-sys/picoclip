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

type completionAuditStub struct {
	decision ports.CompletionAuditDecision
	err      error
	calls    int
}

func (s *completionAuditStub) AuditCompletion(_ context.Context, _ ports.CompletionAuditRequest) (ports.CompletionAuditDecision, error) {
	s.calls++
	return s.decision, s.err
}

func newAuditedTaskService(t *testing.T, auditor ports.CompletionAuditor) (*TaskService, *memory.Storage, *completionAuditStub, domain.Task) {
	t.Helper()
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)}
	stub := auditor.(*completionAuditStub)
	svc := NewTaskService(st, clock, &seqID{}, noopBus{})
	svc.SetCompletionAuditor(stub)
	if err := st.Settings().Set(context.Background(), completionAuditSettingsKey, `{"mode":"enforce","timeout_seconds":1}`); err != nil {
		t.Fatal(err)
	}
	locked := clock.t.Add(-time.Minute)
	expires := clock.t.Add(time.Minute)
	task := domain.Task{ID: "task_audit", AgentID: "agent_worker", Title: "work", Prompt: "do it", Status: domain.TaskStatusInProgress, NeedsRun: false, CheckoutRunID: "run_audit", CheckedOutByAgentID: "agent_worker", ExecutionLockedAt: &locked, LockExpiresAt: &expires, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	return svc, st, stub, task
}

func TestCompletionAuditApproveCompletesTask(t *testing.T) {
	stub := &completionAuditStub{decision: ports.CompletionAuditDecision{Outcome: ports.CompletionAuditApprove, Summary: "evidence is sufficient"}}
	svc, st, gotStub, task := newAuditedTaskService(t, stub)

	got, err := svc.UpdateStatus(context.Background(), task.ID, domain.TaskStatusDone, "finished", "agent_worker")
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if got.Status != domain.TaskStatusDone || got.NeedsRun || got.CompletedAt == nil {
		t.Fatalf("unexpected completed task: %#v", got)
	}
	if gotStub.calls != 1 {
		t.Fatalf("auditor calls = %d, want 1", gotStub.calls)
	}
	events, err := st.Events().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEvent(events, domain.EventCompletionAuditApproved) || !hasEvent(events, domain.EventTaskCompleted) {
		t.Fatalf("missing approval/completion events: %#v", events)
	}
}

func TestCompletionAuditRejectReturnsTaskToInProgressWithoutUnlocking(t *testing.T) {
	stub := &completionAuditStub{decision: ports.CompletionAuditDecision{Outcome: ports.CompletionAuditReject, Summary: "missing verification", Findings: []ports.CompletionAuditFinding{{Code: "verification_missing", Severity: "error", Message: "add tests"}}}}
	svc, st, _, task := newAuditedTaskService(t, stub)

	_, err := svc.UpdateStatus(context.Background(), task.ID, domain.TaskStatusDone, "finished", "agent_worker")
	if !errors.Is(err, domain.ErrCompletionAuditRejected) {
		t.Fatalf("error = %v, want semantic rejection", err)
	}
	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusInProgress || got.NeedsRun {
		t.Fatalf("rejected task must stay non-runnable in_progress: %#v", got)
	}
	if got.CheckoutRunID != task.CheckoutRunID || got.CheckedOutByAgentID != task.CheckedOutByAgentID {
		t.Fatalf("rejection released checkout: %#v", got)
	}
	messages, err := st.Messages().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Role != domain.MessageRoleSystem {
		t.Fatalf("expected durable system feedback, got %#v", messages)
	}
}

func TestCompletionAuditRejectWithoutCheckoutMakesReworkRunnable(t *testing.T) {
	stub := &completionAuditStub{decision: ports.CompletionAuditDecision{Outcome: ports.CompletionAuditReject, Summary: "needs rework"}}
	svc, st, _, task := newAuditedTaskService(t, stub)
	task.CheckoutRunID, task.CheckedOutByAgentID = "", ""
	task.ExecutionLockedAt, task.LockExpiresAt = nil, nil
	if err := st.Tasks().Update(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	_, err := svc.UpdateStatus(context.Background(), task.ID, domain.TaskStatusDone, "finished", "agent_worker")
	if !errors.Is(err, domain.ErrCompletionAuditRejected) {
		t.Fatalf("error = %v, want semantic rejection", err)
	}
	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusInProgress || !got.NeedsRun {
		t.Fatalf("unlocked rejection must be runnable rework: %#v", got)
	}
}

func TestCompletionAuditTimeoutFailsClosedAndIsObservable(t *testing.T) {
	stub := &completionAuditStub{err: context.DeadlineExceeded}
	svc, st, _, task := newAuditedTaskService(t, stub)

	_, err := svc.UpdateStatus(context.Background(), task.ID, domain.TaskStatusDone, "finished", "agent_worker")
	if !errors.Is(err, domain.ErrCompletionAuditTimeout) {
		t.Fatalf("error = %v, want timeout", err)
	}
	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusInProgress || got.CheckoutRunID == "" || got.NeedsRun {
		t.Fatalf("timeout mutated lifecycle: %#v", got)
	}
	events, err := st.Events().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEvent(events, domain.EventCompletionAuditTimeout) {
		t.Fatalf("missing timeout event: %#v", events)
	}
	audits, err := st.CompletionAudits().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || audits[0].Outcome != domain.CompletionAuditTimeout {
		t.Fatalf("timeout audit was not finalized: %#v", audits)
	}
}

func TestCompletionAuditErrorFailsClosedAndIsObservable(t *testing.T) {
	stub := &completionAuditStub{err: errors.New("runtime unavailable")}
	svc, st, _, task := newAuditedTaskService(t, stub)
	_, err := svc.UpdateStatus(context.Background(), task.ID, domain.TaskStatusDone, "finished", "agent_worker")
	if !errors.Is(err, domain.ErrCompletionAuditFailed) {
		t.Fatalf("error = %v, want unavailable", err)
	}
	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusInProgress || got.CheckoutRunID == "" || got.NeedsRun {
		t.Fatalf("error mutated lifecycle: %#v", got)
	}
	audits, err := st.CompletionAudits().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || audits[0].Outcome != domain.CompletionAuditError {
		t.Fatalf("error audit was not finalized: %#v", audits)
	}
}

func TestCompletionAuditInvalidConfigurationFailsClosedAndIsObservable(t *testing.T) {
	stub := &completionAuditStub{}
	svc, st, _, task := newAuditedTaskService(t, stub)
	if err := st.Settings().Set(context.Background(), completionAuditSettingsKey, `{invalid`); err != nil {
		t.Fatal(err)
	}

	_, err := svc.UpdateStatus(context.Background(), task.ID, domain.TaskStatusDone, "finished", "agent_worker")
	if !errors.Is(err, domain.ErrCompletionAuditFailed) {
		t.Fatalf("error = %v, want unavailable", err)
	}
	if stub.calls != 0 {
		t.Fatalf("auditor calls = %d, want 0 for invalid config", stub.calls)
	}
	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.TaskStatusInProgress || got.CheckoutRunID == "" || got.NeedsRun {
		t.Fatalf("invalid config mutated lifecycle: %#v", got)
	}
	audits, err := st.CompletionAudits().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || audits[0].Outcome != domain.CompletionAuditError {
		t.Fatalf("invalid config audit was not persisted: %#v", audits)
	}
	events, err := st.Events().ListByTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasEvent(events, domain.EventCompletionAuditError) {
		t.Fatalf("missing invalid-config error event: %#v", events)
	}
}

func TestCompletionAuditDisabledPreservesExistingCompletionFlow(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)}
	svc := NewTaskService(st, clock, &seqID{}, noopBus{})
	task := domain.Task{ID: "task_disabled", Status: domain.TaskStatusInProgress, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	got, err := svc.UpdateStatus(context.Background(), task.ID, domain.TaskStatusDone, "finished", "")
	if err != nil {
		t.Fatalf("disabled audit should preserve completion: %v", err)
	}
	if got.Status != domain.TaskStatusDone {
		t.Fatalf("status = %s", got.Status)
	}
}

func hasEvent(events []domain.Event, want domain.EventType) bool {
	for _, event := range events {
		if event.Type == want {
			return true
		}
	}
	return false
}
