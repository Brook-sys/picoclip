package runtimes

import (
	"strings"
	"testing"
)

func TestRuntimeAssetNamesIncludeVersionedAndUnversionedForms(t *testing.T) {
	names := runtimeAssetNames("crush", "v0.79.1")
	if len(names) != 2 {
		t.Fatalf("expected two asset candidates, got %#v", names)
	}
	if !strings.Contains(names[0], "crush_0.79.1_") {
		t.Fatalf("expected versioned asset first, got %#v", names)
	}
	if strings.Contains(names[1], "0.79.1") {
		t.Fatalf("expected unversioned asset second, got %#v", names)
	}
}
