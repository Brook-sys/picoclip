//go:build !windows

package runtimes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCancelRuntimeProcessTerminatesProcessGroupChildren(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	childPIDFile := filepath.Join(tmp, "child.pid")

	cmd := exec.CommandContext(ctx, "sh", "-c", "sh -c 'trap \"\" TERM; echo $$ > \"$1\"; while true; do sleep 1; done' child \"$1\" & wait", "parent", childPIDFile)
	if err := startRuntimeCommand(cmd); err != nil {
		t.Fatalf("start command: %v", err)
	}

	childPID := waitForPIDFile(t, childPIDFile)
	if childPID <= 0 {
		t.Fatalf("expected child pid, got %d", childPID)
	}
	if !processAlive(childPID) {
		t.Fatalf("expected child process %d to be alive before cancellation", childPID)
	}

	if err := cancelRuntimeProcess(ctx, cmd.Process.Pid); err != nil {
		t.Fatalf("cancel runtime process: %v", err)
	}
	_ = cmd.Wait()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(childPID) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("child process %d survived runtime cancellation", childPID)
}

func waitForPIDFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if convErr == nil {
				return pid
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("pid file %s was not written", path)
	return 0
}

func processAlive(pid int) bool {
	return runtimeProcessExists(pid)
}
