package services

import (
	"context"
	"errors"
	"log"
	"sync"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type Dispatcher struct {
	storage   ports.Storage
	runner    *Runner
	semaphore chan struct{}
	wg        sync.WaitGroup
}

func NewDispatcher(storage ports.Storage, runner *Runner, maxConcurrentRuns int) *Dispatcher {
	if maxConcurrentRuns < 1 {
		maxConcurrentRuns = 1
	}
	return &Dispatcher{
		storage:   storage,
		runner:    runner,
		semaphore: make(chan struct{}, maxConcurrentRuns),
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context) {
	for {
		task, err := d.storage.Tasks().ClaimNextPending(ctx)
		if err != nil {
			if !errors.Is(err, domain.ErrNoPendingTasks) {
				log.Printf("dispatch failed: %v", err)
			}
			return
		}

		select {
		case d.semaphore <- struct{}{}:
			d.wg.Add(1)
			d.runner.Run(ctx, task)
			d.wg.Done()
			<-d.semaphore
		case <-ctx.Done():
			return
		}
	}
}

func (d *Dispatcher) Wait() {
	d.wg.Wait()
}
