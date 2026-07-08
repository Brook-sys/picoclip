package docs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownLinkCheckerRejectsMissingInternalAnchor(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("create docs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("[bad](docs/guide.md#missing-anchor)\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("# Existing Anchor\n"), 0o644); err != nil {
		t.Fatalf("write guide: %v", err)
	}

	cmd := exec.Command("python3", "../../scripts/check_markdown_links.py", root)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected checker to fail for missing anchor, output:\n%s", output)
	}
	if !strings.Contains(string(output), "missing anchor #missing-anchor") {
		t.Fatalf("expected missing anchor message, output:\n%s", output)
	}
}
