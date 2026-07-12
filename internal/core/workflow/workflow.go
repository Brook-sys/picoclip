// Package workflow defines and validates declarative workflow documents.
// It intentionally does not schedule or execute workflow nodes.
package workflow

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const VersionV1 = "v1"

// Workflow is the versioned, declarative representation of an acyclic workflow.
type Workflow struct {
	Version string `yaml:"version" json:"version"`
	Nodes   []Node `yaml:"nodes" json:"nodes"`
	Edges   []Edge `yaml:"edges,omitempty" json:"edges,omitempty"`
}

// Node identifies the agent configuration and inputs for a workflow step.
type Node struct {
	ID      string         `yaml:"id" json:"id"`
	Agent   string         `yaml:"agent" json:"agent"`
	Profile string         `yaml:"profile,omitempty" json:"profile,omitempty"`
	Inputs  map[string]any `yaml:"inputs,omitempty" json:"inputs,omitempty"`
}

// Edge declares that From must precede To.
type Edge struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to" json:"to"`
}

// ParseYAML decodes one workflow YAML document and validates its graph contract.
func ParseYAML(document []byte) (Workflow, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(document))
	decoder.KnownFields(true)

	var workflow Workflow
	if err := decoder.Decode(&workflow); err != nil {
		return Workflow{}, fmt.Errorf("decode workflow YAML: %w", err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Workflow{}, fmt.Errorf("decode workflow YAML: exactly one YAML document is required")
		}
		return Workflow{}, fmt.Errorf("decode workflow YAML: %w", err)
	}

	// Normalize whitespace in node and edge IDs immediately on parse
	for i := range workflow.Nodes {
		workflow.Nodes[i].ID = strings.TrimSpace(workflow.Nodes[i].ID)
	}
	for i := range workflow.Edges {
		workflow.Edges[i].From = strings.TrimSpace(workflow.Edges[i].From)
		workflow.Edges[i].To = strings.TrimSpace(workflow.Edges[i].To)
	}

	if err := Validate(workflow); err != nil {
		return Workflow{}, err
	}
	return workflow, nil
}

// Validate enforces the v1 declarative workflow contract, including acyclicity.
func Validate(workflow Workflow) error {
	if workflow.Version != VersionV1 {
		return fmt.Errorf("unsupported workflow version %q; supported version is %q", workflow.Version, VersionV1)
	}

	nodeIDs := make(map[string]struct{}, len(workflow.Nodes))
	for index, node := range workflow.Nodes {
		node.ID = strings.TrimSpace(node.ID)
		if node.ID == "" {
			return fmt.Errorf("node at index %d: ID is required", index)
		}
		if _, exists := nodeIDs[node.ID]; exists {
			return fmt.Errorf("duplicate node ID %q", node.ID)
		}
		if strings.TrimSpace(node.Agent) == "" {
			return fmt.Errorf("node %q: agent is required", node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
	}

	adjacency := make(map[string][]string, len(nodeIDs))
	for index, edge := range workflow.Edges {
		from := strings.TrimSpace(edge.From)
		to := strings.TrimSpace(edge.To)
		if from == "" || to == "" {
			return fmt.Errorf("edge at index %d: from and to are required", index)
		}
		if _, exists := nodeIDs[from]; !exists {
			return fmt.Errorf("edge at index %d: from %q references unknown node", index, from)
		}
		if _, exists := nodeIDs[to]; !exists {
			return fmt.Errorf("edge at index %d: to %q references unknown node", index, to)
		}
		adjacency[from] = append(adjacency[from], to)
	}

	visiting := make(map[string]bool, len(nodeIDs))
	visited := make(map[string]bool, len(nodeIDs))
	var visit func(string) error
	visit = func(nodeID string) error {
		if visiting[nodeID] {
			return fmt.Errorf("workflow contains cycle involving node %q", nodeID)
		}
		if visited[nodeID] {
			return nil
		}

		visiting[nodeID] = true
		for _, next := range adjacency[nodeID] {
			if err := visit(next); err != nil {
				return err
			}
		}
		visiting[nodeID] = false
		visited[nodeID] = true
		return nil
	}

	for nodeID := range nodeIDs {
		if err := visit(nodeID); err != nil {
			return err
		}
	}
	return nil
}
