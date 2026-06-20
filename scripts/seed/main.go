package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type SeedScenario struct {
	Projects []ProjectSeed `json:"projects"`
	Skills   []SkillSeed   `json:"skills"`
	Agents   []AgentSeed   `json:"agents"`
	Tasks    []TaskSeed    `json:"tasks"`
}

type ProjectSeed struct {
	RefID       string `json:"ref_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SkillSeed struct {
	RefID        string `json:"ref_id"`
	ProjectRef   string `json:"project_ref"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

type AgentSeed struct {
	RefID            string   `json:"ref_id"`
	ProjectRef       string   `json:"project_ref"`
	Name             string   `json:"name"`
	Title            string   `json:"title"`
	ReportsToRef     string   `json:"reports_to_ref"`
	Tags             []string `json:"tags"`
	Type             string   `json:"type"`
	Description      string   `json:"description"`
	SystemPrompt     string   `json:"system_prompt"`
	SkillRefs        []string `json:"skill_refs"`
	Capability       string   `json:"capability"`
	PermissionPreset string   `json:"permission_preset"`
}

type TaskSeed struct {
	RefID         string `json:"ref_id"`
	ProjectRef    string `json:"project_ref"`
	AgentRef      string `json:"agent_ref"`
	Prompt        string `json:"prompt"`
	Status        string `json:"status"`
	StatusComment string `json:"status_comment"`
	Messages      []struct {
		Role string `json:"role"`
		Body string `json:"body"`
	} `json:"messages"`
	CancelReason string `json:"cancel_reason"`
	Delegations  []struct {
		ToAgentRef string `json:"to_agent_ref"`
		Prompt     string `json:"prompt"`
		RefID      string `json:"ref_id"`
	} `json:"delegations"`
}

var refMap = map[string]string{}

func main() {
	baseURL := flag.String("base-url", "http://127.0.0.1:8080", "Base URL of PicoClip server")
	scenarioFile := flag.String("scenario", "scripts/seed/scenarios/full.json", "Scenario JSON file")
	flag.Parse()

	data, err := os.ReadFile(*scenarioFile)
	if err != nil {
		fmt.Printf("Error reading scenario: %v\n", err)
		os.Exit(1)
	}

	var scenario SeedScenario
	if err := json.Unmarshal(data, &scenario); err != nil {
		fmt.Printf("Error parsing scenario: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Seeding PicoClip at %s from %s...\n\n", *baseURL, *scenarioFile)

	client := &http.Client{Timeout: 10 * time.Second}

	for _, p := range scenario.Projects {
		id := postJSON(client, *baseURL+"/api/v1/projects", map[string]any{
			"name":        p.Name,
			"description": p.Description,
		})
		refMap[p.RefID] = id
		fmt.Printf("✓ Project created: %s (%s)\n", p.Name, id)
	}

	for _, s := range scenario.Skills {
		payload := map[string]any{
			"name":         s.Name,
			"description":  s.Description,
			"instructions": s.Instructions,
		}
		if s.ProjectRef != "" {
			payload["project_id"] = refMap[s.ProjectRef]
		}
		id := postJSON(client, *baseURL+"/api/v1/skills", payload)
		refMap[s.RefID] = id
		fmt.Printf("✓ Skill created: %s (%s)\n", s.Name, id)
	}

	for _, a := range scenario.Agents {
		payload := map[string]any{
			"name":              a.Name,
			"title":             a.Title,
			"type":              a.Type,
			"description":       a.Description,
			"system_prompt":     a.SystemPrompt,
			"tags":              a.Tags,
			"capability":        a.Capability,
			"permission_preset": a.PermissionPreset,
		}
		if a.ProjectRef != "" {
			payload["project_id"] = refMap[a.ProjectRef]
		}
		if a.ReportsToRef != "" {
			payload["reports_to_id"] = refMap[a.ReportsToRef]
		}

		skillIDs := []string{}
		for _, ref := range a.SkillRefs {
			skillIDs = append(skillIDs, refMap[ref])
		}
		payload["skill_ids"] = skillIDs

		id := postJSON(client, *baseURL+"/api/v1/agents", payload)
		refMap[a.RefID] = id
		fmt.Printf("✓ Agent created: %s (%s)\n", a.Name, id)
	}

	for _, t := range scenario.Tasks {
		payload := map[string]any{
			"prompt": t.Prompt,
		}
		if t.ProjectRef != "" {
			payload["project_id"] = refMap[t.ProjectRef]
		}
		if t.AgentRef != "" {
			payload["agent_id"] = refMap[t.AgentRef]
		}
		id := postJSON(client, *baseURL+"/api/v1/tasks", payload)
		refMap[t.RefID] = id
		fmt.Printf("✓ Task created: %s (%s)\n", truncate(t.Prompt, 30), id)

		for _, message := range t.Messages {
			postJSON(client, *baseURL+"/api/v1/tasks/"+id+"/messages", map[string]any{
				"role": message.Role,
				"body": message.Body,
			})
			fmt.Printf("  ↳ Message added: %s\n", truncate(message.Body, 38))
		}

		for _, delegation := range t.Delegations {
			childID := postJSON(client, *baseURL+"/api/v1/tasks/"+id+"/delegate", map[string]any{
				"from_agent_id": refMap[t.AgentRef],
				"to_agent_id":   refMap[delegation.ToAgentRef],
				"prompt":        delegation.Prompt,
			})
			if delegation.RefID != "" {
				refMap[delegation.RefID] = childID
			}
			fmt.Printf("  ↳ Delegated child task: %s (%s)\n", truncate(delegation.Prompt, 30), childID)
		}

		if t.Status != "" {
			patchJSON(client, *baseURL+"/agent-api/tasks/"+id, map[string]any{
				"agent_id": refMap[t.AgentRef],
				"status":   t.Status,
				"comment":  t.StatusComment,
			})
			fmt.Printf("  ↳ Status set to: %s\n", t.Status)
		}

		if t.CancelReason != "" {
			postJSON(client, *baseURL+"/api/v1/tasks/"+id+"/cancel", map[string]any{
				"reason": t.CancelReason,
			})
			fmt.Printf("  ↳ Task cancelled: %s\n", t.CancelReason)
		}
	}

	fmt.Println("\nSeed completed successfully!")
}

func postJSON(client *http.Client, url string, payload map[string]any) string {
	return sendJSON(client, http.MethodPost, url, payload)
}

func patchJSON(client *http.Client, url string, payload map[string]any) string {
	return sendJSON(client, http.MethodPatch, url, payload)
}

func sendJSON(client *http.Client, method string, url string, payload map[string]any) string {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequest(method, url, bytes.NewReader(b))
	if err != nil {
		fmt.Printf("Request error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		fmt.Printf("API error (%d): %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var res struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		fmt.Printf("Parse error: %v\n", err)
		os.Exit(1)
	}
	return res.Data.ID
}

func truncate(s string, l int) string {
	if len(s) > l {
		return s[:l] + "..."
	}
	return s
}
