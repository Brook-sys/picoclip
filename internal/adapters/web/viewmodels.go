package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

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

	for _, task := range allTasks {
		switch task.Status {
		case domain.TaskStatusBlocked:
			view.Stats.BlockedTasks++
			view.NeedsAttention = append(view.NeedsAttention, s.taskResponse(r, task))
		case domain.TaskStatusDone:
			view.Stats.DoneTasks++
		case domain.TaskStatusTodo, domain.TaskStatusBacklog, domain.TaskStatusInProgress, domain.TaskStatusInReview:
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

	// Limit needs attention
	if len(view.NeedsAttention) > 5 {
		view.NeedsAttention = view.NeedsAttention[:5]
	}

	return view, nil
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
