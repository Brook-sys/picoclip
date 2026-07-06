//go:build !windows

package runtimes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const runtimeCancelGracePeriod = 750 * time.Millisecond

func startRuntimeCommand(cmd *exec.Cmd) error {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	return cmd.Start()
}

func cancelRuntimeProcess(ctx context.Context, pid int) error {
	if pid <= 0 {
		return nil
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		process, findErr := os.FindProcess(pid)
		if findErr != nil {
			return nil
		}
		return process.Kill()
	}

	if pgid > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		if waitForProcessGroupExit(ctx, pgid, runtimeCancelGracePeriod) {
			return nil
		}
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}

	process, findErr := os.FindProcess(pid)
	if findErr != nil {
		return nil
	}
	return process.Kill()
}

func waitForProcessGroupExit(ctx context.Context, pgid int, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return !runtimeProcessGroupExists(pgid)
		case <-tick.C:
			if !runtimeProcessGroupExists(pgid) {
				return true
			}
		}
	}
}

func runtimeProcessGroupExists(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	err := syscall.Kill(-pgid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func runtimeProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	if raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) >= 3 && fields[2] == "Z" {
			return false
		}
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
