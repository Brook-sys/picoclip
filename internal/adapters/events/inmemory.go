package events

import (
	"context"
	"sync"

	"picoclip/internal/core/domain"
)

type InMemoryBus struct {
	mu          sync.RWMutex
	subscribers map[chan domain.Event]struct{}
}

func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		subscribers: make(map[chan domain.Event]struct{}),
	}
}

func (b *InMemoryBus) Publish(ctx context.Context, event domain.Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// avoid blocking if channel full
		}
	}
	return nil
}

func (b *InMemoryBus) Subscribe(ctx context.Context) (<-chan domain.Event, error) {
	ch := make(chan domain.Event, 100)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
		close(ch)
	}()

	return ch, nil
}
