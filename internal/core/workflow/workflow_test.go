package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseYAMLAcceptsVersionedAcyclicWorkflow(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("testdata", "valid.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	workflow, err := ParseYAML(document)
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}
	if workflow.Version != VersionV1 {
		t.Fatalf("version = %q, want %q", workflow.Version, VersionV1)
	}
	if len(workflow.Nodes) != 3 || len(workflow.Edges) != 2 {
		t.Fatalf("workflow = %#v, want 3 nodes and 2 edges", workflow)
	}
	if got := workflow.Nodes[0].Inputs["environment"]; got != "staging" {
		t.Fatalf("first node environment = %#v, want staging", got)
	}
}

func TestParseYAMLRejectsInvalidWorkflows(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    string
	}{
		{name: "unsupported version", fixture: "unsupported-version.yaml", want: "unsupported workflow version"},
		{name: "duplicate node ID", fixture: "duplicate-node-id.yaml", want: "duplicate node ID"},
		{name: "unknown edge node", fixture: "unknown-edge-node.yaml", want: "references unknown node"},
		{name: "cycle", fixture: "cycle.yaml", want: "contains cycle"},
		{name: "unknown field", fixture: "unknown-field.yaml", want: "field unexpected not found"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			if err != nil {
				t.Fatal(err)
			}

			_, err = ParseYAML(document)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseYAML error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsNodeWithoutAgent(t *testing.T) {
	err := Validate(Workflow{Version: VersionV1, Nodes: []Node{{ID: "prepare"}}})
	if err == nil || !strings.Contains(err.Error(), "agent is required") {
		t.Fatalf("Validate error = %v, want agent validation error", err)
	}
}

func TestParseYAMLRejectsMultipleDocuments(t *testing.T) {
	_, err := ParseYAML([]byte("version: v1\nnodes: []\n---\nversion: v1\nnodes: []\n"))
	if err == nil || !strings.Contains(err.Error(), "exactly one YAML document") {
		t.Fatalf("ParseYAML error = %v, want single document validation error", err)
	}
}

func TestParseYAMLNormalizesWhitespace(t *testing.T) {
	yamlStr := `
version: v1
nodes:
  - id: " my-node "
    agent: "dummy"
  - id: " next-node "
    agent: "dummy"
edges:
  - from: " my-node "
    to: " next-node "
`
	workflow, err := ParseYAML([]byte(yamlStr))
	if err != nil {
		t.Fatalf("ParseYAML returned error: %v", err)
	}

	if workflow.Nodes[0].ID != "my-node" {
		t.Errorf("expected node ID to be normalized to 'my-node', got %q", workflow.Nodes[0].ID)
	}
	if workflow.Nodes[1].ID != "next-node" {
		t.Errorf("expected node ID to be normalized to 'next-node', got %q", workflow.Nodes[1].ID)
	}
	if workflow.Edges[0].From != "my-node" || workflow.Edges[0].To != "next-node" {
		t.Errorf("expected edge to be normalized to from: 'my-node', to: 'next-node', got from: %q, to: %q", workflow.Edges[0].From, workflow.Edges[0].To)
	}
}
