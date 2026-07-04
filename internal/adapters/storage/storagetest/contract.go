package storagetest

import (
	"context"
	"testing"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type StorageFactory func(t *testing.T) ports.Storage

func RunStorageContract(t *testing.T, factory StorageFactory) {
	t.Helper()
	t.Run("settings", func(t *testing.T) { testSettings(t, factory) })
	t.Run("agents_tasks_runs_messages_events", func(t *testing.T) { testCoreFlow(t, factory) })
	t.Run("runtimes", func(t *testing.T) { testRuntimes(t, factory) })
	t.Run("reset_and_restore", func(t *testing.T) { testResetAndRestore(t, factory) })
}

func testSettings(t *testing.T, factory StorageFactory) {
	ctx := context.Background()
	storage := factory(t)
	if err := storage.Settings().Set(ctx, "general", `{"theme":"dark"}`); err != nil {
		t.Fatal(err)
	}
	value, err := storage.Settings().Get(ctx, "general")
	if err != nil {
		t.Fatal(err)
	}
	if value == "" {
		t.Fatal("expected stored setting")
	}
	all, err := storage.Settings().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if all["general"] == "" {
		t.Fatalf("expected setting in list: %#v", all)
	}
}

func testCoreFlow(t *testing.T, factory StorageFactory) {
	ctx := context.Background()
	storage := factory(t)
	now := time.Now().UTC()
	workspace := domain.Workspace{ID: "ws_contract", Name: "Contract Workspace", RootPath: t.TempDir(), CreatedAt: now, UpdatedAt: now}
	if err := storage.Workspaces().Create(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	agent := domain.Agent{ID: "agt_contract", Name: "Contract Agent", Type: "noop", Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := storage.Agents().Create(ctx, agent); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: "tsk_contract", WorkspaceID: workspace.ID, AgentID: agent.ID, Title: "Task", Prompt: "Do work", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: now, UpdatedAt: now}
	if err := storage.Tasks().Create(ctx, task); err != nil {
		t.Fatal(err)
	}
	claimed, err := storage.Tasks().ClaimNextPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != task.ID {
		t.Fatalf("expected claimed %s, got %s", task.ID, claimed.ID)
	}
	if _, err := storage.Tasks().ClaimNextPending(ctx); err != domain.ErrNoPendingTasks {
		t.Fatalf("expected no pending tasks, got %v", err)
	}
	locked := domain.Task{ID: "tsk_locked", WorkspaceID: workspace.ID, AgentID: agent.ID, Title: "Locked", Prompt: "Do locked work", Status: domain.TaskStatusInProgress, NeedsRun: true, CheckoutRunID: "run_locked", CheckedOutByAgentID: agent.ID, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)}
	if err := storage.Tasks().Create(ctx, locked); err != nil {
		t.Fatal(err)
	}
	exhausted := domain.Task{ID: "tsk_exhausted", WorkspaceID: workspace.ID, AgentID: agent.ID, Title: "Exhausted", Prompt: "Do exhausted work", Status: domain.TaskStatusTodo, NeedsRun: true, Attempts: 1, MaxAttempts: 1, CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)}
	if err := storage.Tasks().Create(ctx, exhausted); err != nil {
		t.Fatal(err)
	}
	low := domain.Task{ID: "tsk_low_priority", WorkspaceID: workspace.ID, AgentID: agent.ID, Title: "Low", Prompt: "Do low work", Status: domain.TaskStatusTodo, NeedsRun: true, Priority: 1, CreatedAt: now.Add(3 * time.Second), UpdatedAt: now.Add(3 * time.Second)}
	high := domain.Task{ID: "tsk_high_priority", WorkspaceID: workspace.ID, AgentID: agent.ID, Title: "High", Prompt: "Do high work", Status: domain.TaskStatusTodo, NeedsRun: true, Priority: 10, CreatedAt: now.Add(4 * time.Second), UpdatedAt: now.Add(4 * time.Second)}
	if err := storage.Tasks().Create(ctx, low); err != nil {
		t.Fatal(err)
	}
	if err := storage.Tasks().Create(ctx, high); err != nil {
		t.Fatal(err)
	}
	claimed, err = storage.Tasks().ClaimNextPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != high.ID {
		t.Fatalf("expected high priority task, got %s", claimed.ID)
	}
	claimed, err = storage.Tasks().ClaimNextPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != low.ID {
		t.Fatalf("expected low priority task, got %s", claimed.ID)
	}
	if _, err := storage.Tasks().ClaimNextPending(ctx); err != domain.ErrNoPendingTasks {
		t.Fatalf("expected locked and exhausted tasks skipped, got %v", err)
	}
	runnable := domain.Task{ID: "tsk_runnable", WorkspaceID: workspace.ID, AgentID: agent.ID, Title: "Runnable", Prompt: "Run me", Status: domain.TaskStatusTodo, NeedsRun: true, CreatedAt: now.Add(5 * time.Second), UpdatedAt: now.Add(5 * time.Second)}
	if err := storage.Tasks().Create(ctx, runnable); err != nil {
		t.Fatal(err)
	}
	runnableClaim, runnableRun, err := storage.Tasks().ClaimNextRunnable(ctx, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if runnableClaim.ID != runnable.ID || runnableClaim.CheckoutRunID == "" || runnableRun.ID != runnableClaim.CheckoutRunID {
		t.Fatalf("unexpected runnable claim task=%#v run=%#v", runnableClaim, runnableRun)
	}
	persistedRun, err := storage.Runs().Get(ctx, runnableRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedRun.TaskID != runnable.ID || persistedRun.Status != domain.RunStatusRunning {
		t.Fatalf("unexpected persisted runnable run: %#v", persistedRun)
	}
	run := domain.Run{ID: "run_contract", TaskID: task.ID, AgentID: agent.ID, DriverType: "noop", Status: domain.RunStatusSucceeded, StartedAt: now}
	if err := storage.Runs().Create(ctx, run); err != nil {
		t.Fatal(err)
	}
	runs, err := storage.Runs().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("expected run in task list, got %#v", runs)
	}
	msg := domain.Message{ID: "msg_contract", TaskID: task.ID, Role: domain.MessageRoleUser, Body: "hello", CreatedAt: now}
	if err := storage.Messages().Create(ctx, msg); err != nil {
		t.Fatal(err)
	}
	messages, err := storage.Messages().ListByTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Body != msg.Body {
		t.Fatalf("expected message, got %#v", messages)
	}
	evt := domain.Event{ID: "evt_contract", Type: domain.EventRunCompleted, TaskID: task.ID, AgentID: agent.ID, RunID: run.ID, Message: "done", CreatedAt: now}
	if err := storage.Events().Create(ctx, evt); err != nil {
		t.Fatal(err)
	}
	recent, err := storage.Events().ListRecent(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) == 0 {
		t.Fatal("expected recent events")
	}
	subscription := domain.WebhookSubscription{ID: "wh_contract", Name: "Contract", URL: "https://example.test/hook", EventTypes: []domain.EventType{domain.EventRunCompleted}, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if err := storage.Webhooks().CreateSubscription(ctx, subscription); err != nil {
		t.Fatal(err)
	}
	subscriptions, err := storage.Webhooks().ListSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 1 || subscriptions[0].ID != subscription.ID {
		t.Fatalf("expected webhook subscription, got %#v", subscriptions)
	}
	delivery := domain.WebhookDelivery{ID: "whd_contract", SubscriptionID: subscription.ID, EventID: evt.ID, EventType: evt.Type, URL: subscription.URL, Status: domain.WebhookDeliveryPending, RequestBody: "{}", CreatedAt: now, UpdatedAt: now}
	if err := storage.Webhooks().CreateDelivery(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	deliveries, err := storage.Webhooks().ListDueDeliveries(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 1 || deliveries[0].ID != delivery.ID {
		t.Fatalf("expected webhook delivery, got %#v", deliveries)
	}
	gotDelivery, err := storage.Webhooks().GetDelivery(ctx, delivery.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotDelivery.ID != delivery.ID {
		t.Fatalf("expected webhook delivery by id, got %#v", gotDelivery)
	}
	if err := storage.Webhooks().DeleteSubscription(ctx, subscription.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Webhooks().GetSubscription(ctx, subscription.ID); err != domain.ErrNotFound {
		t.Fatalf("expected webhook subscription deleted, got %v", err)
	}
}

func testRuntimes(t *testing.T, factory StorageFactory) {
	ctx := context.Background()
	storage := factory(t)
	now := time.Now().UTC()
	state := domain.RuntimeState{ID: "runtime_contract", RuntimeID: "crush", Mode: domain.InstallModeExisting, Enabled: true, BinPath: "/tmp/fake-crush", InstalledAt: now, UpdatedAt: now, SettingsJSON: "{}", MetadataJSON: "{}"}
	if err := storage.Runtimes().Save(ctx, state); err != nil {
		t.Fatal(err)
	}
	got, err := storage.Runtimes().GetByRuntimeID(ctx, "crush")
	if err != nil {
		t.Fatal(err)
	}
	if got.BinPath != state.BinPath {
		t.Fatalf("expected runtime bin %s, got %s", state.BinPath, got.BinPath)
	}
	states, err := storage.Runtimes().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one runtime, got %#v", states)
	}
	if err := storage.Runtimes().Delete(ctx, state.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Runtimes().GetByRuntimeID(ctx, "crush"); err != domain.ErrNotFound {
		t.Fatalf("expected runtime not found, got %v", err)
	}
}

func testResetAndRestore(t *testing.T, factory StorageFactory) {
	ctx := context.Background()
	storage := factory(t)
	now := time.Now().UTC()
	backup := ports.BackupData{
		Settings:   map[string]string{"general": `{"theme":"dark"}`},
		Agents:     []domain.Agent{{ID: "agt_restore", Name: "Restored Agent", Type: "noop", Enabled: true, CreatedAt: now, UpdatedAt: now}},
		Workspaces: []domain.Workspace{{ID: "ws_restore", Name: "Restored Workspace", CreatedAt: now, UpdatedAt: now}},
		Tasks:      []domain.Task{{ID: "tsk_restore", AgentID: "agt_restore", Status: domain.TaskStatusTodo, CreatedAt: now, UpdatedAt: now}},
		Runs:       []domain.Run{{ID: "run_restore", TaskID: "tsk_restore", AgentID: "agt_restore", DriverType: "noop", Status: domain.RunStatusSucceeded, StartedAt: now}},
		Messages:   []domain.Message{{ID: "msg_restore", TaskID: "tsk_restore", Role: domain.MessageRoleUser, Body: "restored", CreatedAt: now}},
		Events:     []domain.Event{{ID: "evt_restore", Type: domain.EventTaskCreated, TaskID: "tsk_restore", Message: "restored", CreatedAt: now}},
		Runtimes:   []domain.RuntimeState{{ID: "runtime_restore", RuntimeID: "picoclaw", Mode: domain.InstallModeExisting, Enabled: true, BinPath: "/tmp/picoclaw", InstalledAt: now, UpdatedAt: now, SettingsJSON: "{}", MetadataJSON: "{}"}},
		Webhooks:   []domain.WebhookSubscription{{ID: "wh_restore", Name: "Restored Webhook", URL: "https://example.test/hook", Enabled: true, CreatedAt: now, UpdatedAt: now}},
	}
	if err := storage.RestoreAllData(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Agents().Get(ctx, "agt_restore"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Runtimes().GetByRuntimeID(ctx, "picoclaw"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Webhooks().GetSubscription(ctx, "wh_restore"); err != nil {
		t.Fatal(err)
	}
	if err := storage.ResetAllData(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Agents().Get(ctx, "agt_restore"); err != domain.ErrNotFound {
		t.Fatalf("expected restored agent deleted, got %v", err)
	}
	if _, err := storage.Runtimes().GetByRuntimeID(ctx, "picoclaw"); err != domain.ErrNotFound {
		t.Fatalf("expected restored runtime deleted, got %v", err)
	}
	if _, err := storage.Webhooks().GetSubscription(ctx, "wh_restore"); err != domain.ErrNotFound {
		t.Fatalf("expected restored webhook deleted, got %v", err)
	}
}
