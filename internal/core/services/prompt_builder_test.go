package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"picoclip/internal/adapters/storage/memory"
	"picoclip/internal/core/domain"
)

func TestPromptBuilderBuildsRunnerInput(t *testing.T) {
	st := memory.NewStorage()
	clock := fixedClock{t: time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)}
	builder := NewPromptBuilder(st)
	agent := domain.Agent{
		ID:           "agent_prompt",
		Name:         "prompt agent",
		Type:         "noop",
		SystemPrompt: "Prefer concise updates.",
		Permissions:  []domain.AgentPermission{domain.PermissionTasksCreate},
		SkillIDs:     []string{"skill_manual"},
	}
	task := domain.Task{ID: "task_prompt", AgentID: agent.ID, Title: "Build prompt", Prompt: "Do the thing", Status: domain.TaskStatusInProgress, CreatedAt: clock.t, UpdatedAt: clock.t}
	run := domain.Run{ID: "run_prompt", TaskID: task.ID, AgentID: agent.ID, StartedAt: clock.t}
	if err := st.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := st.Messages().Create(context.Background(), domain.Message{ID: "msg_user", TaskID: task.ID, Role: domain.MessageRoleUser, Body: "Please include context.", CreatedAt: clock.t}); err != nil {
		t.Fatal(err)
	}
	if err := st.Skills().Create(context.Background(), domain.Skill{ID: "skill_manual", Name: "Manual Skill", Description: "desc", Instructions: "follow these steps", Enabled: true, Kind: domain.SkillKindCustom}); err != nil {
		t.Fatal(err)
	}

	prompt, err := builder.Build(context.Background(), PromptBuildInput{Agent: agent, Task: task, Run: run})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	for _, want := range []string{"Agent permissions:", string(domain.PermissionTasksCreate), "Agent custom system prompt", "Prefer concise updates.", "PicoClip Task Protocol", "Task ID: task_prompt", "Please include context.", "Skill package: Manual Skill", "follow these steps", "User task: Do the thing"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q:\n%s", want, prompt)
		}
	}
}
