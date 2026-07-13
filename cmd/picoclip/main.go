package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"

	"picoclip/internal/adapters/events"
	"picoclip/internal/adapters/logger"
	"picoclip/internal/adapters/runtimes"
	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/adapters/storage/sqlite"
	"picoclip/internal/adapters/web"
	"picoclip/internal/core/ports"
	"picoclip/internal/core/services"
)

func resolveBind(configured string) string {
	if configured == "" {
		return "127.0.0.1"
	}
	return configured
}

func main() {
	ctx := context.Background()
	debugMode := os.Getenv("PICOCLIP_DEBUG") == "true" || os.Getenv("PICOCLIP_DEBUG") == "1"
	logLevel := os.Getenv("PICOCLIP_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	appLogger := logger.NewSlogLogger(logLevel, debugMode)

	var storage ports.Storage
	storageType := os.Getenv("PICOCLIP_STORAGE")
	storageTypeForDiagnostics := storageType
	if storageTypeForDiagnostics == "" {
		storageTypeForDiagnostics = "sqlite"
	}
	dbPathForDiagnostics := os.Getenv("PICOCLIP_DB_PATH")
	if storageType == "memory" {
		appLogger.Info("storage.selected", "type", "memory")
		storage = memory.NewStorage()
	} else {
		dbPath := os.Getenv("PICOCLIP_DB_PATH")
		if dbPath == "" {
			err := os.MkdirAll("data", 0755)
			if err != nil {
				appLogger.Error("storage.mkdir_failed", "err", err)
				os.Exit(1)
			}
			dbPath = filepath.Join("data", "picoclip.db")
		}
		dbPathForDiagnostics = dbPath
		appLogger.Info("storage.selected", "type", "sqlite", "path", dbPath)
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			appLogger.Error("storage.open_failed", "err", err)
			os.Exit(1)
		}
		db.SetMaxOpenConns(1)
		if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;"); err != nil {
			appLogger.Error("storage.pragma_failed", "err", err)
			os.Exit(1)
		}
		sqliteStorage := sqlite.NewStorage(db)
		if err := sqliteStorage.Migrate(ctx); err != nil {
			appLogger.Error("storage.migrate_failed", "err", err)
			os.Exit(1)
		}
		storage = sqliteStorage
	}

	bus := events.NewInMemoryBus()
	clock := services.SystemClock{}
	idGen := &services.TimeIDGenerator{}
	runtimeBase := os.Getenv("PICOCLIP_RUNTIMES")
	if runtimeBase == "" {
		runtimeBase = filepath.Join("data", "runtimes")
	}
	runtimeManager := services.NewRuntimeManager(storage, runtimeBase, clock)

	crushPath := os.Getenv("CRUSH_PATH")
	if crushPath == "" {
		crushPath = "crush"
	}
	picoclawPath := os.Getenv("PICOCLAW_PATH")
	if picoclawPath == "" {
		picoclawPath = "picoclaw"
	}
	claurstPath := os.Getenv("CLAURST_PATH")
	if claurstPath == "" {
		claurstPath = "claurst"
	}
	bwrapPath := os.Getenv("BWRAP_PATH")
	if bwrapPath == "" {
		bwrapPath = "bwrap"
	}
	workspaceBase := os.Getenv("PICOCLIP_WORKSPACES")
	if workspaceBase == "" {
		workspaceBase = "workspaces"
	}
	workspaceRoot, err := filepath.Abs(workspaceBase)
	if err != nil {
		appLogger.Error("workspace.path_invalid", "err", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		appLogger.Error("workspace.mkdir_failed", "err", err)
		os.Exit(1)
	}
	runtimeManager.Register(runtimes.NewCrushAdapter(crushPath))
	runtimeManager.Register(runtimes.NewPicoClawAdapter(picoclawPath))
	runtimeManager.Register(runtimes.NewClaurstAdapter(claurstPath))
	runtimeManager.Register(runtimes.NewBwrapRuntimeWithBinaryAndWorkspaceRoots(bwrapPath, []string{workspaceRoot}))

	config := services.DefaultConfig()
	engine := services.NewEngine(storage, bus, runtimeManager, services.NoopMemoryProvider{}, appLogger, config)
	engine.Start(ctx)
	defer engine.Stop()
	go runtimeManager.TestAllConfigured(context.Background(), appLogger)

	agentService := services.NewAgentService(storage, clock, idGen)
	taskService := services.NewTaskService(storage, clock, idGen, bus)
	taskService.SetCanceler(runtimeManager)
	taskService.SetCompletionAuditor(services.NewRuntimeCompletionAuditor(storage, runtimeManager))
	workspaceService := services.NewWorkspaceService(storage, clock, idGen, workspaceRoot)
	_, _ = workspaceService.EnsureDefault(ctx)
	skillService := services.NewSkillService(storage, clock, idGen)
	_ = skillService.InstallBuiltins(ctx)

	outboxWorker := services.NewOutboxWorker(storage, bus)
	go outboxWorker.Start(ctx)
	webhookWorker := services.NewWebhookDeliveryWorker(storage, clock, nil)
	go webhookWorker.Start(ctx)

	diagnostics := services.NewDiagnosticsService(storage, runtimeManager, services.DiagnosticsConfig{StorageType: storageTypeForDiagnostics, DatabasePath: dbPathForDiagnostics, WorkspacePath: workspaceBase, RuntimePath: runtimeBase, LogLevel: logLevel, DebugMode: debugMode})
	server := web.NewServer(agentService, taskService, skillService, workspaceService, runtimeManager, diagnostics, storage, bus, debugMode)

	mux := http.NewServeMux()
	server.Mount(mux)

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	bind := resolveBind(os.Getenv("BIND"))

	listenAddr := bind + ":" + addr
	appLogger.Info("server.start", "addr", listenAddr, "debug", debugMode, "log_level", logLevel, "runtime_path", runtimeBase, "workspace_path", workspaceBase)
	if err := http.ListenAndServe(listenAddr, mux); err != nil {
		appLogger.Error("server.failed", "err", err)
		os.Exit(1)
	}
}
