package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"picoclip/internal/adapters/storage/memory"
)

func TestDiagnosticsReportFlagsReadOnlyDatabaseFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission test requires non-root user")
	}
	storage := memory.NewStorage()
	baseDir := t.TempDir()
	databasePath := filepath.Join(baseDir, "picoclip.db")
	if err := os.WriteFile(databasePath, []byte("fixture"), 0444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(databasePath, 0644) })

	runtimes := NewRuntimeManager(storage, t.TempDir(), SystemClock{})
	svc := NewDiagnosticsService(storage, runtimes, DiagnosticsConfig{StorageType: "sqlite", DatabasePath: databasePath, WorkspacePath: t.TempDir(), RuntimePath: t.TempDir()})
	report := svc.Report(context.Background())
	for _, check := range report.Checks {
		if check.Name == "database_file_writable" {
			if check.Status != "error" {
				t.Fatalf("database_file_writable status=%q, want error", check.Status)
			}
			return
		}
	}
	t.Fatalf("database_file_writable check missing: %#v", report.Checks)
}

func TestDiagnosticsReportIncludesCoreChecks(t *testing.T) {
	storage := memory.NewStorage()
	clock := SystemClock{}
	runtimes := NewRuntimeManager(storage, t.TempDir(), clock)
	workspace := t.TempDir()
	runtimePath := t.TempDir()

	svc := NewDiagnosticsService(storage, runtimes, DiagnosticsConfig{StorageType: "memory", WorkspacePath: workspace, RuntimePath: runtimePath, LogLevel: "debug", DebugMode: true})
	report := svc.Report(context.Background())

	if report.StorageType != "memory" {
		t.Fatalf("expected memory storage, got %s", report.StorageType)
	}
	if !report.DebugMode {
		t.Fatalf("expected debug mode")
	}
	if len(report.Checks) == 0 {
		t.Fatalf("expected checks")
	}
	var foundStorage bool
	for _, check := range report.Checks {
		if check.Name == "storage_read" && check.Status == "ok" {
			foundStorage = true
		}
	}
	if !foundStorage {
		t.Fatalf("expected storage_read ok check, got %#v", report.Checks)
	}
}
