package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"picoclip/internal/adapters/drivers"
	"picoclip/internal/adapters/events"
	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/adapters/web"
	"picoclip/internal/core/services"
)

func main() {
	ctx := context.Background()
	storage := memory.NewStorage()
	bus := events.NewInMemoryBus()
	clock := services.SystemClock{}
	idGen := &services.TimeIDGenerator{}
	registry := services.NewDriverRegistry()

	crushPath := os.Getenv("CRUSH_PATH")
	if crushPath == "" {
		crushPath = filepath.Join(os.Getenv("HOME"), "crush", "crush")
	}
	registry.Register(drivers.NewCrushDriver(crushPath))
	registry.Register(&drivers.NoopDriver{})

	config := services.DefaultConfig()
	engine := services.NewEngine(storage, bus, registry, services.NoopMemoryProvider{}, config)
	engine.Start(ctx)
	defer engine.Stop()

	agentService := services.NewAgentService(storage, clock, idGen)
	taskService := services.NewTaskService(storage, clock, idGen, bus)
	workspaceBase := os.Getenv("PICOCLIP_WORKSPACES")
	if workspaceBase == "" {
		workspaceBase = "workspaces"
	}
	workspaceService := services.NewWorkspaceService(storage, clock, idGen, workspaceBase)
	_, _ = workspaceService.EnsureDefault(ctx)
	skillService := services.NewSkillService(storage, clock, idGen)
	_ = skillService.InstallBuiltins(ctx)
	server := web.NewServer(agentService, taskService, skillService, workspaceService, storage)

	mux := http.NewServeMux()
	server.Mount(mux)

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	bind := os.Getenv("BIND")
	if bind == "" {
		bind = "0.0.0.0"
	}

	listenAddr := bind + ":" + addr
	log.Printf("PicoClip running at http://%s", listenAddr)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}
