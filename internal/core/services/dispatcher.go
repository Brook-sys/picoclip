package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type Dispatcher struct {
	storage   ports.Storage
	runner    *Runner
	semaphore chan struct{}
	wg        sync.WaitGroup
	logger    ports.Logger
}

func NewDispatcher(storage ports.Storage, runner *Runner, logger ports.Logger, maxConcurrentRuns int) *Dispatcher {
	if maxConcurrentRuns < 1 {
		maxConcurrentRuns = 1
	}
	return &Dispatcher{
		storage:   storage,
		runner:    runner,
		semaphore: make(chan struct{}, maxConcurrentRuns),
		logger:    logger,
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context) {
	for {
		select {
		case d.semaphore <- struct{}{}:
		case <-ctx.Done():
			return
		}

		task, run, err := d.storage.Tasks().ClaimNextRunnable(ctx, time.Now(), 30*time.Minute)
		if err != nil {
			<-d.semaphore
			if !errors.Is(err, domain.ErrNoPendingTasks) {
				d.logger.Warn("dispatcher.claim_failed", "err", err)
			} else {
				d.logger.Debug("dispatcher.no_pending_tasks")
			}
			return
		}

		d.logger.Debug("dispatcher.task_claimed", "task_id", task.ID, "agent_id", task.AgentID, "run_id", run.ID)
		d.wg.Add(1)
		go func(t domain.Task) {
			defer d.wg.Done()
			defer func() { <-d.semaphore }()
			d.runner.Run(ctx, t)
		}(task)
	}
}

func (d *Dispatcher) Wait() {
	d.wg.Wait()
}
