package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"picoclip/internal/core/domain"
	"picoclip/internal/core/ports"
)

// RuntimeCompletionAuditor executes the configured auditor directly through the
// runtime manager. It never creates a Task or Run and therefore cannot re-enter
// TaskService's completion gate.
type RuntimeCompletionAuditor struct {
	storage  ports.Storage
	runtimes *RuntimeManager
}

func NewRuntimeCompletionAuditor(storage ports.Storage, runtimes *RuntimeManager) *RuntimeCompletionAuditor {
	return &RuntimeCompletionAuditor{storage: storage, runtimes: runtimes}
}

func (a *RuntimeCompletionAuditor) AuditCompletion(ctx context.Context, request ports.CompletionAuditRequest) (ports.CompletionAuditDecision, error) {
	raw, err := a.storage.Settings().Get(ctx, completionAuditSettingsKey)
	if err != nil {
		return ports.CompletionAuditDecision{}, fmt.Errorf("audit settings: %w", err)
	}
	var config struct {
		AuditorAgentID string `json:"auditor_agent_id"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil || config.AuditorAgentID == "" {
		return ports.CompletionAuditDecision{}, fmt.Errorf("auditor agent is not configured")
	}
	if config.AuditorAgentID == request.Task.AgentID {
		return ports.CompletionAuditDecision{}, fmt.Errorf("auditor must differ from task agent")
	}
	agent, err := a.storage.Agents().Get(ctx, config.AuditorAgentID)
	if err != nil || !agent.Enabled {
		return ports.CompletionAuditDecision{}, fmt.Errorf("auditor agent unavailable")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return ports.CompletionAuditDecision{}, err
	}
	result, err := a.runtimes.Execute(ctx, domain.RuntimeID(agent.Type), ports.RuntimeExecutionInput{
		Agent: agent,
		Task:  domain.Task{ID: "audit_" + request.AuditID, AgentID: agent.ID, Title: "Completion audit", Prompt: "Return only JSON: {\"outcome\":\"approve|reject\",\"summary\":\"...\",\"findings\":[{\"code\":\"...\",\"severity\":\"info|warning|error\",\"message\":\"...\"}]}. Evidence:\n" + string(payload)},
		Run:   domain.Run{ID: "audit_" + request.AuditID, AgentID: agent.ID, DriverType: string(agent.Type)}, Config: agent.Config, Env: agent.Env, ExtraArgs: agent.ExtraArgs,
	})
	if err != nil {
		return ports.CompletionAuditDecision{}, err
	}
	var decision ports.CompletionAuditDecision
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Output)), &decision); err != nil {
		return ports.CompletionAuditDecision{}, fmt.Errorf("invalid auditor response: %w", err)
	}
	if decision.Outcome != ports.CompletionAuditApprove && decision.Outcome != ports.CompletionAuditReject {
		return ports.CompletionAuditDecision{}, fmt.Errorf("invalid auditor outcome")
	}
	return decision, nil
}

var _ ports.CompletionAuditor = (*RuntimeCompletionAuditor)(nil)
