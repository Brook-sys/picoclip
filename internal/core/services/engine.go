package services

import (
	"context"
	"sync"

	"picoclip/internal/core/ports"
)

type Engine struct {
	scheduler *Scheduler
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	logger    ports.Logger
}

func NewEngine(storage ports.Storage, bus ports.EventBus, runtimes *RuntimeManager, memory ports.MemoryProvider, logger ports.Logger, config Config) *Engine {
	clock := SystemClock{}
	idGen := &TimeIDGenerator{}
	runner := NewRunner(storage, clock, idGen, bus, runtimes, memory, logger, config)
	dispatcher := NewDispatcher(storage, runner, logger, config.MaxConcurrentRuns)
	reconciler := NewReconciler(storage, clock, bus, idGen, logger)
	reconciler.SetCanceler(runtimes)
	scheduler := NewScheduler(config.PollInterval, dispatcher, reconciler, logger)
	return &Engine{scheduler: scheduler, logger: logger}
}

func (e *Engine) Start(ctx context.Context) {
	engineCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.scheduler.Start(engineCtx)
	}()
}

func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
}
