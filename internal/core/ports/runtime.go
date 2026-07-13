package ports

import (
	"context"
	"errors"

	"picoclip/internal/core/domain"
)

type RuntimeExecutionInput struct {
	Agent     domain.Agent
	Task      domain.Task
	Run       domain.Run
	Memory    string
	Config    map[string]string
	Env       map[string]string
	ExtraArgs []string
	OnStart   func(pid int)
	OnOutput  func(stdout, stderr []byte)
}

type RuntimeExecutionResult struct {
	Output string
	Logs   []string
	Data   map[string]string
}

type RuntimeAdapter interface {
	ID() domain.RuntimeID
	Name() string
	Kind() domain.RuntimeKind
	SupportedInstallModes() []domain.InstallMode
	ListVersions(ctx context.Context, limit int) ([]domain.RuntimeVersion, error)
	Install(ctx context.Context, mode domain.InstallMode, destDir string, versionAlias string) (domain.RuntimeState, error)
	Resolve(ctx context.Context, state domain.RuntimeState) error
	Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth
	ReadConfig(ctx context.Context, state domain.RuntimeState) ([]domain.RuntimeConfigFile, error)
	WriteConfig(ctx context.Context, state domain.RuntimeState, fileName string, content []byte) error
	Execute(ctx context.Context, state domain.RuntimeState, input RuntimeExecutionInput) (RuntimeExecutionResult, error)
	Cancel(ctx context.Context, state domain.RuntimeState, run domain.Run) error
}

type RuntimeQuickConfigurator interface {
	QuickSetupSchema() domain.RuntimeQuickSetupSchema
	ReadQuickSetup(ctx context.Context, state domain.RuntimeState) (domain.RuntimeQuickSetupView, error)
	ApplyQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) error
	TestQuickSetup(ctx context.Context, state domain.RuntimeState, input domain.RuntimeQuickSetupInput) (domain.RuntimeModelTestResult, error)
}

type RuntimeExistingPathsResolver interface {
	ResolveExistingPaths(binPath string) domain.RuntimeState
}

var ErrSandboxCommandRequired = errors.New("sandbox command is required")

type SandboxCommand struct {
	Command   string
	Args      []string
	Env       []string
	Workspace string
	OnStart   func(pid int)
	OnOutput  func(stdout, stderr []byte)
}

type SandboxResult struct {
	Output string
}

// Sandbox executes an explicitly configured command in an isolated environment.
// Implementations must fail closed: they must never fall back to a host command.
type Sandbox interface {
	Isolate(ctx context.Context, command SandboxCommand) (SandboxResult, error)
}

type RuntimeRepository interface {
	GetByRuntimeID(ctx context.Context, runtimeID domain.RuntimeID) (domain.RuntimeState, error)
	List(ctx context.Context) ([]domain.RuntimeState, error)
	Save(ctx context.Context, state domain.RuntimeState) error
	Delete(ctx context.Context, id string) error
}
