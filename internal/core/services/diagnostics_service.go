package services

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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

func (s *DiagnosticsService) RecoveryLiveness(ctx context.Context, limit int) (domain.RecoveryLivenessDiagnostics, error) {
	now := time.Now().UTC()
	counts := map[string]int{
		"runtime_stalled_events": 0,
		"run_recovered_events":   0,
		"retry_scheduled_events": 0,
		"timeout_runs":           0,
		"pending_retry_wakeups":  0,
		"expired_locks":          0,
	}
	items := make([]domain.RecoveryLivenessDiagnosticsItem, 0)

	events, err := s.storage.Events().ListRecent(ctx, 100)
	if err != nil {
		return domain.RecoveryLivenessDiagnostics{}, err
	}
	for _, event := range events {
		kind := ""
		switch event.Type {
		case domain.EventRuntimeStalled:
			counts["runtime_stalled_events"]++
			kind = "runtime_stalled"
		case domain.EventRunRecovered:
			counts["run_recovered_events"]++
			kind = "run_recovered"
		case domain.EventRetryScheduled:
			counts["retry_scheduled_events"]++
			kind = "retry_scheduled"
		}
		if kind == "" {
			continue
		}
		items = append(items, domain.RecoveryLivenessDiagnosticsItem{Kind: kind, TaskID: event.TaskID, AgentID: event.AgentID, RunID: event.RunID, EventID: event.ID, Message: event.Message, CreatedAt: event.CreatedAt})
	}

	runningRuns, err := s.storage.Runs().ListRunning(ctx)
	if err != nil {
		return domain.RecoveryLivenessDiagnostics{}, err
	}
	for _, run := range runningRuns {
		if run.StallTimeout <= 0 || run.LastOutputAt == nil || !run.LastOutputAt.Add(time.Duration(run.StallTimeout)*time.Second).Before(now) {
			continue
		}
		items = append(items, domain.RecoveryLivenessDiagnosticsItem{Kind: "stalled_running_run", TaskID: run.TaskID, AgentID: run.AgentID, RunID: run.ID, CreatedAt: run.StartedAt, Status: string(run.Status)})
	}

	tasks, err := s.storage.Tasks().List(ctx, ports.TaskFilter{})
	if err != nil {
		return domain.RecoveryLivenessDiagnostics{}, err
	}
	for _, task := range tasks {
		if task.LockExpiresAt != nil && task.LockExpiresAt.Before(now) && (task.CheckoutRunID != "" || task.CheckedOutByAgentID != "") {
			counts["expired_locks"]++
			items = append(items, domain.RecoveryLivenessDiagnosticsItem{Kind: "expired_lock", TaskID: task.ID, AgentID: task.CheckedOutByAgentID, RunID: task.CheckoutRunID, CreatedAt: *task.LockExpiresAt, Status: string(task.Status)})
		}
		runs, err := s.storage.Runs().ListByTask(ctx, task.ID)
		if err != nil {
			return domain.RecoveryLivenessDiagnostics{}, err
		}
		for _, run := range runs {
			if run.Status != domain.RunStatusTimeout {
				continue
			}
			counts["timeout_runs"]++
			createdAt := run.StartedAt
			if run.FinishedAt != nil {
				createdAt = *run.FinishedAt
			}
			items = append(items, domain.RecoveryLivenessDiagnosticsItem{Kind: "timeout_run", TaskID: run.TaskID, AgentID: run.AgentID, RunID: run.ID, CreatedAt: createdAt, Status: string(run.Status)})
		}
		wakeups, err := s.storage.Wakeups().ListByTask(ctx, task.ID)
		if err != nil {
			return domain.RecoveryLivenessDiagnostics{}, err
		}
		for _, wakeup := range wakeups {
			if wakeup.Status != domain.WakeupStatusPending || wakeup.Reason != domain.WakeupReasonRetry {
				continue
			}
			counts["pending_retry_wakeups"]++
			items = append(items, domain.RecoveryLivenessDiagnosticsItem{Kind: "pending_retry_wakeup", TaskID: wakeup.TaskID, AgentID: wakeup.AgentID, WakeupID: wakeup.ID, CreatedAt: wakeup.CreatedAt, DueAt: wakeup.DueAt, Status: string(wakeup.Status)})
		}
	}

	sort.SliceStable(items, func(i, j int) bool { return diagnosticItemTime(items[i]).After(diagnosticItemTime(items[j])) })
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return domain.RecoveryLivenessDiagnostics{GeneratedAt: now, Counts: counts, Items: items}, nil
}

func diagnosticItemTime(item domain.RecoveryLivenessDiagnosticsItem) time.Time {
	if !item.DueAt.IsZero() {
		return item.DueAt
	}
	return item.CreatedAt
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
