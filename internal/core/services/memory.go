package services

import (
	"context"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type NoopMemoryProvider struct{}

func (NoopMemoryProvider) Store(ctx context.Context, document ports.MemoryDocument) error {
	return nil
}

func (NoopMemoryProvider) Search(ctx context.Context, query ports.MemoryQuery) ([]ports.MemorySearchResult, error) {
	return nil, nil
}

func (NoopMemoryProvider) ContextForTask(ctx context.Context, task domain.Task, agent domain.Agent) (string, error) {
	return "", nil
}

func (NoopMemoryProvider) SaveRun(ctx context.Context, task domain.Task, run domain.Run) error {
	return nil
}
