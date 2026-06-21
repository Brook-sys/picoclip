package services

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type DiagnosticsService struct {
	storage       ports.Storage
	runtimes      *RuntimeManager
	storageType   string
	databasePath  string
	workspacePath string
	runtimePath   string
	logLevel      string
	debugMode     bool
}

type DiagnosticsConfig struct {
	StorageType   string
	DatabasePath  string
	WorkspacePath string
	RuntimePath   string
	LogLevel      string
	DebugMode     bool
}

func NewDiagnosticsService(storage ports.Storage, runtimes *RuntimeManager, config DiagnosticsConfig) *DiagnosticsService {
	return &DiagnosticsService{storage: storage, runtimes: runtimes, storageType: config.StorageType, databasePath: config.DatabasePath, workspacePath: config.WorkspacePath, runtimePath: config.RuntimePath, logLevel: config.LogLevel, debugMode: config.DebugMode}
}

func (s *DiagnosticsService) Report(ctx context.Context) domain.DiagnosticsReport {
	now := time.Now().UTC()
	report := domain.DiagnosticsReport{
		StorageType:   s.storageType,
		DatabasePath:  s.databasePath,
		WorkspacePath: s.workspacePath,
		RuntimePath:   s.runtimePath,
		LogLevel:      s.logLevel,
		DebugMode:     s.debugMode,
		GeneratedAt:   now,
	}
	report.Checks = append(report.Checks, s.pathCheck("database_parent_writable", filepath.Dir(s.databasePath), true, now))
	report.Checks = append(report.Checks, s.pathCheck("workspace_path_writable", s.workspacePath, true, now))
	report.Checks = append(report.Checks, s.pathCheck("runtime_path_writable", s.runtimePath, true, now))

	if _, err := s.storage.Settings().List(ctx); err != nil {
		report.Checks = append(report.Checks, domain.DiagnosticCheck{Name: "storage_read", Status: "error", Message: err.Error(), CheckedAt: now})
	} else {
		report.Checks = append(report.Checks, domain.DiagnosticCheck{Name: "storage_read", Status: "ok", Message: "storage is readable", CheckedAt: now})
	}

	for _, manifest := range s.runtimes.Catalog() {
		health, _ := s.runtimes.Health(ctx, manifest.ID)
		status := health.Status
		message := health.Version
		if status == "" || status == "not_configured" {
			status = "warning"
			message = "runtime is not configured"
		}
		if status == "ok" && message == "" {
			message = "runtime health check passed"
		}
		if len(health.Errors) > 0 {
			status = "error"
			message = health.Errors[0]
		}
		report.Checks = append(report.Checks, domain.DiagnosticCheck{Name: "runtime_" + string(manifest.ID), Status: status, Message: message, CheckedAt: now})
	}
	return report
}

func (s *DiagnosticsService) pathCheck(name string, path string, shouldCreate bool, now time.Time) domain.DiagnosticCheck {
	if path == "" || path == "." {
		return domain.DiagnosticCheck{Name: name, Status: "warning", Message: "path is not configured", CheckedAt: now}
	}
	if shouldCreate {
		_ = os.MkdirAll(path, 0755)
	}
	probe := filepath.Join(path, ".picoclip-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0644); err != nil {
		return domain.DiagnosticCheck{Name: name, Status: "error", Message: err.Error(), CheckedAt: now}
	}
	_ = os.Remove(probe)
	return domain.DiagnosticCheck{Name: name, Status: "ok", Message: path, CheckedAt: now}
}
