package ports

import (
	"context"

	"picoclip/internal/core/domain"
)

type EventBus interface {
	Publish(ctx context.Context, event domain.Event) error
	Subscribe(ctx context.Context) (<-chan domain.Event, error)
}
