package runtimes

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"picoclip/internal/core/domain"
)

func fakeRuntimeBinary(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-runtime")
	if runtime.GOOS == "windows" {
		path += ".bat"
		body = "@echo off\r\n" + body
	} else {
		body = "#!/bin/sh\n" + body
	}
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCrushExistingHealthWithFakeBinary(t *testing.T) {
	bin := fakeRuntimeBinary(t, "echo crush test-version\n")
	adapter := NewCrushAdapter(bin)
	health := adapter.Health(context.Background(), domain.RuntimeState{BinPath: bin})
	if health.Status != "ok" {
		t.Fatalf("expected ok health, got %#v", health)
	}
	if health.Version == "" {
		t.Fatalf("expected version output")
	}
}

func TestPicoClawExistingHealthWithFakeBinary(t *testing.T) {
	bin := fakeRuntimeBinary(t, "echo picoclaw test-version\n")
	adapter := NewPicoClawAdapter(bin)
	health := adapter.Health(context.Background(), domain.RuntimeState{BinPath: bin})
	if health.Status != "ok" {
		t.Fatalf("expected ok health, got %#v", health)
	}
	if health.Version == "" {
		t.Fatalf("expected version output")
	}
}

func TestClaurstExistingHealthWithFakeBinary(t *testing.T) {
	bin := fakeRuntimeBinary(t, "echo claurst v0.1.5\n")
	adapter := NewClaurstAdapter(bin)
	health := adapter.Health(context.Background(), domain.RuntimeState{BinPath: bin})
	if health.Status != "ok" {
		t.Fatalf("expected ok health, got %#v", health)
	}
	if health.Version == "" {
		t.Fatalf("expected version output")
	}
}
