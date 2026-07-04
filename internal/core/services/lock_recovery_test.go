package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

type seqID struct {
	prefix string
	seq    int
}

func (s *seqID) NewID(prefix string) string {
	s.seq++
	return prefix + "_seq" + itoa(s.seq)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for i > 0 {
		digits = append(digits, byte('0'+i%10))
		i /= 10
	}
	for l, r := 0, len(digits)-1; l < r; l, r = l+1, r-1 {
		digits[l], digits[r] = digits[r], digits[l]
	}
	return string(digits)
}

type noopBus struct{}

func (noopBus) Publish(ctx context.Context, ev domain.Event) error { return nil }
func (noopBus) Subscribe(ctx context.Context) (<-chan domain.Event, error) {
	return nil, nil
}

func TestCheckoutRequiresRunID(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	svc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	_ = st.Agents().Create(context.Background(), agent)

	task, _ := svc.Create(context.Background(), agent.ID, "t", "do")
	_, err := svc.Checkout(context.Background(), task.ID, agent.ID, "", nil)
	if err == nil {
		t.Fatal("expected error when run_id empty")
	}
}

func TestCheckoutSetsLockFields(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	svc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	_ = st.Agents().Create(context.Background(), agent)

	task, _ := svc.Create(context.Background(), agent.ID, "t", "do")
	got, err := svc.Checkout(context.Background(), task.ID, agent.ID, "run_1", nil)
	if err != nil {
		t.Fatalf("checkout error: %v", err)
	}
	if got.ExecutionLockedAt == nil || got.LockExpiresAt == nil {
		t.Fatal("lock timestamps not set")
	}
	if got.CheckoutRunID != "run_1" {
		t.Fatalf("run_id=%s want run_1", got.CheckoutRunID)
	}
}

func TestStaleLockRecoveryReleasesExpiredLocks(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := noopBus{}
	svc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_1", Name: "a", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	_ = st.Agents().Create(context.Background(), agent)

	task, _ := svc.Create(context.Background(), agent.ID, "t", "do")
	locked, _ := svc.Checkout(context.Background(), task.ID, agent.ID, "run_1", nil)

	expired := clock.t.Add(-time.Hour)
	locked.ExecutionLockedAt = &expired
	locked.LockExpiresAt = &expired
	_ = st.Tasks().Update(context.Background(), locked)

	recovery := NewLockRecoveryService(st, clock, bus, idgen)
	count, err := recovery.SweepStaleLocks(context.Background())
	if err != nil {
		t.Fatalf("sweep error: %v", err)
	}
	if count != 1 {
		t.Fatalf("recovered=%d want 1", count)
	}

	refreshed, _ := st.Tasks().Get(context.Background(), task.ID)
	if refreshed.CheckoutRunID != "" || refreshed.CheckedOutByAgentID != "" {
		t.Fatal("lock not cleared")
	}
	if refreshed.Status != domain.TaskStatusTodo || !refreshed.NeedsRun {
		t.Fatal("task not returned to todo/needs_run")
	}
}
