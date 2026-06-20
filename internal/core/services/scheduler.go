package services

import (
	"context"
	"log"
	"time"
)

type Scheduler struct {
	interval   time.Duration
	dispatcher *Dispatcher
}

func NewScheduler(interval time.Duration, dispatcher *Dispatcher) *Scheduler {
	return &Scheduler{interval: interval, dispatcher: dispatcher}
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
			log.Println("scheduler stopped")
			return
		}
	}
}
