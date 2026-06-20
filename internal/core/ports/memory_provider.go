package ports

import (
	"context"

	"picoclip/internal/core/domain"
)

type MemoryProvider interface {
	ContextForTask(ctx context.Context, task domain.Task, agent domain.Agent) (string, error)
	SaveRun(ctx context.Context, task domain.Task, run domain.Run) error
}
