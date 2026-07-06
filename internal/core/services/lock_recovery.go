package services

import (
	"context"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type LockRecoveryService struct {
	storage ports.Storage
	clock   ports.Clock
	bus     ports.EventBus
	idGen   ports.IDGenerator
}

func NewLockRecoveryService(storage ports.Storage, clock ports.Clock, bus ports.EventBus, idGen ports.IDGenerator) *LockRecoveryService {
	return &LockRecoveryService{
		storage: storage,
		clock:   clock,
		bus:     bus,
		idGen:   idGen,
	}
}

func scheduleRecoveredContinuousTask(task *domain.Task, now time.Time) {
	task.Status = domain.TaskStatusWaitingNextCycle
	task.NeedsRun = false
	task.FinishedAt = &now
	if task.LoopPausedAt != nil {
		task.LoopNextRunAt = nil
		return
	}
	delay := task.LoopDelaySeconds
	if delay < 1 {
		delay = 60
		task.LoopDelaySeconds = delay
	}
	nextRunAt := now.Add(time.Duration(delay) * time.Second)
	task.LoopRunCount++
	task.LoopNextRunAt = &nextRunAt
}

func (s *LockRecoveryService) SweepStaleLocks(ctx context.Context) (int, error) {
	now := s.clock.Now()
	tasks, err := s.storage.Tasks().List(ctx, ports.TaskFilter{})
	if err != nil {
		return 0, err
	}

	count := 0
	for _, task := range tasks {
		if task.CheckoutRunID == "" || task.CheckedOutByAgentID == "" {
			continue
		}
		if task.LockExpiresAt == nil || task.LockExpiresAt.After(now) {
			continue
		}
		if task.CheckoutRunID != "" {
			if run, err := s.storage.Runs().Get(ctx, task.CheckoutRunID); err == nil && run.Status == domain.RunStatusRunning {
				run.Status = domain.RunStatusTimeout
				run.Error = "stale task lock recovered"
				finished := now
				run.FinishedAt = &finished
				_ = s.storage.Runs().Update(ctx, run)
				_ = s.storage.Events().Create(ctx, domain.Event{ID: s.idGen.NewID("evt"), Type: domain.EventRunRecovered, TaskID: task.ID, AgentID: task.AgentID, RunID: run.ID, Message: run.Error, CreatedAt: now})
			}
		}
		task.CheckedOutByAgentID = ""
		task.CheckoutRunID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
		createRecoveryWakeup := true
		if task.Status == domain.TaskStatusInProgress {
			if task.Mode == domain.TaskModeContinuous {
				scheduleRecoveredContinuousTask(&task, now)
				createRecoveryWakeup = false
			} else {
				task.Status = domain.TaskStatusTodo
				task.NeedsRun = true
			}
		}
		task.UpdatedAt = now
		if err := s.storage.Tasks().Update(ctx, task); err != nil {
			return count, err
		}
		_ = s.storage.Events().Create(ctx, domain.Event{
			ID:        s.idGen.NewID("evt"),
			Type:      domain.EventTaskReleased,
			TaskID:    task.ID,
			AgentID:   task.AgentID,
			Message:   "stale lock recovered",
			CreatedAt: now,
		})
		if createRecoveryWakeup {
			_, _ = NewWakeupService(s.storage, s.clock, s.idGen).Create(ctx, CreateWakeupInput{AgentID: task.AgentID, TaskID: task.ID, Reason: domain.WakeupReasonRecovery, Priority: task.Priority})
		}
		count++
	}
	return count, nil
}
