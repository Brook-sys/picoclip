package sqlite

import (
	"context"
	"encoding/json"

	"picoclip/internal/core/ports"
)

func (s *Storage) RestoreAllData(ctx context.Context, data ports.BackupData) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM webhook_deliveries;
		DELETE FROM webhook_subscriptions;
		DELETE FROM completion_audits;
		DELETE FROM wakeups;
		DELETE FROM messages;
		DELETE FROM events;
		DELETE FROM runs;
		DELETE FROM tasks;
		DELETE FROM skills;
		DELETE FROM agents;
		DELETE FROM workspaces;
		DELETE FROM settings;
		DELETE FROM runtime_states;
	`); err != nil {
		return err
	}

	for k, v := range data.Settings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))`, k, v); err != nil {
			return err
		}
	}
	for _, x := range data.Workspaces {
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, name, description, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, x.ID, x.Name, x.Description, x.RootPath, x.CreatedAt, x.UpdatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Agents {
		tags, _ := json.Marshal(x.Tags)
		permissions, _ := json.Marshal(x.Permissions)
		skillIDs, _ := json.Marshal(x.SkillIDs)
		config, _ := json.Marshal(x.Config)
		env, _ := json.Marshal(x.Env)
		extraArgs, _ := json.Marshal(x.ExtraArgs)
		if _, err := tx.ExecContext(ctx, `INSERT INTO agents (id, project_id, name, title, reports_to_id, tags, type, description, system_prompt, instruction_file, enabled, capability, permissions, skill_ids, config, env, extra_args, input_tokens, output_tokens, total_tokens, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.ProjectID, x.Name, x.Title, x.ReportsToID, string(tags), string(x.Type), x.Description, x.SystemPrompt, x.InstructionFile, x.Enabled, string(x.Capability), string(permissions), string(skillIDs), string(config), string(env), string(extraArgs), x.InputTokens, x.OutputTokens, x.TotalTokens, x.CreatedAt, x.UpdatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Skills {
		metadata, _ := json.Marshal(x.Metadata)
		files, _ := json.Marshal(x.Files)
		defaultFiles, _ := json.Marshal(x.DefaultFiles)
		agentIDs, _ := json.Marshal(x.AgentIDs)
		allowedAgentTypes, _ := json.Marshal(x.AllowedAgentTypes)
		allowedPermissions, _ := json.Marshal(x.AllowedPermissions)
		if _, err := tx.ExecContext(ctx, `INSERT INTO skills (id, project_id, name, slug, description, license, compatibility, allowed_tools, metadata, instructions, default_instructions, files, default_files, kind, builtin_key, permission, agent_ids, allowed_agent_types, allowed_permissions, source, version, enabled, is_modified, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.ProjectID, x.Name, x.Slug, x.Description, x.License, x.Compatibility, x.AllowedTools, string(metadata), x.Instructions, x.DefaultInstructions, string(files), string(defaultFiles), string(x.Kind), x.BuiltinKey, string(x.Permission), string(agentIDs), string(allowedAgentTypes), string(allowedPermissions), x.Source, x.Version, x.Enabled, x.IsModified, x.CreatedAt, x.UpdatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Tasks {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tasks (id, parent_id, workspace_id, agent_id, title, prompt, status, priority, mode, loop_delay_seconds, loop_run_count, loop_next_run_at, loop_paused_at, loop_audit_prompt, attempts, max_attempts, needs_run, checkout_run_id, checked_out_by_agent_id, execution_locked_at, lock_expires_at, cancel_reason, input_tokens, output_tokens, total_tokens, created_at, updated_at, started_at, finished_at, completed_at, cancelled_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.ParentID, x.WorkspaceID, x.AgentID, x.Title, x.Prompt, string(x.Status), x.Priority, string(x.Mode), x.LoopDelaySeconds, x.LoopRunCount, x.LoopNextRunAt, x.LoopPausedAt, x.LoopAuditPrompt, x.Attempts, x.MaxAttempts, x.NeedsRun, x.CheckoutRunID, x.CheckedOutByAgentID, x.ExecutionLockedAt, x.LockExpiresAt, x.CancelReason, x.InputTokens, x.OutputTokens, x.TotalTokens, x.CreatedAt, x.UpdatedAt, x.StartedAt, x.FinishedAt, x.CompletedAt, x.CancelledAt); err != nil {
			return err
		}
	}
	for _, x := range data.Runs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO runs (id, task_id, agent_id, driver_type, status, attempt, input, output, error, input_tokens, output_tokens, total_tokens, process_id, last_output_at, stall_timeout, started_at, finished_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.TaskID, x.AgentID, x.DriverType, string(x.Status), x.Attempt, x.Input, x.Output, x.Error, x.InputTokens, x.OutputTokens, x.TotalTokens, x.ProcessID, x.LastOutputAt, x.StallTimeout, x.StartedAt, x.FinishedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Runtimes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_states (id, runtime_id, mode, enabled, version, bin_path, config_path, home_path, data_path, logs_path, source, source_url, checksum, installed_at, updated_at, last_health_at, last_health_json, settings_json, metadata_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.RuntimeID, x.Mode, x.Enabled, x.Version, x.BinPath, x.ConfigPath, x.HomePath, x.DataPath, x.LogsPath, x.Source, x.SourceURL, x.Checksum, x.InstalledAt, x.UpdatedAt, x.LastHealthAt, x.LastHealthJSON, x.SettingsJSON, x.MetadataJSON); err != nil {
			return err
		}
	}
	for _, x := range data.Wakeups {
		payload, _ := json.Marshal(x.Payload)
		if _, err := tx.ExecContext(ctx, `INSERT INTO wakeups (id, agent_id, task_id, reason, status, priority, due_at, claimed_at, payload, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.AgentID, x.TaskID, string(x.Reason), string(x.Status), x.Priority, x.DueAt, x.ClaimedAt, string(payload), x.CreatedAt, x.UpdatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Usage {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_events (id, run_id, task_id, agent_id, provider, model, input_tokens, output_tokens, cached_tokens, cost_micros, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.RunID, x.TaskID, x.AgentID, x.Provider, x.Model, x.InputTokens, x.OutputTokens, x.CachedTokens, x.CostMicros, x.CreatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Messages {
		if _, err := tx.ExecContext(ctx, `INSERT INTO messages (id, task_id, from_id, to_id, role, body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, x.ID, x.TaskID, x.FromID, x.ToID, string(x.Role), x.Body, x.CreatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Events {
		dataJSON, _ := json.Marshal(x.Data)
		if _, err := tx.ExecContext(ctx, `INSERT INTO events (id, type, task_id, agent_id, run_id, message, data, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, string(x.Type), x.TaskID, x.AgentID, x.RunID, x.Message, string(dataJSON), x.CreatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Budgets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO budgets (id, scope, workspace_id, agent_id, limit_tokens, limit_runs, limit_cost_micros, hard_stop, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, string(x.Scope), x.WorkspaceID, x.AgentID, x.LimitTokens, x.LimitRuns, x.LimitCostMicros, x.HardStop, x.Enabled, x.CreatedAt, x.UpdatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.Webhooks {
		eventTypes, _ := json.Marshal(x.EventTypes)
		if _, err := tx.ExecContext(ctx, `INSERT INTO webhook_subscriptions (id, name, url, secret, event_types, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.Name, x.URL, x.Secret, string(eventTypes), x.Enabled, x.CreatedAt, x.UpdatedAt); err != nil {
			return err
		}
	}
	for _, x := range data.CompletionAudits {
		if _, err := tx.ExecContext(ctx, `INSERT INTO completion_audits (id, task_id, requested_by_agent_id, outcome, summary, findings_json, requested_at, decided_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, x.ID, x.TaskID, x.RequestedByAgentID, string(x.Outcome), x.Summary, x.FindingsJSON, x.RequestedAt, x.DecidedAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}
