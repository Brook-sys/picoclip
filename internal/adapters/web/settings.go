package web

import (
	"encoding/json"
	"sort"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/services"
)

type SettingsView struct {
	General           GeneralSettings
	Adapters          map[string]map[string]string
	Environment       map[string]string
	Runtimes          []RuntimeCardView
	Diagnostics       domain.DiagnosticsReport
	Stats             SettingsStats
	Budgets           []domain.Budget
	Webhooks          []domain.WebhookSubscription
	WebhookDeliveries map[string][]domain.WebhookDelivery
	WebhookEventTypes []domain.EventType
	Agents            []domain.Agent
	Projects          []domain.Workspace
}

type GeneralSettings struct {
	Theme               string
	Density             string
	LogLevel            string
	MaxTaskRetries      string
	DefaultTaskProtocol string
}

type SettingsStats struct {
	Agents   int
	Tasks    int
	Runs     int
	Skills   int
	Projects int
	Events   int
	Messages int
}

func defaultSettingsView() SettingsView {
	return SettingsView{
		General: GeneralSettings{Theme: "system", Density: "comfortable", LogLevel: "info", MaxTaskRetries: "3", DefaultTaskProtocol: services.DefaultTaskProtocolPrompt()},
		Adapters: map[string]map[string]string{
			"crush": {"binary_path": "", "default_args": "", "timeout": "30m", "cwd_strategy": "project"},
			"noop":  {"timeout": "1m"},
		},
		Environment:       map[string]string{},
		WebhookDeliveries: map[string][]domain.WebhookDelivery{},
		WebhookEventTypes: []domain.EventType{
			domain.EventTaskCreated,
			domain.EventTaskCompleted,
			domain.EventTaskFailed,
			domain.EventTaskCanceled,
			domain.EventTaskDelegated,
			domain.EventRunStarted,
			domain.EventRunCompleted,
			domain.EventRunFailed,
			domain.EventRunCanceled,
			domain.EventMessageCreated,
			domain.EventBudgetBlocked,
		},
	}
}

func encodeSettingsValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func decodeSettingsValue[T any](raw string, fallback T) T {
	if raw == "" {
		return fallback
	}
	var value T
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fallback
	}
	return value
}

func formatEnvText(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	var keys []string
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		lines = append(lines, k+"="+env[k])
	}
	return strings.Join(lines, "\n")
}
