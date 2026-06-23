package runtimes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"picoclip/internal/core/domain"
)

func envPairs(m map[string]string) []string {
	var out []string
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func userBinDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.Getenv("USERPROFILE")
		}
		return filepath.Join(base, "Programs", "PicoClip", "bin")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func commandVersion(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(out))
	if err != nil && outputStr != "" {
		if strings.Contains(outputStr, "not found") && strings.Contains(outputStr, "Error relocating") {
			return outputStr, fmt.Errorf("%w (Incompatible binary: usually glibc/musl mismatch) - output: %s", err, outputStr)
		}
		return outputStr, fmt.Errorf("%w - output: %s", err, outputStr)
	}
	return outputStr, err
}

func writeFileIfMissing(path string, content []byte, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, mode)
}

func copyExistingBinary(src, dst string) error {
	if strings.TrimSpace(src) == "" {
		return errors.New("binary path required")
	}
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0755)
}

func nowState(id domain.RuntimeID, mode domain.InstallMode, binPath, configPath, homePath, dataPath, logsPath string) domain.RuntimeState {
	now := time.Now().UTC()
	return domain.RuntimeState{
		ID:           "runtime_" + string(id),
		RuntimeID:    id,
		Mode:         mode,
		Enabled:      true,
		BinPath:      binPath,
		ConfigPath:   configPath,
		HomePath:     homePath,
		DataPath:     dataPath,
		LogsPath:     logsPath,
		Source:       "manual-or-existing",
		InstalledAt:  now,
		UpdatedAt:    now,
		SettingsJSON: "{}",
		MetadataJSON: "{}",
	}
}
