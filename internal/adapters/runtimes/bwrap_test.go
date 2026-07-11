package runtimes

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

func TestBwrapSandboxFailsClosedWhenBinaryIsUnavailable(t *testing.T) {
	sandbox := NewBwrapSandbox(filepath.Join(t.TempDir(), "missing-bwrap"))
	_, err := sandbox.Isolate(context.Background(), ports.SandboxCommand{Command: "/bin/echo", Args: []string{"hello"}})
	if err == nil || !strings.Contains(err.Error(), "sandbox unavailable") {
		t.Fatalf("expected unavailable bwrap to fail closed, got %v", err)
	}
}

func TestBwrapSandboxRejectsRelativeAndUnapprovedCommands(t *testing.T) {
	sandbox := NewBwrapSandbox(fakeBwrapBinary(t))
	for _, command := range []string{"echo", filepath.Join(t.TempDir(), "command")} {
		_, err := sandbox.Isolate(context.Background(), ports.SandboxCommand{Command: command})
		if err == nil {
			t.Fatalf("expected command %q to be rejected", command)
		}
	}
}

func TestBwrapSandboxBuildsMinimalRootAndClearsHostEnvironment(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "bwrap-args")
	envFile := filepath.Join(t.TempDir(), "bwrap-env")
	sandbox := NewBwrapSandbox(fakeBwrapBinary(t, argsFile, envFile))
	old := os.Getenv("HOST_SECRET")
	t.Setenv("HOST_SECRET", "must-not-reach-bwrap")
	defer os.Setenv("HOST_SECRET", old)

	result, err := sandbox.Isolate(context.Background(), ports.SandboxCommand{Command: "/bin/echo", Args: []string{"hello"}, Env: []string{"SAFE=value"}})
	if err != nil {
		t.Fatalf("isolate command: %v", err)
	}
	if result.Output != "sandboxed" {
		t.Fatalf("expected sandbox output, got %q", result.Output)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read bwrap arguments: %v", err)
	}
	args := string(raw)
	for _, expected := range []string{"--unshare-all", "--die-with-parent", "--tmpfs\n/", "--remount-ro\n/", "--ro-bind\n/usr/bin\n/usr/bin", "--proc\n/proc", "--dev\n/dev", "--clearenv", "--setenv\nSAFE\nvalue", "--ro-bind\n/proc/self/fd/3\n/command"} {
		if !strings.Contains(args, expected) {
			t.Errorf("bwrap arguments missing %q:\n%s", expected, args)
		}
	}
	for _, forbidden := range []string{"--ro-bind\n/\n/", "--share-net", "--bind\n/run"} {
		if strings.Contains(args, forbidden) {
			t.Errorf("bwrap arguments must not contain %q:\n%s", forbidden, args)
		}
	}
	env, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read bwrap environment: %v", err)
	}
	if strings.Contains(string(env), "HOST_SECRET") {
		t.Fatalf("host environment leaked to bwrap: %s", env)
	}
}

func TestBwrapSandboxWorkspaceMustBeUnderAuthorizedRoot(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sandbox := NewBwrapSandboxWithWorkspaceRoots(fakeBwrapBinary(t), []string{root})
	if _, err := sandbox.Isolate(context.Background(), ports.SandboxCommand{Command: "/bin/echo", Workspace: workspace}); err != nil {
		t.Fatalf("authorized workspace rejected: %v", err)
	}
	outside := t.TempDir()
	if _, err := sandbox.Isolate(context.Background(), ports.SandboxCommand{Command: "/bin/echo", Workspace: outside}); err == nil || !strings.Contains(err.Error(), "authorized roots") {
		t.Fatalf("expected unauthorized workspace rejection, got %v", err)
	}
}

func TestBwrapSandboxStreamsOutputBeforeProcessExit(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "bwrap")
	body := "#!/bin/sh\nprintf 'first\\n'\nsleep 1\nprintf 'second\\n'\n"
	if err := os.WriteFile(binary, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	sandbox := NewBwrapSandbox(binary)
	firstOutput := make(chan string, 1)
	done := make(chan error, 1)

	go func() {
		_, err := sandbox.Isolate(context.Background(), ports.SandboxCommand{
			Command: "/bin/echo",
			OnOutput: func(stdout, stderr []byte) {
				if len(stdout) > 0 {
					select {
					case firstOutput <- string(stdout):
					default:
					}
				}
			},
		})
		done <- err
	}()

	select {
	case output := <-firstOutput:
		if !strings.Contains(output, "first") {
			t.Fatalf("expected first output chunk, got %q", output)
		}
	case err := <-done:
		t.Fatalf("sandbox exited before streaming output: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected output callback while sandbox was still running")
	}

	if err := <-done; err != nil {
		t.Fatalf("sandbox execution: %v", err)
	}
}

func TestBwrapSandboxIntegrationIsolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("bubblewrap sandbox is Linux-only")
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skip("bwrap unavailable: " + err.Error())
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(root, "host-sentinel")
	if err := os.WriteFile(sentinel, []byte("host only"), 0o600); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(root, "host.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	sandbox := NewBwrapSandboxWithWorkspaceRoots(bwrap, []string{root})
	script := "set -eu; test ! -e \"$1\"; test ! -S \"$2\"; touch authorized; ! touch /host-write; ! wget -T 2 -q http://1.1.1.1/; ! wget -T 2 -q http://does-not-resolve.invalid/"
	if _, err := sandbox.Isolate(context.Background(), ports.SandboxCommand{Command: "/bin/sh", Args: []string{"-c", script, "sh", sentinel, socketPath}, Workspace: workspace}); err != nil {
		t.Fatalf("real bwrap isolation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "authorized")); err != nil {
		t.Fatalf("authorized workspace write missing: %v", err)
	}
	if _, err := os.Stat("/host-write"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sandbox wrote outside workspace: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = sandbox.Isolate(ctx, ports.SandboxCommand{Command: "/bin/sh", Args: []string{"-c", "sleep 5; touch should-not-exist"}, Workspace: workspace})
	if err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected context deadline to stop sandbox, got %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, statErr := os.Stat(filepath.Join(workspace, "should-not-exist")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("sandbox subprocess survived cancellation: %v", statErr)
	}
}

func TestBwrapRuntimeExecuteRequiresExplicitAbsoluteCommand(t *testing.T) {
	runtime := NewBwrapRuntime(NewBwrapSandbox(fakeBwrapBinary(t)))
	_, err := runtime.Execute(context.Background(), domain.RuntimeState{Enabled: true}, ports.RuntimeExecutionInput{Task: domain.Task{Prompt: "do not execute this prompt"}})
	if !errors.Is(err, ports.ErrSandboxCommandRequired) {
		t.Fatalf("expected ErrSandboxCommandRequired, got %v", err)
	}
}

func TestRequireLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		if err := requireLinux(runtime.GOOS); err == nil {
			t.Fatal("expected non-linux platform to be rejected")
		}
	}
	if err := requireLinux("windows"); err == nil || !strings.Contains(err.Error(), "only supported on linux") {
		t.Fatalf("expected clear platform error, got %v", err)
	}
}

func fakeBwrapBinary(t *testing.T, paths ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bwrap")
	argsPath, envPath := "", ""
	if len(paths) > 0 {
		argsPath = paths[0]
	}
	if len(paths) > 1 {
		envPath = paths[1]
	}
	body := "#!/bin/sh\n"
	if argsPath != "" {
		body += "printf '%s\\n' \"$@\" > " + shellSingleQuote(argsPath) + "\n"
	}
	if envPath != "" {
		body += "env | sort > " + shellSingleQuote(envPath) + "\n"
	}
	body += "echo sandboxed\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}
