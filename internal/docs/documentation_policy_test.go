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

func TestAPIReferenceDocumentsCompactAgentAPIContracts(t *testing.T) {
	content, err := os.ReadFile("../../docs/API_REFERENCE.md")
	if err != nil {
		t.Fatalf("read API reference: %v", err)
	}
	text := string(content)

	required := []string{
		"### Contratos JSON compactos da Agent API",
		"#### `GET /agent-api/agents/me/inbox-lite`",
		"`agent_id` é obrigatório",
		"`inbox[]`",
		"#### `GET /agent-api/tasks/{id}/heartbeat-context`",
		"`meta.mode`",
		"`execution_state.pending_wakeups`",
		"até 3 wakeups pendentes",
		"até 5 eventos recentes",
		"`include` aceita apenas `prompt`, `execution_state`, `skills` e `apis`",
		"#### Operações compactas de task/issue",
		"`POST /agent-api/tasks/{id}/checkout`",
		"`expected_statuses`",
		"`POST /agent-api/tasks/{id}/release`",
		"`PATCH /agent-api/tasks/{id}`",
		"`POST /agent-api/tasks/{id}/wake`",
		"Os aliases `/agent-api/issues...`",
		"`400 Bad Request`",
		"`403 Forbidden`",
		"`404 Not Found`",
		"`409 Conflict`",
	}
	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Fatalf("API_REFERENCE.md missing %q", want)
		}
	}
}
