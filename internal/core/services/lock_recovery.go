package services

import (
	"context"

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
		task.CheckedOutByAgentID = ""
		task.CheckoutRunID = ""
		task.ExecutionLockedAt = nil
		task.LockExpiresAt = nil
		if task.Status == domain.TaskStatusInProgress {
			task.Status = domain.TaskStatusTodo
			task.NeedsRun = true
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
		_, _ = NewWakeupService(s.storage, s.clock, s.idGen).Create(ctx, CreateWakeupInput{AgentID: task.AgentID, TaskID: task.ID, Reason: domain.WakeupReasonRecovery, Priority: task.Priority})
		count++
	}
	return count, nil
}
