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
}

func NewEngine(storage ports.Storage, bus ports.EventBus, runtimes *RuntimeManager, memory ports.MemoryProvider, config Config) *Engine {
	clock := SystemClock{}
	idGen := &TimeIDGenerator{}
	runner := NewRunner(storage, clock, idGen, bus, runtimes, memory, config)
	dispatcher := NewDispatcher(storage, runner, config.MaxConcurrentRuns)
	scheduler := NewScheduler(config.PollInterval, dispatcher)
	return &Engine{scheduler: scheduler}
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
