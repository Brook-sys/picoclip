package services

import (
	"context"
	"time"

	"picoclip/internal/core/ports"
)

type Scheduler struct {
	interval   time.Duration
	dispatcher *Dispatcher
	logger     ports.Logger
}

func NewScheduler(interval time.Duration, dispatcher *Dispatcher, logger ports.Logger) *Scheduler {
	return &Scheduler{interval: interval, dispatcher: dispatcher, logger: logger}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.dispatcher.Dispatch(ctx)
	for {
		select {
		case <-ticker.C:
			s.dispatcher.Dispatch(ctx)
		case <-ctx.Done():
			s.logger.Info("scheduler.stopped")
			return
		}
	}
}
