package services

import (
	"context"
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

func NewRuntimeManager(storage ports.Storage, baseDir string, clock ports.Clock) *RuntimeManager {
	return &RuntimeManager{
		storage:  storage,
		baseDir:  baseDir,
		clock:    clock,
		adapters: make(map[domain.RuntimeID]ports.RuntimeAdapter),
		catalog: []RuntimeManifest{
			{ID: "crush", Name: "Crush", Description: "Charmbracelet agentic coding runtime.", Kind: domain.RuntimeKindNative, Repo: "charmbracelet/crush", DocsURL: "https://github.com/charmbracelet/crush"},
			{ID: "picoclaw", Name: "PicoClaw", Description: "Sipeed ultra-lightweight Go AI assistant runtime.", Kind: domain.RuntimeKindNative, Repo: "sipeed/picoclaw", DocsURL: "https://docs.picoclaw.io/"},
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
	return m.storage.Runtimes().Delete(ctx, string(state.RuntimeID))
}

func (m *RuntimeManager) Health(ctx context.Context, id domain.RuntimeID) (domain.RuntimeHealth, error) {
	adapter, ok := m.Adapter(id)
	if !ok {
		return domain.RuntimeHealth{}, domain.ErrDriverUnavailable
	}
	state, err := m.State(ctx, id)
	if err != nil {
		return domain.RuntimeHealth{Status: "not_configured", CheckedAt: time.Now().UTC()}, nil
	}
	health := adapter.Health(ctx, state)
	state.LastHealthAt = &health.CheckedAt
	_ = m.storage.Runtimes().Save(ctx, state)
	return health, nil
}

func (m *RuntimeManager) Execute(ctx context.Context, id domain.RuntimeID, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	adapter, ok := m.Adapter(id)
	if !ok {
		return ports.RuntimeExecutionResult{}, domain.ErrDriverUnavailable
	}
	state, err := m.State(ctx, id)
	if err != nil {
		state = domain.RuntimeState{ID: "runtime_" + string(id), RuntimeID: id, Mode: domain.InstallModeExisting, Enabled: true}
	}
	return adapter.Execute(ctx, state, input)
}
