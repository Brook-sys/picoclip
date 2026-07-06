package web

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

type QuestionForUser struct {
	TaskID    string
	TaskTitle string
	AgentID   string
	Body      string
	CreatedAt time.Time
}

type DashboardView struct {
	Stats struct {
		TotalProjects int
		ActiveAgents  int
		OpenTasks     int
		TotalSkills   int
		RunningRuns   int
		FailedRuns    int
		BlockedTasks  int
		DoneTasks     int
	}
	SystemHealth struct {
		StorageDriver string
		LastEventTime string
		AgentsIdle    int
		AgentsRunning int
	}
	NeedsAttention   []taskResponse
	CurrentlyRunning []domain.Run
	RecentActivity   []domain.Event
	QuestionsForUser []QuestionForUser
	RuntimeWarnings  []string
}

func loadDashboardView(ctx context.Context, s *Server, r *http.Request) (DashboardView, error) {
	var view DashboardView

	// Base lists
	agents, _ := s.agents.List(ctx)
	projects, _ := s.projects.List(ctx)
	skills, _ := s.skills.List(ctx, "")
	allTasks, _ := s.tasks.List(ctx, ports.TaskFilter{})

	view.Stats.TotalProjects = len(projects)
	view.Stats.TotalSkills = len(skills)

	activeAgents := 0
	for _, a := range agents {
		if a.Enabled {
			activeAgents++
		}
	}
	view.Stats.ActiveAgents = activeAgents

	view.SystemHealth.StorageDriver = "SQLite" // We migrated to sqlite explicitly now
	states, _ := s.runtimes.States(ctx)
	if len(states) == 0 {
		view.RuntimeWarnings = append(view.RuntimeWarnings, "No runtime configured. Tasks cannot be executed until Crush or PicoClaw is installed.")
	} else {
		for _, state := range states {
			tested, _, functional, checks, _ := runtimeHealthSummary(state)
			if tested && !functional {
				message := string(state.RuntimeID) + " is installed but not functional."
				for _, check := range checks {
					if check.Status == "error" {
						message += " " + check.Name + ": " + check.Message
						break
					}
				}
				view.RuntimeWarnings = append(view.RuntimeWarnings, message)
			}
		}
	}

	for _, task := range allTasks {
		view.QuestionsForUser = append(view.QuestionsForUser, questionsForTask(ctx, s, task)...)
		switch task.Status {
		case domain.TaskStatusBlocked:
			view.Stats.BlockedTasks++
			view.NeedsAttention = append(view.NeedsAttention, s.taskResponse(r, task))
		case domain.TaskStatusDone:
			view.Stats.DoneTasks++
		case domain.TaskStatusTodo, domain.TaskStatusBacklog, domain.TaskStatusInProgress, domain.TaskStatusWaitingNextCycle, domain.TaskStatusInReview:
			view.Stats.OpenTasks++
		}
	}

	allRuns := make([]domain.Run, 0)
	for _, task := range allTasks {
		taskRuns, err := s.storage.Runs().ListByTask(ctx, task.ID)
		if err == nil {
			allRuns = append(allRuns, taskRuns...)
		}
	}

	for _, run := range allRuns {
		if run.Status == domain.RunStatusRunning {
			view.Stats.RunningRuns++
			view.CurrentlyRunning = append(view.CurrentlyRunning, run)
		} else if run.Status == domain.RunStatusFailed {
			view.Stats.FailedRuns++
		}
	}

	// Calculate agent states roughly (if they own a running run, they are running)
	runningAgents := make(map[string]bool)
	for _, run := range view.CurrentlyRunning {
		runningAgents[run.AgentID] = true
	}
	view.SystemHealth.AgentsRunning = len(runningAgents)
	view.SystemHealth.AgentsIdle = activeAgents - view.SystemHealth.AgentsRunning

	recentEvents, _ := s.storage.Events().ListRecent(ctx, 10)
	view.RecentActivity = recentEvents
	if len(recentEvents) > 0 {
		view.SystemHealth.LastEventTime = timeSince(recentEvents[0].CreatedAt)
	} else {
		view.SystemHealth.LastEventTime = "No events yet"
	}

	sort.Slice(view.QuestionsForUser, func(i, j int) bool {
		return view.QuestionsForUser[i].CreatedAt.After(view.QuestionsForUser[j].CreatedAt)
	})
	if len(view.QuestionsForUser) > 5 {
		view.QuestionsForUser = view.QuestionsForUser[:5]
	}

	// Limit needs attention
	if len(view.NeedsAttention) > 5 {
		view.NeedsAttention = view.NeedsAttention[:5]
	}

	return view, nil
}

func questionsForTask(ctx context.Context, s *Server, task domain.Task) []QuestionForUser {
	if task.Mode != domain.TaskModeContinuous || task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
		return nil
	}
	messages, err := s.storage.Messages().ListByTask(ctx, task.ID)
	if err != nil {
		return nil
	}
	return openQuestionsForTask(task, messages)
}

func openQuestionsForTask(task domain.Task, messages []domain.Message) []QuestionForUser {
	if len(messages) == 0 {
		return nil
	}
	latestUserAt := time.Time{}
	for _, message := range messages {
		if message.Role == domain.MessageRoleUser && message.CreatedAt.After(latestUserAt) {
			latestUserAt = message.CreatedAt
		}
	}

	questions := make([]QuestionForUser, 0)
	for _, message := range messages {
		if message.CreatedAt.Before(latestUserAt) || message.CreatedAt.Equal(latestUserAt) {
			continue
		}
		if message.Role != domain.MessageRoleAgent && message.Role != domain.MessageRoleSystem {
			continue
		}
		body := explicitUserQuestion(strings.TrimSpace(message.Body))
		if body == "" {
			continue
		}
		questions = append(questions, QuestionForUser{TaskID: task.ID, TaskTitle: task.Title, AgentID: task.AgentID, Body: body, CreatedAt: message.CreatedAt})
	}
	return questions
}

func explicitUserQuestion(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	markers := []string{"Pergunta para você:", "Pergunta ao usuário:", "User question:", "Question for user:"}
	for _, marker := range markers {
		idx := strings.Index(strings.ToLower(body), strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		question := strings.TrimSpace(body[idx+len(marker):])
		if question == "" || !strings.Contains(question, "?") {
			return ""
		}
		return question
	}
	return ""
}

func runIssueLabel(run domain.Run) string {
	message := strings.ToLower(run.Error)
	if message == "" {
		if run.Status == domain.RunStatusTimeout {
			return "Timeout"
		}
		return ""
	}
	if strings.Contains(message, "429") || strings.Contains(message, "too many requests") || strings.Contains(message, "rate limit") || strings.Contains(message, "rate_limit") {
		return "Provider rate limit"
	}
	if strings.Contains(message, "internal_server_error") || strings.Contains(message, "internal server error") || strings.Contains(message, "code\":500") || strings.Contains(message, "status\":500") {
		return "Provider unstable"
	}
	if strings.Contains(message, "auth_unavailable") || strings.Contains(message, "no auth available") || strings.Contains(message, "unauthorized") || strings.Contains(message, "invalid api key") {
		return "Provider auth"
	}
	if strings.Contains(message, "quota") || strings.Contains(message, "insufficient_quota") {
		return "Provider quota"
	}
	if strings.Contains(message, "fork/exec") || strings.Contains(message, "no such file or directory") || strings.Contains(message, "runtime unavailable") || strings.Contains(message, "start failed") {
		return "Runtime configuration"
	}
	if strings.Contains(message, "signal: killed") {
		return "Process killed"
	}
	return "Runtime error"
}

func runIssueAdvice(run domain.Run) string {
	switch runIssueLabel(run) {
	case "Provider rate limit":
		return "Aguarde o cooldown ou troque provider/model antes de retomar a task."
	case "Provider unstable":
		return "Provider retornou 5xx em sequência; a task foi pausada para evitar cascata. Tente outro modelo/provider ou retome depois."
	case "Provider auth":
		return "Revise API key, base URL e provider/model em Settings antes de retomar."
	case "Provider quota":
		return "Revise quota/créditos do provider ou altere o modelo."
	case "Runtime configuration":
		return "Revise instalação/path do runtime e salve a configuração novamente."
	case "Process killed", "Timeout":
		return "Aumente timeout, reduza escopo da task ou verifique consumo de recursos."
	case "Runtime error":
		return "Abra stderr e tool calls para investigar o ponto de falha."
	default:
		return ""
	}
}

func runIssueClass(run domain.Run) string {
	switch runIssueLabel(run) {
	case "Provider unstable", "Provider auth", "Provider quota", "Runtime configuration":
		return "badge-bad"
	case "Provider rate limit", "Process killed", "Timeout":
		return "badge-warn"
	case "Runtime error":
		return "badge-info"
	default:
		return ""
	}
}

func timeSince(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "Just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if d < 24*time.Hour {
		hrs := int(d.Hours())
		if hrs == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hrs)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
