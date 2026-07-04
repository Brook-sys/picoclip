package services

import (
	"context"
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

func (r *Reconciler) Reconcile(ctx context.Context) {
	dueContinuous := r.activateDueContinuousTasks(ctx)
	if dueContinuous > 0 {
		r.logger.Info("reconciler.continuous_tasks_activated", "count", dueContinuous)
	}

	recovery := NewLockRecoveryService(r.storage, r.clock, r.bus, r.idGen)
	count, err := recovery.SweepStaleLocks(ctx)
	if err != nil {
		r.logger.Warn("reconciler.stale_lock_sweep_failed", "err", err)
		return
	}
	if count > 0 {
		r.logger.Info("reconciler.stale_locks_recovered", "count", count)
	}

	processed, err := NewWakeupService(r.storage, r.clock, r.idGen).ProcessDue(ctx, 100)
	if err != nil {
		r.logger.Warn("reconciler.wakeup_process_failed", "err", err)
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
	if task.Status == domain.TaskStatusCancelled || task.Status == domain.TaskStatusDone || task.LoopPausedAt != nil {
		return
	}
	delay := task.LoopDelaySeconds
	if delay < 1 {
		delay = 60
		task.LoopDelaySeconds = delay
	}
	nextRunAt := finishedAt.Add(time.Duration(delay) * time.Second)
	task.Status = domain.TaskStatusWaitingNextCycle
	task.NeedsRun = false
	task.LoopRunCount++
	task.LoopNextRunAt = &nextRunAt
	task.FinishedAt = &finishedAt
	task.UpdatedAt = finishedAt
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
		finished := now
		run.FinishedAt = &finished
		_ = r.storage.Runs().Update(ctx, run)
		if r.canceler != nil {
			_ = r.canceler.CancelRun(ctx, run)
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
				task.NeedsRun = true
				task.UpdatedAt = now
				_ = r.storage.Tasks().Update(ctx, task)
				requeued = true
			}
		}

		if requeued {
			due := now.Add(time.Duration(run.Attempt+1) * 30 * time.Second)
			if due.Sub(now) > 5*time.Minute {
				due = now.Add(5 * time.Minute)
			}
			wakeup := domain.WakeupRequest{
				ID:        r.idGen.NewID("wakeup"),
				TaskID:    run.TaskID,
				AgentID:   run.AgentID,
				Reason:    domain.WakeupReasonRetry,
				Status:    domain.WakeupStatusPending,
				Priority:  5,
				DueAt:     due,
				CreatedAt: now,
			}
			_ = r.storage.Wakeups().Create(ctx, wakeup)
		}
		count++
	}

	return count
}

func (r *Reconciler) recoverOrphanedRuns(ctx context.Context) int {
	runs, err := r.storage.Runs().ListRunning(ctx)
	if err != nil {
		return 0
	}
	now := r.clock.Now()
	count := 0
	for _, run := range runs {
		if run.LastOutputAt != nil {
			continue
		}
		if now.Sub(run.StartedAt) < 2*time.Minute {
			continue
		}
		run.Status = domain.RunStatusTimeout
		finished := now
		run.FinishedAt = &finished
		_ = r.storage.Runs().Update(ctx, run)
		if r.canceler != nil {
			_ = r.canceler.CancelRun(ctx, run)
		}

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
				task.NeedsRun = true
				task.UpdatedAt = now
				_ = r.storage.Tasks().Update(ctx, task)
			}
		}
		count++
	}
	return count
}
