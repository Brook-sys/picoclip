package docs_test

import (
	"strings"
	"testing"

	"os"
)

func TestDocumentationPolicyHasChangeTypeValidationMatrix(t *testing.T) {
	content, err := os.ReadFile("../../docs/DOCUMENTATION_POLICY.md")
	if err != nil {
		t.Fatalf("read documentation policy: %v", err)
	}
	text := string(content)

	required := []string{
		"## Matriz de validação mínima por tipo de mudança",
		"Docs-only",
		"UI / Templ / CSS",
		"API / Agent API",
		"Storage / migrations",
		"Robustez / retry / recovery",
		"Workflow dev / comandos",
		"Roadmap / current-state",
		"make templ-generate",
		"make check",
	}
	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Fatalf("DOCUMENTATION_POLICY.md missing %q", want)
		}
	}
}
