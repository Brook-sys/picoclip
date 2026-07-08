package services

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestWakeupServiceProcessDueRecordsHeartbeatPilotEvent(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 8, 18, 0, 0, 0, time.UTC)}
	idgen := &seqID{}
	bus := &recordingBus{}
	taskSvc := NewTaskService(st, clock, idgen, bus)

	agent := domain.Agent{ID: "agt_heartbeat", Name: "heartbeat", Type: "internal", Enabled: true, Capability: domain.CapabilityWorker, CreatedAt: clock.t, UpdatedAt: clock.t}
	if err := st.Agents().Create(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	task, err := taskSvc.Create(context.Background(), agent.ID, "heartbeat pilot", "do")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	task.Status = domain.TaskStatusBlocked
	task.NeedsRun = false
	if err := st.Tasks().Update(context.Background(), task); err != nil {
		t.Fatalf("prepare task: %v", err)
	}

	wakeupSvc := NewWakeupServiceWithBus(st, clock, idgen, bus)
	wakeup, err := wakeupSvc.Create(context.Background(), CreateWakeupInput{AgentID: agent.ID, TaskID: task.ID, Reason: domain.WakeupReasonComment, Priority: 7})
	if err != nil {
		t.Fatalf("create wakeup: %v", err)
	}
	processed, err := wakeupSvc.ProcessDue(context.Background(), 10)
	if err != nil {
		t.Fatalf("process due: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed=%d want 2", processed)
	}

	var found bool
	for _, event := range bus.events {
		if event.Type != domain.EventAgentHeartbeatWakeup {
			continue
		}
		found = true
		if event.AgentID != agent.ID || event.TaskID != task.ID {
			t.Fatalf("event scope agent=%q task=%q want %q/%q", event.AgentID, event.TaskID, agent.ID, task.ID)
		}
		if event.Message == "" {
			t.Fatal("heartbeat wakeup event should include an operational message")
		}
		if event.Data["wakeup_id"] != wakeup.ID {
			continue
		}
		if event.Data["wake_reason"] != string(domain.WakeupReasonComment) {
			t.Fatalf("event data=%#v want wake reason", event.Data)
		}
		if event.Data["engine_mode"] != "pilot" || event.Data["context_route"] != "/agent-api/tasks/{id}/heartbeat-context" {
			t.Fatalf("event data=%#v want pilot mode and heartbeat-context route", event.Data)
		}
	}
	if !found {
		t.Fatalf("missing %s event; events=%#v", domain.EventAgentHeartbeatWakeup, bus.events)
	}
}

type recordingBus struct {
	events []domain.Event
}

func (b *recordingBus) Publish(ctx context.Context, event domain.Event) error {
	b.events = append(b.events, event)
	return nil
}

func (b *recordingBus) Subscribe(ctx context.Context) (<-chan domain.Event, error) {
	ch := make(chan domain.Event)
	close(ch)
	return ch, nil
}
