package events

import (
	"context"

	"picoclip/internal/core/domain"
)

type InMemoryBus struct {
	ch chan domain.Event
}

func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		ch: make(chan domain.Event, 100),
	}
}

func (b *InMemoryBus) Publish(ctx context.Context, event domain.Event) error {
	select {
	case b.ch <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Se o buffer estiver cheio, descartamos (ou bloqueamos, depende da estratégia)
		// Para o MVP, evitar block:
		select {
		case b.ch <- event:
		default:
		}
		return nil
	}
}

func (b *InMemoryBus) Subscribe(ctx context.Context) (<-chan domain.Event, error) {
	return b.ch, nil
}
