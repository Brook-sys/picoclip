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
			if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
				task.Status = domain.TaskStatusBlocked
				task.NeedsRun = false
			} else {
				task.NeedsRun = true
				requeued = true
			}
			task.UpdatedAt = now
			_ = r.storage.Tasks().Update(ctx, task)
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
			if task.MaxAttempts > 0 && task.Attempts >= task.MaxAttempts {
				task.Status = domain.TaskStatusBlocked
				task.NeedsRun = false
			} else {
				task.NeedsRun = true
			}
			task.UpdatedAt = now
			_ = r.storage.Tasks().Update(ctx, task)
		}
		count++
	}
	return count
}
