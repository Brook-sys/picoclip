package services

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type Reconciler struct {
	storage  ports.Storage
	clock    ports.Clock
	bus      ports.EventBus
	idGen    ports.IDGenerator
	logger   ports.Logger
	canceler RunCanceler
}

func NewReconciler(storage ports.Storage, clock ports.Clock, bus ports.EventBus, idGen ports.IDGenerator, logger ports.Logger) *Reconciler {
	return &Reconciler{
		storage: storage,
		clock:   clock,
		bus:     bus,
		idGen:   idGen,
		logger:  logger,
	}
}

func (r *Reconciler) SetCanceler(canceler RunCanceler) {
	r.canceler = canceler
}

func (r *Reconciler) emitRuntimeEvent(ctx context.Context, eventType domain.EventType, run domain.Run, message string, data map[string]string, now time.Time) {
	if data == nil {
		data = map[string]string{}
	}
	if _, ok := data["runtime_id"]; !ok {
		data["runtime_id"] = run.DriverType
	}
	_ = r.storage.Events().Create(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: eventType, TaskID: run.TaskID, AgentID: run.AgentID, RunID: run.ID, Message: message, Data: data, CreatedAt: now})
}

var secretLikeErrorPattern = regexp.MustCompile(`(?i)(token|secret|password|api[_-]?key|authorization)=([^\s,;]+)`)

func sanitizeDiagnosticError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "unknown error"
	}
	return secretLikeErrorPattern.ReplaceAllString(msg, "$1=[redacted]")
}

func (r *Reconciler) emitFailureEvent(ctx context.Context, phase string, err error) {
	message := "Reconciler failed during " + strings.ReplaceAll(phase, "_", " ")
	_ = r.storage.Events().Create(ctx, domain.Event{
		ID:        r.idGen.NewID("evt"),
		Type:      domain.EventReconcilerFailed,
		Message:   message,
		Data:      map[string]string{"phase": phase, "error": sanitizeDiagnosticError(err)},
		CreatedAt: r.clock.Now(),
	})
}

func (r *Reconciler) Reconcile(ctx context.Context) {
	dueContinuous := r.activateDueContinuousTasks(ctx)
	if dueContinuous > 0 {
		r.logger.Info("reconciler.continuous_tasks_activated", "count", dueContinuous)
	}

	recovery := NewLockRecoveryService(r.storage, r.clock, r.bus, r.idGen)
	count, err := recovery.SweepStaleLocks(ctx)
	if err != nil {
		r.logger.Warn("reconciler.stale_lock_sweep_failed", "err", err)
		r.emitFailureEvent(ctx, "stale_lock_sweep", err)
		return
	}
	if count > 0 {
		r.logger.Info("reconciler.stale_locks_recovered", "count", count)
	}

	processed, err := NewWakeupService(r.storage, r.clock, r.idGen).ProcessDue(ctx, 100)
	if err != nil {
		r.logger.Warn("reconciler.wakeup_process_failed", "err", err)
		r.emitFailureEvent(ctx, "wakeup_processing", err)
		return
	}
	if processed > 0 {
		r.logger.Info("reconciler.wakeups_processed", "count", processed)
	}

	stalled := r.detectStalledRuns(ctx)
	if stalled > 0 {
		r.logger.Info("reconciler.stalled_runs_detected", "count", stalled)
	}

	orphans := r.recoverOrphanedRuns(ctx)
	if orphans > 0 {
		r.logger.Info("reconciler.orphaned_runs_recovered", "count", orphans)
	}
}

func (r *Reconciler) activateDueContinuousTasks(ctx context.Context) int {
	tasks, err := r.storage.Tasks().List(ctx, ports.TaskFilter{Status: domain.TaskStatusWaitingNextCycle})
	if err != nil {
		r.logger.Warn("reconciler.continuous_tasks_list_failed", "err", err)
		return 0
	}

	now := r.clock.Now()
	count := 0
	for _, task := range tasks {
		if task.Mode != domain.TaskModeContinuous || task.LoopPausedAt != nil || task.CheckoutRunID != "" || task.CheckedOutByAgentID != "" {
			continue
		}
		if task.LoopNextRunAt == nil || task.LoopNextRunAt.After(now) {
			continue
		}
		task.Status = domain.TaskStatusTodo
		task.NeedsRun = true
		task.LoopNextRunAt = nil
		task.FinishedAt = nil
		task.CompletedAt = nil
		task.UpdatedAt = now
		if err := r.storage.Tasks().Update(ctx, task); err != nil {
			r.logger.Warn("reconciler.continuous_task_activate_failed", "task_id", task.ID, "err", err)
			continue
		}
		count++
	}
	return count
}

func (r *Reconciler) scheduleNextContinuousCycle(ctx context.Context, task domain.Task, finishedAt time.Time) {
	task.FinishedAt = &finishedAt
	task.UpdatedAt = finishedAt

	if task.Status == domain.TaskStatusCancelled || task.Status == domain.TaskStatusDone || task.Mode != domain.TaskModeContinuous {
		_ = r.storage.Tasks().Update(ctx, task)
		return
	}

	task.Status = domain.TaskStatusWaitingNextCycle
	task.NeedsRun = false
	if task.LoopPausedAt != nil {
		_ = r.storage.Tasks().Update(ctx, task)
		return
	}

	delay := task.LoopDelaySeconds
	if delay < 1 {
		delay = 60
		task.LoopDelaySeconds = delay
	}
	nextRunAt := finishedAt.Add(time.Duration(delay) * time.Second)
	task.LoopRunCount++
	task.LoopNextRunAt = &nextRunAt
	_ = r.storage.Tasks().Update(ctx, task)
}

func (r *Reconciler) detectStalledRuns(ctx context.Context) int {
	runs, err := r.storage.Runs().ListRunning(ctx)
	if err != nil {
		r.logger.Warn("reconciler.list_running_failed", "err", err)
		return 0
	}

	now := r.clock.Now()
	count := 0

	for _, run := range runs {
		if run.LastOutputAt == nil || run.StallTimeout <= 0 {
			continue
		}
		if now.Sub(*run.LastOutputAt) <= time.Duration(run.StallTimeout)*time.Second {
			continue
		}

		run.Status = domain.RunStatusTimeout
		run.Error = "run stalled: no output before stall timeout"
		finished := now
		run.FinishedAt = &finished
		_ = r.storage.Runs().Update(ctx, run)
		_ = r.storage.Events().Create(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunTimeout, TaskID: run.TaskID, AgentID: run.AgentID, RunID: run.ID, Message: run.Error, CreatedAt: now})
		r.emitRuntimeEvent(ctx, domain.EventRuntimeStalled, run, "Runtime stalled", map[string]string{"phase": "stall_detected", "stall_timeout_seconds": strconv.Itoa(run.StallTimeout)}, now)
		if r.canceler != nil {
			r.emitRuntimeEvent(ctx, domain.EventRuntimeCancelRequested, run, "Runtime cancellation requested", map[string]string{"phase": "cancel_requested", "reason": "run_stalled"}, now)
			if err := r.canceler.CancelRun(ctx, run); err != nil {
				r.emitRuntimeEvent(ctx, domain.EventRuntimeCancelFailed, run, "Runtime cancellation failed", map[string]string{"phase": "cancel_failed", "reason": "run_stalled", "error": err.Error()}, now)
			} else {
				r.emitRuntimeEvent(ctx, domain.EventRuntimeCancelSucceeded, run, "Runtime cancellation succeeded", map[string]string{"phase": "cancel_succeeded", "reason": "run_stalled"}, now)
			}
		}

		requeued := false
		task, err := r.storage.Tasks().Get(ctx, run.TaskID)
		if err == nil && task.CheckoutRunID == run.ID {
			task.CheckoutRunID = ""
			task.CheckedOutByAgentID = ""
			task.ExecutionLockedAt = nil
			task.LockExpiresAt = nil
			if task.Mode == domain.TaskModeContinuous {
				r.scheduleNextContinuousCycle(ctx, task, now)
			} else if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
				task.Status = domain.TaskStatusBlocked
				task.NeedsRun = false
				task.UpdatedAt = now
				_ = r.storage.Tasks().Update(ctx, task)
			} else {
				task.NeedsRun = false
				task.UpdatedAt = now
				_ = r.storage.Tasks().Update(ctx, task)
				requeued = true
			}
		}

		if requeued {
			r.scheduleRetryWakeup(ctx, run, now, "run_timeout", "Retry scheduled after run timeout")
		}
		count++
	}

	return count
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1<<(attempt-1)) * 30 * time.Second
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func (r *Reconciler) retryWakeupExists(ctx context.Context, run domain.Run) bool {
	wakeups, err := r.storage.Wakeups().ListByTask(ctx, run.TaskID)
	if err != nil {
		return false
	}
	for _, wakeup := range wakeups {
		if wakeup.Reason != domain.WakeupReasonRetry || wakeup.Status != domain.WakeupStatusPending {
			continue
		}
		if wakeup.Payload["previous_run_id"] == run.ID {
			return true
		}
	}
	return false
}

func (r *Reconciler) scheduleRetryWakeup(ctx context.Context, run domain.Run, now time.Time, reason, message string) {
	if r.retryWakeupExists(ctx, run) {
		return
	}
	delay := retryBackoff(run.Attempt)
	payload := map[string]string{
		"previous_run_id": run.ID,
		"attempt":         strconv.Itoa(run.Attempt),
		"backoff_seconds": strconv.Itoa(int(delay.Seconds())),
		"retryable":       "true",
		"classification":  "retryable",
		"reason":          reason,
	}
	wakeup := domain.WakeupRequest{
		ID:        r.idGen.NewID("wakeup"),
		TaskID:    run.TaskID,
		AgentID:   run.AgentID,
		Reason:    domain.WakeupReasonRetry,
		Status:    domain.WakeupStatusPending,
		Priority:  5,
		DueAt:     now.Add(delay),
		Payload:   payload,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_ = r.storage.Wakeups().Create(ctx, wakeup)
	_ = r.storage.Events().Create(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRetryScheduled, TaskID: run.TaskID, AgentID: run.AgentID, RunID: run.ID, Message: message, Data: payload, CreatedAt: now})
}

func (r *Reconciler) recoverOrphanedRuns(ctx context.Context) int {
	runs, err := r.storage.Runs().ListRunning(ctx)
	if err != nil {
		return 0
	}
	now := r.clock.Now()
	count := 0
	for _, run := range runs {
		if _, err := r.storage.Tasks().Get(ctx, run.TaskID); err != nil {
			run.Status = domain.RunStatusTimeout
			run.Error = "orphaned run recovered: task not found"
			finished := now
			run.FinishedAt = &finished
			_ = r.storage.Runs().Update(ctx, run)
			_ = r.storage.Events().Create(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunRecovered, TaskID: run.TaskID, AgentID: run.AgentID, RunID: run.ID, Message: run.Error, CreatedAt: now})
			if r.canceler != nil {
				_ = r.canceler.CancelRun(ctx, run)
			}
			count++
			continue
		}
		if run.LastOutputAt != nil {
			continue
		}
		if now.Sub(run.StartedAt) < 2*time.Minute {
			continue
		}
		run.Status = domain.RunStatusTimeout
		run.Error = "orphaned run recovered: missing output heartbeat"
		finished := now
		run.FinishedAt = &finished
		_ = r.storage.Runs().Update(ctx, run)
		_ = r.storage.Events().Create(ctx, domain.Event{ID: r.idGen.NewID("evt"), Type: domain.EventRunRecovered, TaskID: run.TaskID, AgentID: run.AgentID, RunID: run.ID, Message: run.Error, CreatedAt: now})
		if r.canceler != nil {
			_ = r.canceler.CancelRun(ctx, run)
		}

		task, err := r.storage.Tasks().Get(ctx, run.TaskID)
		requeued := false
		if err == nil && task.CheckoutRunID == run.ID {
			task.CheckoutRunID = ""
			task.CheckedOutByAgentID = ""
			task.ExecutionLockedAt = nil
			task.LockExpiresAt = nil
			if task.Mode == domain.TaskModeContinuous {
				r.scheduleNextContinuousCycle(ctx, task, now)
			} else if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
				task.Status = domain.TaskStatusBlocked
				task.NeedsRun = false
				task.UpdatedAt = now
				_ = r.storage.Tasks().Update(ctx, task)
			} else {
				task.NeedsRun = false
				task.UpdatedAt = now
				_ = r.storage.Tasks().Update(ctx, task)
				requeued = true
			}
		}
		if requeued {
			r.scheduleRetryWakeup(ctx, run, now, "orphaned_run", "Retry scheduled after orphaned run recovery")
		}
		count++
	}
	return count
}
