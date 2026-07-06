//go:build windows

package runtimes

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func startRuntimeCommand(cmd *exec.Cmd) error {
	return cmd.Start()
}

func cancelRuntimeProcess(ctx context.Context, pid int) error {
	if pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	return process.Kill()
}

func waitForProcessExit(ctx context.Context, pid int, timeout time.Duration) bool {
	return false
}

func runtimeProcessExists(pid int) bool {
	return false
}
