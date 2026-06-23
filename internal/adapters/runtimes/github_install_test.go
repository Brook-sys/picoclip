package runtimes

import (
	"strings"
	"testing"
)

func TestRuntimeAssetNamesIncludeVersionedAndUnversionedForms(t *testing.T) {
	names := runtimeAssetNames("crush", "v0.79.1")
	if len(names) != 4 {
		t.Fatalf("expected four asset candidates, got %#v", names)
	}
	if !strings.Contains(names[0], "crush_0.79.1_") {
		t.Fatalf("expected versioned asset first, got %#v", names)
	}
	if strings.Contains(names[1], "0.79.1") {
		t.Fatalf("expected unversioned asset second, got %#v", names)
	}
	if !strings.Contains(strings.Join(names, "\n"), "claurst") && strings.Contains(strings.Join(names, "\n"), "claurst-") {
		t.Fatalf("unexpected claurst candidate in crush names: %#v", names)
	}
}

func TestRuntimeAssetNamesIncludeHyphenatedLowercaseForms(t *testing.T) {
	names := runtimeAssetNames("claurst", "v0.1.5")
	joined := strings.Join(names, "\n")
	if !strings.Contains(joined, "claurst-linux-x86_64.tar.gz") && !strings.Contains(joined, "claurst-darwin-x86_64.tar.gz") && !strings.Contains(joined, "claurst-windows-x86_64.zip") {
		t.Fatalf("expected hyphenated lowercase asset candidate, got %#v", names)
	}
}

func TestSanitizeOutput(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"\x1b[1;38;2;62;93;185mhello", "hello"},
		{"picoclaw version 0.2.9 (git: 2992eccb)", "picoclaw version 0.2.9 (git: 2992eccb)"},
	}
	for _, c := range cases {
		if got := sanitizeTerminalOutput(c.input); got != c.want {
			t.Errorf("sanitizeTerminalOutput(%q) = %q; want %q", c.input, got, c.want)
		}
	}
}

func TestExtractRuntimeVersion(t *testing.T) {
	cases := []struct {
		id    string
		input string
		want  string
	}{
		{"picoclaw", "picoclaw 0.2.9 (git: 2992eccb)", "picoclaw 0.2.9"},
		{"picoclaw", "\x1b[1;38;2;62;93;185m██████╗\x1b[0m\npicoclaw 0.2.9 (git: 123)", "picoclaw 0.2.9"},
		{"crush", "crush version v0.79.1", "crush v0.79.1"},
		{"claurst", "claurst version v0.1.5", "claurst v0.1.5"},
		{"claurst", "v0.1.5", "claurst v0.1.5"},
		{"", "v1.2.3", "v1.2.3"},
	}
	for _, c := range cases {
		if got := extractRuntimeVersion(c.id, c.input); got != c.want {
			t.Errorf("extractRuntimeVersion(%q, %q) = %q; want %q", c.id, c.input, got, c.want)
		}
	}
}
