package runtimes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type ClaurstAdapter struct {
	FallbackBinary string
}

func NewClaurstAdapter(fallbackBinary string) *ClaurstAdapter {
	if fallbackBinary == "" {
		fallbackBinary = "claurst"
	}
	return &ClaurstAdapter{FallbackBinary: fallbackBinary}
}

func (a *ClaurstAdapter) ID() domain.RuntimeID     { return "claurst" }
func (a *ClaurstAdapter) Name() string             { return "Claurst" }
func (a *ClaurstAdapter) Kind() domain.RuntimeKind { return domain.RuntimeKindNative }
func (a *ClaurstAdapter) SupportedInstallModes() []domain.InstallMode {
	return []domain.InstallMode{domain.InstallModeExclusive, domain.InstallModeGlobal, domain.InstallModeExisting}
}

func (a *ClaurstAdapter) ListVersions(ctx context.Context, limit int) ([]domain.RuntimeVersion, error) {
	return listGitHubVersions(ctx, "Kuberwastaken", "claurst", limit)
}

func (a *ClaurstAdapter) Install(ctx context.Context, mode domain.InstallMode, destDir string, versionAlias string) (domain.RuntimeState, error) {
	binName := "claurst"
	if runtime.GOOS == "windows" {
		binName = "claurst.exe"
	}
	binPath := filepath.Join(destDir, "bin", binName)
	homePath := filepath.Join(destDir, "home")
	configPath := filepath.Join(homePath, "settings.json")
	logsPath := filepath.Join(destDir, "logs")
	if mode == domain.InstallModeGlobal {
		binPath = filepath.Join(userBinDir(), binName)
		home, _ := os.UserHomeDir()
		homePath = filepath.Join(home, ".claurst")
		configPath = filepath.Join(homePath, "settings.json")
		logsPath = filepath.Join(homePath, "logs")
	}

	version, sourceURL, err := installFromGitHubRelease(ctx, "Kuberwastaken", "claurst", "claurst", "claurst", versionAlias, binPath)
	if err != nil {
		if err := copyExistingBinary(a.FallbackBinary, binPath); err != nil {
			return domain.RuntimeState{}, fmt.Errorf("failed to download release and fallback failed: %w", err)
		}
	}

	if err := writeFileIfMissing(configPath, []byte("{\n  \"theme\": \"default\",\n  \"auto_update\": false\n}\n"), 0644); err != nil {
		return domain.RuntimeState{}, err
	}
	_ = os.MkdirAll(logsPath, 0755)

	state := nowState(a.ID(), mode, binPath, configPath, homePath, homePath, logsPath)
	if version != "" {
		state.Version = version
		state.SourceURL = sourceURL
		state.Source = "github_release"
	}
	return state, nil
}

func (a *ClaurstAdapter) Resolve(ctx context.Context, state domain.RuntimeState) error {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	_, err := os.Stat(bin)
	return err
}

func (a *ClaurstAdapter) Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	now := time.Now().UTC()
	health := domain.RuntimeHealth{Status: "ok", CheckedAt: now}
	if _, err := os.Stat(bin); err != nil {
		health.Status = "error"
		health.Errors = append(health.Errors, err.Error())
		health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "error", Message: err.Error(), CheckedAt: now})
		return health
	}
	health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "ok", Message: bin, CheckedAt: now})
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := commandVersion(testCtx, bin, "--version")
	if err != nil {
		version, err = commandVersion(testCtx, bin, "version")
	}
	if err != nil {
		message := err.Error()
		health.Status = "error"
		health.Errors = append(health.Errors, message)
		health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "version_command", Status: "error", Message: message, CheckedAt: now})
		return health
	}
	health.Version = extractRuntimeVersion(string(a.ID()), version)
	health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "version_command", Status: "ok", Message: health.Version, CheckedAt: now})
	if state.ConfigPath != "" {
		if _, err := os.Stat(state.ConfigPath); err != nil {
			health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "config_exists", Status: "warning", Message: err.Error(), CheckedAt: now})
		} else {
			health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "config_exists", Status: "ok", Message: state.ConfigPath, CheckedAt: now})
		}
	}
	return health
}

func (a *ClaurstAdapter) ReadConfig(ctx context.Context, state domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	if state.ConfigPath == "" {
		return nil, nil
	}
	content, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		return nil, err
	}
	return []domain.RuntimeConfigFile{{Path: state.ConfigPath, Name: "settings.json", Language: "json", Content: content, Editable: true}}, nil
}

func (a *ClaurstAdapter) WriteConfig(ctx context.Context, state domain.RuntimeState, fileName string, content []byte) error {
	if state.ConfigPath == "" {
		return fmt.Errorf("config path is not configured")
	}
	return os.WriteFile(state.ConfigPath, content, 0644)
}

func (a *ClaurstAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	bin := state.BinPath
	if input.Config["binary_path"] != "" {
		bin = input.Config["binary_path"]
	}
	if bin == "" {
		bin = a.FallbackBinary
	}
	args := []string{"-p", input.Task.Prompt}
	args = append(args, input.ExtraArgs...)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(cmd.Environ(), envPairs(input.Env)...)
	if state.HomePath != "" {
		cmd.Env = append(cmd.Env, "CLAURST_HOME="+state.HomePath)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("claurst execution failed: %w, output: %s", err, string(output))
	}
	return ports.RuntimeExecutionResult{Output: strings.TrimSpace(string(output))}, nil
}
