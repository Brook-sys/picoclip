package services

import (
	"context"

	"picoclip/internal/core/domain"
)

type NoopMemoryProvider struct{}

func (NoopMemoryProvider) ContextForTask(ctx context.Context, task domain.Task, agent domain.Agent) (string, error) {
	return "", nil
}

func (NoopMemoryProvider) SaveRun(ctx context.Context, task domain.Task, run domain.Run) error {
	return nil
}
