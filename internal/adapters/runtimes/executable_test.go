package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"picoclip/internal/core/domain"
)

func TestResolveConfiguredExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable mode semantics")
	}

	tmp := t.TempDir()
	executable := filepath.Join(tmp, "runtime-tool")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	nonExecutable := filepath.Join(tmp, "not-executable")
	if err := os.WriteFile(nonExecutable, []byte("plain"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmp)

	resolved, err := resolveConfiguredExecutable("runtime-tool")
	if err != nil {
		t.Fatalf("resolve PATH executable: %v", err)
	}
	want, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}

	for name, path := range map[string]string{
		"directory":      tmp,
		"non-executable": nonExecutable,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveConfiguredExecutable(path); err == nil {
				t.Fatalf("resolveConfiguredExecutable(%q) succeeded", path)
			}
		})
	}
}

func TestNativeAdaptersResolveConfiguredExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable mode semantics")
	}

	tmp := t.TempDir()
	executable := filepath.Join(tmp, "runtime-tool")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	adapters := []interface {
		Resolve(context.Context, domain.RuntimeState) error
	}{
		NewCrushAdapter("runtime-tool"),
		NewPicoClawAdapter("runtime-tool"),
		NewClaurstAdapter("runtime-tool"),
	}
	for _, adapter := range adapters {
		if err := adapter.Resolve(context.Background(), domain.RuntimeState{}); err != nil {
			t.Fatalf("Resolve PATH executable: %v", err)
		}
		if err := adapter.Resolve(context.Background(), domain.RuntimeState{BinPath: tmp}); err == nil {
			t.Fatal("Resolve accepted directory as executable")
		}
	}
}

func TestResolveConfiguredExecutableCanonicalizesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink and executable mode semantics")
	}

	tmp := t.TempDir()
	target := filepath.Join(tmp, "target")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	resolved, err := resolveConfiguredExecutable(link)
	if err != nil {
		t.Fatalf("resolve symlink: %v", err)
	}
	if resolved != target {
		t.Fatalf("resolved = %q, want %q", resolved, target)
	}
}
