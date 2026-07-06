package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type PromptBuilder struct {
	storage ports.Storage
}

type PromptBuildInput struct {
	Agent    domain.Agent
	Task     domain.Task
	Run      domain.Run
	Messages []domain.Message
	Skills   []domain.Skill
}

func NewPromptBuilder(storage ports.Storage) PromptBuilder {
	return PromptBuilder{storage: storage}
}

func (b PromptBuilder) Build(ctx context.Context, input PromptBuildInput) (string, error) {
	messages := input.Messages
	if messages == nil {
		loaded, err := b.storage.Messages().ListByTask(ctx, input.Task.ID)
		if err != nil {
			return "", err
		}
		messages = loaded
	}
	skills := input.Skills
	if skills == nil {
		skills = b.skillsForAgent(ctx, input.Agent)
	}

	conversation := make([]string, 0, len(messages)+len(skills)+5)
	conversation = append(conversation, b.permissionContext(input.Agent))
	if input.Agent.InstructionFile != "" {
		content, fileErr := os.ReadFile(input.Agent.InstructionFile)
		if fileErr == nil {
			conversation = append(conversation, fmt.Sprintf("Agent instruction file (%s):\n%s", input.Agent.InstructionFile, string(content)))
		} else {
			conversation = append(conversation, fmt.Sprintf("Failed to load agent instruction file %s: %v", input.Agent.InstructionFile, fileErr))
		}
	}
	if input.Agent.SystemPrompt != "" {
		conversation = append(conversation, "Agent custom system prompt:\n"+input.Agent.SystemPrompt)
	}

	conversation = append(conversation, b.taskProtocolContext(ctx, input.Task, input.Run, messages))
	for _, skill := range skills {
		conversation = append(conversation, b.skillContext(skill))
	}
	conversation = append(conversation, "User task: "+input.Task.Prompt)
	return strings.Join(conversation, "\n\n"), nil
}

func (b PromptBuilder) skillsForAgent(ctx context.Context, agent domain.Agent) []domain.Skill {
	all, err := b.storage.Skills().List(ctx, agent.ProjectID)
	if err != nil {
		return nil
	}
	manual := make(map[string]struct{}, len(agent.SkillIDs))
	for _, id := range agent.SkillIDs {
		manual[id] = struct{}{}
	}
	permissions := make(map[domain.AgentPermission]struct{}, len(agent.Permissions))
	for _, permission := range agent.Permissions {
		permissions[permission] = struct{}{}
	}
	enabled := make([]domain.Skill, 0, len(all))
	seen := map[string]struct{}{}
	for _, skill := range all {
		if !skill.Enabled {
			continue
		}
		_, isManual := manual[skill.ID]
		_, hasPermission := permissions[skill.Permission]
		if skill.Kind == domain.SkillKindBuiltin && skill.Permission != "" && hasPermission || isManual || skillAllowedForAgent(skill, agent) {
			if _, ok := seen[skill.ID]; ok {
				continue
			}
			seen[skill.ID] = struct{}{}
			enabled = append(enabled, skill)
		}
	}
	return enabled
}

func skillAllowedForAgent(skill domain.Skill, agent domain.Agent) bool {
	if len(skill.AgentIDs) > 0 {
		for _, id := range skill.AgentIDs {
			if id == agent.ID {
				return true
			}
		}
	}
	if len(skill.AllowedAgentTypes) > 0 {
		for _, agentType := range skill.AllowedAgentTypes {
			if agentType == agent.Type {
				return true
			}
		}
	}
	if len(skill.AllowedPermissions) > 0 {
		for _, required := range skill.AllowedPermissions {
			for _, permission := range agent.Permissions {
				if required == permission {
					return true
				}
			}
		}
	}
	return false
}

func (b PromptBuilder) permissionContext(agent domain.Agent) string {
	return fmt.Sprintf("Agent permissions:\n%s", joinPermissions(agent.Permissions))
}

func (b PromptBuilder) skillContext(skill domain.Skill) string {
	parts := []string{fmt.Sprintf("Skill package: %s\n%s\nInstructions:\n%s", skill.Name, skill.Description, skill.Instructions)}
	for _, file := range skill.Files {
		parts = append(parts, fmt.Sprintf("Skill file %s:\n%s", file.Path, file.Content))
	}
	return strings.Join(parts, "\n\n")
}

func (b PromptBuilder) taskProtocolPrompt(ctx context.Context) string {
	raw, err := b.storage.Settings().Get(ctx, "general")
	if err == nil && raw != "" {
		var general struct {
			DefaultTaskProtocol string `json:"DefaultTaskProtocol"`
		}
		if jsonErr := json.Unmarshal([]byte(raw), &general); jsonErr == nil && strings.TrimSpace(general.DefaultTaskProtocol) != "" {
			return general.DefaultTaskProtocol
		}
	}
	return DefaultTaskProtocolPrompt()
}

func (b PromptBuilder) formatChildTaskForPrompt(ctx context.Context, child domain.Task) string {
	parts := []string{formatTaskSummary(child)}
	if child.Prompt != "" {
		parts = append(parts, "prompt: "+promptSnippet(child.Prompt, 180))
	}
	if runs, err := b.storage.Runs().ListByTask(ctx, child.ID); err == nil && len(runs) > 0 {
		latest := runs[len(runs)-1]
		runSummary := fmt.Sprintf("latest run: %s", latest.Status)
		if latest.Output != "" {
			runSummary += " output: " + promptSnippet(latest.Output, 220)
		} else if latest.Error != "" {
			runSummary += " error: " + promptSnippet(latest.Error, 220)
		}
		parts = append(parts, runSummary)
	}
	if messages, err := b.storage.Messages().ListByTask(ctx, child.ID); err == nil {
		if latest := latestMessageSnippet(messages); latest != "" {
			parts = append(parts, "latest message: "+latest)
		}
	}
	return strings.Join(parts, " | ")
}

func (b PromptBuilder) taskProtocolContext(ctx context.Context, task domain.Task, run domain.Run, messages []domain.Message) string {
	var sb strings.Builder
	sb.WriteString(b.taskProtocolPrompt(ctx))
	sb.WriteString("\n\nCompact Task Context:\n")
	sb.WriteString(fmt.Sprintf("- Task ID: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("- Title: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("- Status: %s\n", string(task.Status)))
	sb.WriteString(fmt.Sprintf("- Run ID: %s\n", run.ID))
	if task.ParentID != "" {
		sb.WriteString(fmt.Sprintf("- Parent Task ID: %s\n", task.ParentID))
	}
	if task.Mode == domain.TaskModeContinuous {
		sb.WriteString("\nContinuous Task Instructions:\n")
		sb.WriteString(fmt.Sprintf("- Current cycle: %d\n", task.LoopRunCount+1))
		sb.WriteString(fmt.Sprintf("- Delay after this cycle: %d seconds\n", task.LoopDelaySeconds))
		sb.WriteString("- This heartbeat should make incremental progress, inspect recent context, and report what changed.\n")
		sb.WriteString("- Do not block waiting for a human answer. If you need information, ask clearly in a comment and continue with safe assumptions or next best work.\n")
		sb.WriteString("- User answers/comments are consumed on the next scheduled cycle; do not request immediate reruns unless explicitly necessary.\n")
		sb.WriteString("- If you delegate subtasks, supervise them in later cycles by checking child task status before creating duplicates.\n")
		sb.WriteString("- Do not mark the parent task done unless the continuous loop objective should permanently stop.\n")
	}

	children, _ := b.storage.Tasks().List(ctx, ports.TaskFilter{ParentID: task.ID})
	if len(children) > 0 {
		sb.WriteString("\nChild Tasks To Supervise:\n")
		sb.WriteString("Before delegating, compare the requested work with these child tasks. Update, wait for, or summarize existing children instead of creating duplicates.\n")
		limit := len(children) - 8
		if limit < 0 {
			limit = 0
		}
		for _, child := range children[limit:] {
			sb.WriteString("- " + b.formatChildTaskForPrompt(ctx, child) + "\n")
		}
	}

	questions := userQuestions(messages)
	if task.Mode == domain.TaskModeContinuous && len(questions) > 0 {
		sb.WriteString("\nOpen Questions Raised For User (non-blocking):\n")
		start := len(questions) - 5
		if start < 0 {
			start = 0
		}
		for _, question := range questions[start:] {
			sb.WriteString(fmt.Sprintf("[%s] %s\n", question.CreatedAt.Format("15:04"), question.Body))
		}
	}

	latestUserComment := latestCommentByRole(messages, domain.MessageRoleUser)
	if latestUserComment != "" {
		sb.WriteString("\nLatest User Comment:\n")
		sb.WriteString(latestUserComment + "\n")
	}

	sb.WriteString("\nRecent Comments:\n")
	if len(messages) == 0 {
		sb.WriteString("(no comments)\n")
	} else {
		start := len(messages) - 8
		if start < 0 {
			start = 0
		}
		for _, m := range messages[start:] {
			sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.CreatedAt.Format("15:04"), m.Role, m.Body))
		}
	}

	return sb.String()
}
