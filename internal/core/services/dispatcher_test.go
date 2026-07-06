package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestDispatcherDoesNotClaimTaskWhenConcurrencySlotUnavailable(t *testing.T) {
	st := memory.NewStorage()
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	agent := domain.Agent{ID: "agt_dispatch", Name: "dispatcher", Type: "noop", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task := domain.Task{ID: "tsk_dispatch", AgentID: agent.ID, Title: "wait", Prompt: "do", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: now, UpdatedAt: now}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	dispatcher := NewDispatcher(st, nil, testLogger{}, 1)
	dispatcher.semaphore <- struct{}{} // simulate the only worker slot already being used

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	dispatcher.Dispatch(ctx)

	got, err := st.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.CheckoutRunID != "" || got.CheckedOutByAgentID != "" || !got.NeedsRun || got.Status != domain.TaskStatusTodo {
		t.Fatalf("dispatcher claimed task without an available concurrency slot: %#v", got)
	}
	claimed, run, err := st.Tasks().ClaimNextRunnable(context.Background(), now, time.Minute)
	if err != nil {
		t.Fatalf("task should remain safely claimable after dispatcher returns without a slot: %v", err)
	}
	if claimed.ID != task.ID || run.TaskID != task.ID {
		t.Fatalf("expected original task to be claimable, got task=%#v run=%#v", claimed, run)
	}
}
