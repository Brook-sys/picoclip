package services

import (
	"context"
	"time"

	"picoclip/internal/core/ports"
)

type Scheduler struct {
	interval   time.Duration
	dispatcher *Dispatcher
	reconciler *Reconciler
	logger     ports.Logger
}

func NewScheduler(interval time.Duration, dispatcher *Dispatcher, reconciler *Reconciler, logger ports.Logger) *Scheduler {
	return &Scheduler{interval: interval, dispatcher: dispatcher, reconciler: reconciler, logger: logger}
}

func (s *Scheduler) reconcile(ctx context.Context) {
	if s.reconciler != nil {
		s.reconciler.Reconcile(ctx)
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	s.reconcile(ctx)
	s.dispatcher.Dispatch(ctx)
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.runOnce(ctx)
	for {
		select {
		case <-ticker.C:
			s.runOnce(ctx)
		case <-ctx.Done():
			s.logger.Info("scheduler.stopped")
			return
		}
	}
}
