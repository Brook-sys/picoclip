package services

import (
	"context"
	"fmt"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type WakeupService struct {
	storage ports.Storage
	clock   ports.Clock
	idGen   ports.IDGenerator
}

type CreateWakeupInput struct {
	AgentID  string
	TaskID   string
	Reason   domain.WakeupReason
	Priority int
	Payload  map[string]string
}

func NewWakeupService(storage ports.Storage, clock ports.Clock, idGen ports.IDGenerator) *WakeupService {
	return &WakeupService{storage: storage, clock: clock, idGen: idGen}
}

func (s *WakeupService) Create(ctx context.Context, input CreateWakeupInput) (domain.WakeupRequest, error) {
	if input.AgentID == "" || input.Reason == "" {
		return domain.WakeupRequest{}, fmt.Errorf("%w: agent_id and reason are required", domain.ErrInvalidInput)
	}
	now := s.clock.Now()
	wakeup := domain.WakeupRequest{
		ID:        s.idGen.NewID("wkp"),
		AgentID:   input.AgentID,
		TaskID:    input.TaskID,
		Reason:    input.Reason,
		Status:    domain.WakeupStatusPending,
		Priority:  input.Priority,
		DueAt:     now,
		Payload:   input.Payload,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.storage.Wakeups().Create(ctx, wakeup); err != nil {
		return domain.WakeupRequest{}, err
	}
	return wakeup, nil
}

func (s *WakeupService) ListDue(ctx context.Context, limit int) ([]domain.WakeupRequest, error) {
	return s.storage.Wakeups().ListPending(ctx, s.clock.Now(), limit)
}

func (s *WakeupService) MarkCompleted(ctx context.Context, id string) (domain.WakeupRequest, error) {
	wakeup, err := s.storage.Wakeups().Get(ctx, id)
	if err != nil {
		return domain.WakeupRequest{}, err
	}
	wakeup.Status = domain.WakeupStatusCompleted
	wakeup.UpdatedAt = s.clock.Now()
	return wakeup, s.storage.Wakeups().Update(ctx, wakeup)
}

func (s *WakeupService) ProcessDue(ctx context.Context, limit int) (int, error) {
	wakeups, err := s.ListDue(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, wakeup := range wakeups {
		if wakeup.TaskID != "" {
			if err := s.wakeTask(ctx, wakeup.TaskID); err != nil {
				return processed, err
			}
		}
		wakeup.Status = domain.WakeupStatusCompleted
		wakeup.UpdatedAt = s.clock.Now()
		if err := s.storage.Wakeups().Update(ctx, wakeup); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *WakeupService) wakeTask(ctx context.Context, taskID string) error {
	task, err := s.storage.Tasks().Get(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
		return nil
	}
	if task.CheckoutRunID != "" || task.CheckedOutByAgentID != "" {
		return nil
	}
	if task.Status == domain.TaskStatusInReview || task.Status == domain.TaskStatusBlocked || task.Status == domain.TaskStatusBacklog {
		task.Status = domain.TaskStatusTodo
	}
	task.NeedsRun = true
	task.UpdatedAt = s.clock.Now()
	return s.storage.Tasks().Update(ctx, task)
}
