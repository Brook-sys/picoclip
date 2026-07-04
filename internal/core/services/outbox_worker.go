package services

import (
	"context"
	"time"

	"picoclip/internal/core/ports"
)

type OutboxWorker struct {
	storage ports.Storage
	bus     ports.EventBus
}

func NewOutboxWorker(storage ports.Storage, bus ports.EventBus) *OutboxWorker {
	return &OutboxWorker{
		storage: storage,
		bus:     bus,
	}
}

func (w *OutboxWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processOutbox(ctx)
		}
	}
}

func (w *OutboxWorker) processOutbox(ctx context.Context) {
	events, err := w.storage.Events().ListOutbox(ctx, 50)
	if err != nil || len(events) == 0 {
		return
	}

	for _, ev := range events {
		if err := EnqueueWebhookDeliveries(ctx, w.storage, ev, time.Now()); err != nil {
			_ = w.storage.Events().MarkOutboxFailed(ctx, ev.ID, err.Error(), time.Now().Add(5*time.Second))
			continue
		}
		if err := w.bus.Publish(ctx, ev); err != nil {
			_ = w.storage.Events().MarkOutboxFailed(ctx, ev.ID, err.Error(), time.Now().Add(5*time.Second))
			continue
		}
		if err := w.storage.Events().DeleteOutbox(ctx, ev.ID); err != nil {
			_ = w.storage.Events().MarkOutboxFailed(ctx, ev.ID, err.Error(), time.Now().Add(5*time.Second))
		}
	}
}
