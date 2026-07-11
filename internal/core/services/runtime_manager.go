package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type RuntimeManifest struct {
	ID          domain.RuntimeID
	Name        string
	Description string
	Kind        domain.RuntimeKind
	Repo        string
	DocsURL     string
}

type RuntimeManager struct {
	storage  ports.Storage
	baseDir  string
	clock    ports.Clock
	adapters map[domain.RuntimeID]ports.RuntimeAdapter
	catalog  []RuntimeManifest
}

type RuntimeAITestResult struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Output    string    `json:"output,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

func NewRuntimeManager(storage ports.Storage, baseDir string, clock ports.Clock) *RuntimeManager {
	return &RuntimeManager{
		storage:  storage,
		baseDir:  baseDir,
		clock:    clock,
		adapters: make(map[domain.RuntimeID]ports.RuntimeAdapter),
		catalog: []RuntimeManifest{
			{ID: "crush", Name: "Crush", Description: "Charmbracelet agentic coding runtime.", Kind: domain.RuntimeKindNative, Repo: "charmbracelet/crush", DocsURL: "https://github.com/charmbracelet/crush"},
			{ID: "picoclaw", Name: "PicoClaw", Description: "Sipeed ultra-lightweight Go AI assistant runtime.", Kind: domain.RuntimeKindNative, Repo: "sipeed/picoclaw", DocsURL: "https://docs.picoclaw.io/"},
			{ID: "claurst", Name: "Claurst", Description: "Open-source multi-provider terminal coding agent built in Rust.", Kind: domain.RuntimeKindNative, Repo: "Kuberwastaken/claurst", DocsURL: "https://claurst.kuber.studio/docs"},
			{ID: "bwrap", Name: "Bubblewrap Sandbox", Description: "Isolated fail-closed application container runtime.", Kind: domain.RuntimeKindSandbox, Repo: "containers/bubblewrap", DocsURL: "https://github.com/containers/bubblewrap"},
		},
	}
}

func (m *RuntimeManager) Register(adapter ports.RuntimeAdapter) {
	m.adapters[adapter.ID()] = adapter
}

func (m *RuntimeManager) Catalog() []RuntimeManifest {
	return append([]RuntimeManifest(nil), m.catalog...)
}

func (m *RuntimeManager) Adapter(id domain.RuntimeID) (ports.RuntimeAdapter, bool) {
	adapter, ok := m.adapters[id]
	return adapter, ok
}

func (m *RuntimeManager) State(ctx context.Context, id domain.RuntimeID) (domain.RuntimeState, error) {
	return m.storage.Runtimes().GetByRuntimeID(ctx, id)
}

func (m *RuntimeManager) States(ctx context.Context) (map[domain.RuntimeID]domain.RuntimeState, error) {
	states, err := m.storage.Runtimes().List(ctx)
	if err != nil {
		return nil, err
	}
	res := make(map[domain.RuntimeID]domain.RuntimeState, len(states))
	for _, state := range states {
		res[state.RuntimeID] = state
	}
	return res, nil
}

func (m *RuntimeManager) ConfigureExisting(ctx context.Context, id domain.RuntimeID, binPath string) (domain.RuntimeState, error) {
	adapter, ok := m.Adapter(id)
	if !ok {
		return domain.RuntimeState{}, domain.ErrDriverUnavailable
	}
	now := m.clock.Now()
	state := domain.RuntimeState{
		ID:           "runtime_" + string(id),
		RuntimeID:    id,
		Mode:         domain.InstallModeExisting,
		Enabled:      true,
		BinPath:      binPath,
		InstalledAt:  now,
		UpdatedAt:    now,
		SettingsJSON: "{}",
		MetadataJSON: "{}",
	}
	if err := adapter.Resolve(ctx, state); err != nil {
		return domain.RuntimeState{}, err
	}
	if err := m.storage.Runtimes().Save(ctx, state); err != nil {
		return domain.RuntimeState{}, err
	}
	return state, nil
}

func (m *RuntimeManager) Install(ctx context.Context, id domain.RuntimeID, mode domain.InstallMode, versionAlias string) (domain.RuntimeState, error) {
	adapter, ok := m.Adapter(id)
	if !ok {
		return domain.RuntimeState{}, domain.ErrDriverUnavailable
	}
	if mode != domain.InstallModeExclusive && mode != domain.InstallModeGlobal {
		return domain.RuntimeState{}, errors.New("unsupported install mode")
	}
	state, err := adapter.Install(ctx, mode, filepath.Join(m.baseDir, string(id)), versionAlias)
	if err != nil {
		return domain.RuntimeState{}, err
	}
	state.ID = "runtime_" + string(id)
	state.RuntimeID = id
	state.Mode = mode
	state.Enabled = true
	if state.InstalledAt.IsZero() {
		state.InstalledAt = m.clock.Now()
	}
	state.UpdatedAt = m.clock.Now()
	if state.SettingsJSON == "" {
		state.SettingsJSON = "{}"
	}
	if state.MetadataJSON == "" {
		state.MetadataJSON = "{}"
	}
	if err := m.storage.Runtimes().Save(ctx, state); err != nil {
		return domain.RuntimeState{}, err
	}
	return state, nil
}

func (m *RuntimeManager) Uninstall(ctx context.Context, id domain.RuntimeID) error {
	state, err := m.State(ctx, id)
	if err != nil {
		return err
	}
	if state.Mode == domain.InstallModeExclusive {
		_ = os.RemoveAll(filepath.Join(m.baseDir, string(id)))
	}
	return m.storage.Runtimes().Delete(ctx, state.ID)
}

func (m *RuntimeManager) SetEnabled(ctx context.Context, id domain.RuntimeID, enabled bool) (domain.RuntimeState, error) {
	state, err := m.State(ctx, id)
	if err != nil {
		return domain.RuntimeState{}, err
	}
	state.Enabled = enabled
	state.UpdatedAt = m.clock.Now()
	if err := m.storage.Runtimes().Save(ctx, state); err != nil {
		return domain.RuntimeState{}, err
	}
	return state, nil
}

func (m *RuntimeManager) InstalledRuntimeIDs(ctx context.Context) []domain.RuntimeID {
	states, err := m.storage.Runtimes().List(ctx)
	if err != nil {
		return nil
	}
	ids := make([]domain.RuntimeID, 0, len(states))
	for _, s := range states {
		ids = append(ids, s.RuntimeID)
	}
	return ids
}

func (m *RuntimeManager) Test(ctx context.Context, id domain.RuntimeID) (domain.RuntimeHealth, error) {
	adapter, ok := m.Adapter(id)
	if !ok {
		return domain.RuntimeHealth{}, domain.ErrDriverUnavailable
	}
	state, err := m.State(ctx, id)
	if err != nil {
		return domain.RuntimeHealth{Status: "not_configured", CheckedAt: time.Now().UTC()}, nil
	}
	health := adapter.Health(ctx, state)
	if health.CheckedAt.IsZero() {
		health.CheckedAt = m.clock.Now()
	}
	state.LastHealthAt = &health.CheckedAt
	if raw, err := json.Marshal(health); err == nil {
		state.LastHealthJSON = string(raw)
	}
	_ = m.storage.Runtimes().Save(ctx, state)
	return health, nil
}

func (m *RuntimeManager) TestAllConfigured(ctx context.Context, logger ports.Logger) {
	states, err := m.storage.Runtimes().List(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("runtime.test_all.list_failed", "err", err)
		}
		return
	}
	for _, state := range states {
		if _, err := m.Test(ctx, state.RuntimeID); err != nil && logger != nil {
			logger.Warn("runtime.test_failed", "runtime_id", state.RuntimeID, "err", err)
		}
	}
}

func (m *RuntimeManager) Health(ctx context.Context, id domain.RuntimeID) (domain.RuntimeHealth, error) {
	return m.Test(ctx, id)
}

func (m *RuntimeManager) TestAI(ctx context.Context, id domain.RuntimeID) (RuntimeAITestResult, error) {
	state, err := m.State(ctx, id)
	if err != nil {
		return RuntimeAITestResult{Status: "error", Message: "Not installed"}, nil
	}
	testCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	now := m.clock.Now()
	res, err := m.Execute(testCtx, id, ports.RuntimeExecutionInput{
		Agent: domain.Agent{Name: "System AI Tester", Type: domain.AgentType(id)},
		Task:  domain.Task{Prompt: "Say exactly and only the word PONG"},
		Run:   domain.Run{ID: "run_test_ai"},
	})

	result := RuntimeAITestResult{CheckedAt: now}
	if err != nil {
		result.Status = "error"
		result.Message = "AI request failed"
		result.Output = err.Error()
	} else {
		result.Status = "ok"
		result.Message = "AI request successful"
		result.Output = res.Output
	}

	metadata := make(map[string]any)
	if state.MetadataJSON != "" && state.MetadataJSON != "{}" {
		_ = json.Unmarshal([]byte(state.MetadataJSON), &metadata)
	}
	metadata["last_ai_test"] = result
	if raw, err := json.Marshal(metadata); err == nil {
		state.MetadataJSON = string(raw)
		_ = m.storage.Runtimes().Save(ctx, state)
	}
	return result, nil
}

func (m *RuntimeManager) Execute(ctx context.Context, id domain.RuntimeID, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	adapter, ok := m.Adapter(id)
	if !ok {
		return ports.RuntimeExecutionResult{}, domain.ErrDriverUnavailable
	}
	state, err := m.State(ctx, id)
	if err != nil {
		state = domain.RuntimeState{ID: "runtime_" + string(id), RuntimeID: id, Mode: domain.InstallModeExisting, Enabled: true}
	} else if !state.Enabled {
		return ports.RuntimeExecutionResult{}, errors.New("runtime is disabled")
	}
	return adapter.Execute(ctx, state, input)
}

func (m *RuntimeManager) CancelRun(ctx context.Context, run domain.Run) error {
	id := domain.RuntimeID(run.DriverType)
	return m.Cancel(ctx, id, run)
}

func (m *RuntimeManager) Cancel(ctx context.Context, id domain.RuntimeID, run domain.Run) error {
	adapter, ok := m.Adapter(id)
	if !ok {
		return domain.ErrDriverUnavailable
	}
	state, err := m.State(ctx, id)
	if err != nil {
		// Se não tiver state, mas tiver adapter e run, tentamos cancelar passando fallback
		state = domain.RuntimeState{ID: "runtime_" + string(id), RuntimeID: id, Mode: domain.InstallModeExisting, Enabled: true}
	}
	return adapter.Cancel(ctx, state, run)
}
