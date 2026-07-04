package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestReconcilerProcessesWakeupsAndWakesTasks(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 15, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	logger := testLogger{}
	svc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	_ = st.Agents().Create(context.Background(), agent)

	task, _ := svc.Create(context.Background(), agent.ID, "t", "do")
	locked, _ := svc.Checkout(context.Background(), task.ID, agent.ID, "run_1", nil)

	// cria wakeup manual
	wakeupSvc := NewWakeupService(st, clock, idgen)
	_, _ = wakeupSvc.Create(context.Background(), CreateWakeupInput{
		AgentID:  agent.ID,
		TaskID:   task.ID,
		Reason:   domain.WakeupReasonManual,
		Priority: 5,
	})

	// marca task como locked (já está) e expira lock
	expired := clock.t.Add(-time.Minute)
	locked.ExecutionLockedAt = &expired
	locked.LockExpiresAt = &expired
	_ = st.Tasks().Update(context.Background(), locked)

	reconciler := NewReconciler(st, clock, bus, idgen, logger)
	reconciler.Reconcile(context.Background())

	got, _ := st.Tasks().Get(context.Background(), task.ID)
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" {
		t.Fatal("lock not cleared")
	}
	if !got.NeedsRun || got.Status != domain.TaskStatusTodo {
		t.Fatalf("expected todo/needs_run, got status=%s needs_run=%v", got.Status, got.NeedsRun)
	}
}
