package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"picoclip/internal/adapters/drivers"
	"picoclip/internal/adapters/events"
	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/adapters/storage/sqlite"
	"picoclip/internal/adapters/web"
	"picoclip/internal/core/ports"
	"picoclip/internal/core/services"
)

func main() {
	ctx := context.Background()

	var storage ports.Storage
	storageType := os.Getenv("PICOCLIP_STORAGE")
	if storageType == "memory" {
		log.Println("Using memory storage")
		storage = memory.NewStorage()
	} else {
		dbPath := os.Getenv("PICOCLIP_DB_PATH")
		if dbPath == "" {
			err := os.MkdirAll("data", 0755)
			if err != nil {
				log.Fatalf("Failed to create data dir: %v", err)
			}
			dbPath = filepath.Join("data", "picoclip.db")
		}
		log.Printf("Using SQLite storage at %s", dbPath)
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			log.Fatalf("Failed to open db: %v", err)
		}
		db.SetMaxOpenConns(1)
		if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;"); err != nil {
			log.Fatalf("Failed to configure db: %v", err)
		}
		sqliteStorage := sqlite.NewStorage(db)
		if err := sqliteStorage.Migrate(ctx); err != nil {
			log.Fatalf("Failed to migrate db: %v", err)
		}
		storage = sqliteStorage
	}

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
	server := web.NewServer(agentService, taskService, skillService, workspaceService, storage, bus)

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
