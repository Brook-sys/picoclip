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
	"sync"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

const bwrapCommandConfigKey = "sandbox_command"
const bwrapWorkspaceConfigKey = "sandbox_workspace_path"

var bwrapCommandRoots = []string{"/bin", "/usr/bin"}

type BwrapSandbox struct {
	binary         string
	workspaceRoots []string
}

// NewBwrapSandbox permits no writable workspace. Callers that need one must pass
// canonical, operator-controlled roots to NewBwrapSandboxWithWorkspaceRoots.
func NewBwrapSandbox(binary string) *BwrapSandbox {
	return NewBwrapSandboxWithWorkspaceRoots(binary, nil)
}

func NewBwrapSandboxWithWorkspaceRoots(binary string, workspaceRoots []string) *BwrapSandbox {
	return &BwrapSandbox{binary: binary, workspaceRoots: canonicalDirectories(workspaceRoots)}
}

func (s *BwrapSandbox) Isolate(ctx context.Context, command ports.SandboxCommand) (ports.SandboxResult, error) {
	if err := requireLinux(runtime.GOOS); err != nil {
		return ports.SandboxResult{}, err
	}
	_, commandFile, err := canonicalExecutable(command.Command, bwrapCommandRoots)
	if err != nil {
		return ports.SandboxResult{}, err
	}
	defer commandFile.Close()

	binary, err := canonicalBinary(s.binary)
	if err != nil {
		return ports.SandboxResult{}, err
	}

	args := minimalBwrapArgs()
	// Freeze the empty root after mounting the runtime. Later bind mounts restore
	// writability only for the explicitly approved workspace.
	args = append(args, "--remount-ro", "/")
	extraFiles := []*os.File{commandFile}
	const sandboxCommandPath = "/command"
	args = append(args, "--ro-bind", "/proc/self/fd/3", sandboxCommandPath)
	if command.Workspace != "" {
		_, workspaceFile, err := s.authorizedWorkspace(command.Workspace)
		if err != nil {
			return ports.SandboxResult{}, err
		}
		defer workspaceFile.Close()
		extraFiles = append(extraFiles, workspaceFile)
		// The inherited descriptor pins the directory selected during validation,
		// avoiding a path re-resolution race between validation and bwrap's bind.
		// A fixed destination avoids recreating an arbitrary host path in the root.
		args = append(args, "--dir", "/workspace", "--bind", "/proc/self/fd/4", "/workspace", "--chdir", "/workspace")
	}
	args = append(args, "--clearenv", "--setenv", "PATH", "/usr/bin:/bin", "--setenv", "HOME", "/tmp")
	for _, value := range command.Env {
		key, val, err := sandboxEnv(value)
		if err != nil {
			return ports.SandboxResult{}, err
		}
		args = append(args, "--setenv", key, val)
	}
	args = append(args, "--", sandboxCommandPath)
	args = append(args, command.Args...)

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "LANG=C", "TMPDIR=/tmp"}
	cmd.ExtraFiles = extraFiles
	var callbackMu sync.Mutex
	stdout := &sandboxOutputWriter{onOutput: command.OnOutput, callbackMu: &callbackMu}
	stderr := &sandboxOutputWriter{stderr: true, onOutput: command.OnOutput, callbackMu: &callbackMu}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := startRuntimeCommand(cmd); err != nil {
		return ports.SandboxResult{}, fmt.Errorf("sandbox start failed: %w", err)
	}
	if command.OnStart != nil && cmd.Process != nil {
		command.OnStart(cmd.Process.Pid)
	}
	err = cmd.Wait()
	if err != nil {
		return ports.SandboxResult{}, fmt.Errorf("sandbox execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ports.SandboxResult{Output: strings.TrimSpace(stdout.String())}, nil
}

type sandboxOutputWriter struct {
	buffer     bytes.Buffer
	stderr     bool
	onOutput   func(stdout, stderr []byte)
	callbackMu *sync.Mutex
}

func (w *sandboxOutputWriter) Write(chunk []byte) (int, error) {
	n, err := w.buffer.Write(chunk)
	if n > 0 && w.onOutput != nil {
		copied := append([]byte(nil), chunk[:n]...)
		w.callbackMu.Lock()
		if w.stderr {
			w.onOutput(nil, copied)
		} else {
			w.onOutput(copied, nil)
		}
		w.callbackMu.Unlock()
	}
	return n, err
}

func (w *sandboxOutputWriter) String() string {
	return w.buffer.String()
}

func minimalBwrapArgs() []string {
	args := []string{
		"--unshare-all", "--die-with-parent", "--new-session",
		"--tmpfs", "/",
		"--dir", "/usr", "--dir", "/usr/bin", "--ro-bind", "/usr/bin", "/usr/bin",
		"--dir", "/lib", "--ro-bind", "/lib", "/lib",
		"--dir", "/proc", "--proc", "/proc",
		"--dir", "/dev", "--dev", "/dev",
		"--dir", "/tmp", "--tmpfs", "/tmp",
	}
	if info, err := os.Stat("/lib64"); err == nil && info.IsDir() {
		args = append(args, "--dir", "/lib64", "--ro-bind", "/lib64", "/lib64")
	}
	if info, err := os.Stat("/usr/lib"); err == nil && info.IsDir() {
		args = append(args, "--dir", "/usr/lib", "--ro-bind", "/usr/lib", "/usr/lib")
	}
	return args
}

func (s *BwrapSandbox) authorizedWorkspace(path string) (string, *os.File, error) {
	if len(s.workspaceRoots) == 0 {
		return "", nil, fmt.Errorf("sandbox workspace is not enabled")
	}
	workspace, err := canonicalDirectory(path)
	if err != nil {
		return "", nil, fmt.Errorf("sandbox workspace unavailable: %w", err)
	}
	allowed := false
	for _, root := range s.workspaceRoots {
		if pathWithin(root, workspace) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil, fmt.Errorf("sandbox workspace is outside authorized roots: %q", workspace)
	}
	file, err := os.Open(workspace)
	if err != nil {
		return "", nil, fmt.Errorf("sandbox workspace open: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.IsDir() {
		file.Close()
		return "", nil, fmt.Errorf("sandbox workspace is not a directory: %q", workspace)
	}
	return workspace, file, nil
}

func canonicalExecutable(path string, allowedRoots []string) (string, *os.File, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil, ports.ErrSandboxCommandRequired
	}
	if !filepath.IsAbs(path) {
		return "", nil, fmt.Errorf("sandbox command must be an absolute path: %q", path)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, fmt.Errorf("sandbox command unavailable: %w", err)
	}
	allowed := false
	for _, root := range canonicalDirectories(allowedRoots) {
		if pathWithin(root, canonical) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil, fmt.Errorf("sandbox command is outside approved executable roots: %q", canonical)
	}
	file, err := os.Open(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("sandbox command open: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		file.Close()
		return "", nil, fmt.Errorf("sandbox command is not an executable regular file: %q", canonical)
	}
	return canonical, file, nil
}

func canonicalBinary(binary string) (string, error) {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return "", fmt.Errorf("sandbox unavailable: bwrap binary is not configured")
	}
	if !filepath.IsAbs(binary) {
		resolved, err := exec.LookPath(binary)
		if err != nil {
			return "", fmt.Errorf("sandbox unavailable: bwrap binary: %w", err)
		}
		binary = resolved
	}
	canonical, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return "", fmt.Errorf("sandbox unavailable: bwrap binary: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("sandbox unavailable: bwrap binary is not executable: %q", canonical)
	}
	return canonical, nil
}

func canonicalDirectories(paths []string) []string {
	roots := make([]string, 0, len(paths))
	for _, path := range paths {
		if canonical, err := canonicalDirectory(path); err == nil {
			roots = append(roots, canonical)
		}
	}
	return roots
}

func canonicalDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute: %q", path)
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %q", canonical)
	}
	return canonical, nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func sandboxEnv(value string) (string, string, error) {
	key, val, ok := strings.Cut(value, "=")
	if !ok || key == "" || strings.ContainsAny(key, "=\x00\n\r") || strings.ContainsAny(val, "\x00") {
		return "", "", fmt.Errorf("invalid sandbox environment entry %q", value)
	}
	if strings.HasPrefix(key, "LD_") || key == "BASH_ENV" || key == "ENV" {
		return "", "", fmt.Errorf("sandbox environment entry is not permitted: %q", key)
	}
	return key, val, nil
}

func requireLinux(goos string) error {
	if goos != "linux" {
		return fmt.Errorf("bubblewrap sandbox is only supported on linux (current platform: %s)", goos)
	}
	return nil
}

type BwrapRuntime struct {
	sandbox ports.Sandbox
	binary  string
	roots   []string
}

func NewBwrapRuntime(sandbox ...ports.Sandbox) *BwrapRuntime {
	if len(sandbox) > 0 && sandbox[0] != nil {
		return &BwrapRuntime{sandbox: sandbox[0]}
	}
	return NewBwrapRuntimeWithBinary("bwrap")
}

func NewBwrapRuntimeWithBinary(binary string) *BwrapRuntime {
	return NewBwrapRuntimeWithBinaryAndWorkspaceRoots(binary, nil)
}

func NewBwrapRuntimeWithBinaryAndWorkspaceRoots(binary string, roots []string) *BwrapRuntime {
	return &BwrapRuntime{sandbox: NewBwrapSandboxWithWorkspaceRoots(binary, roots), binary: binary, roots: roots}
}

func (r *BwrapRuntime) ID() domain.RuntimeID     { return "bwrap" }
func (r *BwrapRuntime) Name() string             { return "Bubblewrap Sandbox" }
func (r *BwrapRuntime) Kind() domain.RuntimeKind { return domain.RuntimeKindSandbox }
func (r *BwrapRuntime) SupportedInstallModes() []domain.InstallMode {
	return []domain.InstallMode{domain.InstallModeExisting}
}
func (r *BwrapRuntime) ListVersions(context.Context, int) ([]domain.RuntimeVersion, error) {
	return nil, nil
}
func (r *BwrapRuntime) Install(context.Context, domain.InstallMode, string, string) (domain.RuntimeState, error) {
	return domain.RuntimeState{}, fmt.Errorf("bubblewrap must be installed by the host package manager and configured as an existing runtime")
}
func (r *BwrapRuntime) Resolve(ctx context.Context, state domain.RuntimeState) error {
	if err := requireLinux(runtime.GOOS); err != nil {
		return err
	}
	bin := state.BinPath
	if bin == "" {
		bin = r.binary
	}
	_, err := canonicalBinary(bin)
	return err
}
func (r *BwrapRuntime) Health(ctx context.Context, state domain.RuntimeState) domain.RuntimeHealth {
	now := time.Now().UTC()
	if err := requireLinux(runtime.GOOS); err != nil {
		return domain.RuntimeHealth{Status: "error", Errors: []string{err.Error()}, CheckedAt: now}
	}
	bin := state.BinPath
	if bin == "" {
		bin = r.binary
	}
	canonical, err := canonicalBinary(bin)
	if err != nil {
		return domain.RuntimeHealth{Status: "error", Errors: []string{err.Error()}, CheckedAt: now}
	}
	version, err := commandVersion(ctx, canonical, "--version")
	if err != nil {
		return domain.RuntimeHealth{Status: "error", Errors: []string{err.Error()}, CheckedAt: now}
	}
	return domain.RuntimeHealth{Status: "ok", Version: strings.TrimSpace(version), Checks: []domain.DiagnosticCheck{{Name: "version_command", Status: "ok", Message: strings.TrimSpace(version), CheckedAt: now}}, CheckedAt: now}
}
func (r *BwrapRuntime) ReadConfig(context.Context, domain.RuntimeState) ([]domain.RuntimeConfigFile, error) {
	return nil, nil
}
func (r *BwrapRuntime) WriteConfig(context.Context, domain.RuntimeState, string, []byte) error {
	return fmt.Errorf("bubblewrap sandbox configuration is supplied by explicit agent configuration")
}
func (r *BwrapRuntime) Execute(ctx context.Context, state domain.RuntimeState, input ports.RuntimeExecutionInput) (ports.RuntimeExecutionResult, error) {
	command := strings.TrimSpace(input.Config[bwrapCommandConfigKey])
	if command == "" {
		return ports.RuntimeExecutionResult{}, ports.ErrSandboxCommandRequired
	}
	workspace := strings.TrimSpace(input.Config[bwrapWorkspaceConfigKey])
	sandbox := r.sandbox
	if r.binary != "" {
		binary := state.BinPath
		if binary == "" {
			binary = r.binary
		}
		sandbox = NewBwrapSandboxWithWorkspaceRoots(binary, r.roots)
	}
	result, err := sandbox.Isolate(ctx, ports.SandboxCommand{Command: command, Args: input.ExtraArgs, Env: envPairs(input.Env), Workspace: workspace, OnStart: input.OnStart, OnOutput: input.OnOutput})
	if err != nil {
		return ports.RuntimeExecutionResult{}, err
	}
	return ports.RuntimeExecutionResult{Output: result.Output}, nil
}
func (r *BwrapRuntime) Cancel(ctx context.Context, state domain.RuntimeState, run domain.Run) error {
	if run.ProcessID <= 0 {
		return nil
	}
	return cancelRuntimeProcess(ctx, run.ProcessID)
}
