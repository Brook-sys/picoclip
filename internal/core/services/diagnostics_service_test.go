package services

import (
	"context"
	"testing"

	"picoclip/internal/adapters/storage/memory"
)

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
