package runtimes

import (
	"bytes"
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

type CrushAdapter struct {
	FallbackBinary string
}

func NewCrushAdapter(fallbackBinary string) *CrushAdapter {
	if fallbackBinary == "" {
		fallbackBinary = "crush"
	}
	return &CrushAdapter{FallbackBinary: fallbackBinary}
}

func (a *CrushAdapter) ID() domain.RuntimeID     { return "crush" }
func (a *CrushAdapter) Name() string             { return "Crush" }
func (a *CrushAdapter) Kind() domain.RuntimeKind { return domain.RuntimeKindNative }
func (a *CrushAdapter) SupportedInstallModes() []domain.InstallMode {
	return []domain.InstallMode{domain.InstallModeExclusive, domain.InstallModeGlobal, domain.InstallModeExisting}
}

func (a *CrushAdapter) ListVersions(ctx context.Context, limit int) ([]domain.RuntimeVersion, error) {
	return listGitHubVersions(ctx, "charmbracelet", "crush", limit)
}

func (a *CrushAdapter) Install(ctx context.Context, mode domain.InstallMode, destDir string, versionAlias string) (domain.RuntimeState, error) {
	binName := "crush"
	if runtime.GOOS == "windows" {
		binName = "crush.exe"
	}
	binPath := filepath.Join(destDir, "bin", binName)
	configPath := filepath.Join(destDir, "config", "crush.json")
	dataPath := filepath.Join(destDir, "data")
	logsPath := filepath.Join(destDir, "logs")
	if mode == domain.InstallModeGlobal {
		binPath = filepath.Join(userBinDir(), binName)
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "crush", "crush.json")
		dataPath = filepath.Join(home, ".local", "share", "crush")
		logsPath = filepath.Join(dataPath, "logs")
	}

	version, sourceURL, err := installFromGitHubRelease(ctx, "charmbracelet", "crush", "crush", "crush", versionAlias, binPath)
	if err != nil {
		if err := copyExistingBinary(a.FallbackBinary, binPath); err != nil {
			return domain.RuntimeState{}, fmt.Errorf("failed to download release and fallback failed: %w", err)
		}
	}

	if err := writeFileIfMissing(configPath, []byte("{\n  \"$schema\": \"https://charm.land/crush.json\",\n  \"options\": {\n    \"disable_metrics\": true\n  }\n}\n"), 0644); err != nil {
		return domain.RuntimeState{}, err
	}
	_ = os.MkdirAll(dataPath, 0755)
	_ = os.MkdirAll(logsPath, 0755)

	state := nowState(a.ID(), mode, binPath, configPath, "", dataPath, logsPath)
	if version != "" {
		state.Version = version
		state.SourceURL = sourceURL
		state.Source = "github_release"
	}
	return state, nil
}

func (a *CrushAdapter) Resolve(ctx context.Context, state domain.RuntimeState) error {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	_, err := resolveConfiguredExecutable(bin)
	return err
}

func (a *CrushAdapter) Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth {
	bin := state.BinPath
	if bin == "" {
		bin = a.FallbackBinary
	}
	now := time.Now().UTC()
	health := domain.RuntimeHealth{Status: "ok", CheckedAt: now}
	canonical, err := resolveConfiguredExecutable(bin)
	if err != nil {
		health.Status = "error"
		health.Errors = append(health.Errors, err.Error())
		health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "error", Message: err.Error(), CheckedAt: now})
		return health
	}
	bin = canonical
	health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "binary_exists", Status: "ok", Message: bin, CheckedAt: now})
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := commandVersion(testCtx, bin, "--version")
	if err != nil {
		version, err = commandVersion(testCtx, bin, "version")
	}
	if err != nil {
		health.Status = "error"
		health.Errors = append(health.Errors, err.Error())
		health.Checks = append(health.Checks, domain.DiagnosticCheck{Name: "version_command", Status: "error", Message: err.Error(), CheckedAt: now})
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

func (a *CrushAdapter) ReadConfig(ctx context.Context, state domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	if state.ConfigPath == "" {
		return nil, nil
	}
	content, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		return nil, err
	}
	return []domain.RuntimeConfigFile{{Path: state.ConfigPath, Name: "crush.json", Language: "json", Content: content, Editable: true}}, nil
}

func (a *CrushAdapter) WriteConfig(ctx context.Context, state domain.RuntimeState, fileName string, content []byte) error {
	if state.ConfigPath == "" {
		return fmt.Errorf("config path is not configured")
	}
	return atomicWriteFile(state.ConfigPath, content, secureConfigMode(state.ConfigPath))
}

func (a *CrushAdapter) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	bin := state.BinPath
	if input.Config["binary_path"] != "" {
		bin = input.Config["binary_path"]
	}
	if bin == "" {
		bin = a.FallbackBinary
	}
	canonical, err := resolveConfiguredExecutable(bin)
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}
	bin = canonical
	args := []string{"run"}
	args = append(args, input.ExtraArgs...)
	args = append(args, input.Task.Prompt)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(cmd.Environ(), envPairs(input.Env)...)
	if input.Task.WorkspaceID != "" {
		cmd.Env = append(cmd.Env, "PICOCLIP_WORKSPACE_ID="+input.Task.WorkspaceID)
	}
	if state.ConfigPath != "" {
		cmd.Env = append(cmd.Env, "CRUSH_GLOBAL_CONFIG="+filepath.Dir(state.ConfigPath))
	}
	if state.DataPath != "" {
		cmd.Env = append(cmd.Env, "CRUSH_GLOBAL_DATA="+state.DataPath)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}

	if err := startRuntimeCommand(cmd); err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("crush start failed: %w", err)
	}
	if input.OnStart != nil && cmd.Process != nil {
		input.OnStart(cmd.Process.Pid)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	done := make(chan bool, 2)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				stdoutBuf.Write(chunk)
				if input.OnOutput != nil {
					input.OnOutput(chunk, nil)
				}
			}
			if err != nil {
				break
			}
		}
		done <- true
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				stderrBuf.Write(chunk)
				if input.OnOutput != nil {
					input.OnOutput(nil, chunk)
				}
			}
			if err != nil {
				break
			}
		}
		done <- true
	}()

	<-done
	<-done

	err = cmd.Wait()

	if input.OnOutput != nil {
		input.OnOutput(stdoutBuf.Bytes(), stderrBuf.Bytes())
	}

	if err != nil {
		return ports.RuntimeExecutionResult{}, fmt.Errorf("crush execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderrBuf.String()))
	}
	return ports.RuntimeExecutionResult{Output: strings.TrimSpace(stdoutBuf.String())}, nil
}

func (a *CrushAdapter) Cancel(ctx context.Context, state domain.RuntimeState, run domain.Run) error {
	if run.ProcessID <= 0 {
		return nil
	}
	return cancelRuntimeProcess(ctx, run.ProcessID)
}
