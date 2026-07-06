package services

import (
	"context"
	"fmt"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type CleanupService struct {
	storage ports.Storage
}

type CleanupResult struct {
	Deleted int `json:"deleted"`
}

func NewCleanupService(storage ports.Storage) *CleanupService {
	return &CleanupService{storage: storage}
}

func (s *CleanupService) DeleteTask(ctx context.Context, id string) error {
	task, err := s.storage.Tasks().Get(ctx, id)
	if err != nil {
		return err
	}
	if task.Status == domain.TaskStatusInProgress || task.CheckoutRunID != "" || task.CheckedOutByAgentID != "" {
		return fmt.Errorf("%w: cannot delete a running task; stop it first", domain.ErrConflict)
	}
	return s.storage.Tasks().Delete(ctx, id)
}

func (s *CleanupService) DeleteFinishedTasks(ctx context.Context) (CleanupResult, error) {
	count, err := s.storage.Tasks().DeleteFinished(ctx)
	return CleanupResult{Deleted: count}, err
}

func (s *CleanupService) DeleteRun(ctx context.Context, id string) error {
	run, err := s.storage.Runs().Get(ctx, id)
	if err != nil {
		return err
	}
	if run.Status == domain.RunStatusRunning {
		return fmt.Errorf("%w: cannot delete a running run; stop the task first", domain.ErrConflict)
	}
	return s.storage.Runs().Delete(ctx, id)
}

func (s *CleanupService) DeleteFinishedRuns(ctx context.Context) (CleanupResult, error) {
	count, err := s.storage.Runs().DeleteFinished(ctx)
	return CleanupResult{Deleted: count}, err
}

func (s *CleanupService) DeleteEvent(ctx context.Context, id string) error {
	return s.storage.Events().Delete(ctx, id)
}

func (s *CleanupService) DeleteFinishedActivity(ctx context.Context) (CleanupResult, error) {
	count, err := s.storage.Events().DeleteFinished(ctx)
	return CleanupResult{Deleted: count}, err
}

func (s *CleanupService) DeleteAllActivity(ctx context.Context) (CleanupResult, error) {
	count, err := s.storage.Events().DeleteAll(ctx)
	return CleanupResult{Deleted: count}, err
}
