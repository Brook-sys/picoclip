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
	if err := storage.ResetAllData(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Agents().Get(ctx, "agt_restore"); err != domain.ErrNotFound {
		t.Fatalf("expected restored agent deleted, got %v", err)
	}
	if _, err := storage.Runtimes().GetByRuntimeID(ctx, "picoclaw"); err != domain.ErrNotFound {
		t.Fatalf("expected restored runtime deleted, got %v", err)
	}
}
