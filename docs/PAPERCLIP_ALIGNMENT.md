# PicoClip Paperclip Alignment Study

This document is a strategic and technical study for evolving PicoClip toward the same functional goals as Paperclip while preserving PicoClip's differentiators:

- smaller Go binary;
- lower RAM usage;
- lower token overhead;
- local-first operation;
- modular runtime/storage/agent architecture;
- simple server-rendered UI;
- stronger portability;
- fewer infrastructure dependencies.

Paperclip is the functional target. Sortie is a useful Go reference for orchestration, adapters, durability and operational rigor, but it solves a narrower problem: running coding agents from external trackers. PicoClip should learn from Sortie technically without becoming Sortie.

## Executive Summary

PicoClip already has many of the correct primitives:

- projects/workspaces;
- agents with hierarchy fields;
- tasks with Paperclip-like statuses;
- subtasks/delegation;
- comments/messages;
- runs;
- runtime adapters;
- skills;
- permissions model;
- SQLite persistence;
- events/SSE;
- token counters;
- server-rendered HTMX UI.

The main gap is not raw CRUD. The main gap is **behavioral correctness and operational semantics**:

1. Atomic checkout and lock recovery must become a first-class state machine.
2. Agent execution should move from a simple run-on-task model toward a heartbeat/wakeup model.
3. Agents need a compact, structured API and context flow similar to Paperclip's heartbeat context.
4. Permissions exist but must be enforced consistently.
5. Runs need stronger lifecycle, retry, liveness and cancellation semantics.
6. Costs/tokens need to become a ledger rather than only counters.
7. UI should expose inbox/attention/recovery states, not only object details.
8. Skills should become richer packages with files, metadata and assignment rules.
9. Workspaces should become execution boundaries, not just project folders.
10. Adapter/runtime APIs should evolve toward evented sessions, normalized usage and process control.

The recommended path is incremental: keep PicoClip's small core and local-first web UI, then add Paperclip-compatible behavior layer by layer.

## Product North Star

PicoClip should feel like a local, lightweight Paperclip-compatible control plane:

- Human creates or reviews issues/tasks.
- Agents have roles, permissions, skills and reporting structure.
- Work is assigned, checked out, performed, commented on, delegated or completed.
- Runs are observable and recoverable.
- The UI explains what needs attention.
- The system can run locally with low memory and minimal dependencies.
- The prompts sent to agents are compact and specific, avoiding token bloat.

PicoClip should not become a heavyweight SaaS clone. It should be the lean single-node implementation of the same operating model.

## Reference Systems

### Paperclip as Functional Target

Paperclip's core semantics:

- issues are the unit of work;
- status lifecycle: `backlog`, `todo`, `in_progress`, `in_review`, `blocked`, `done`, `cancelled`;
- atomic checkout moves an issue to `in_progress` and records run ownership;
- agents work in heartbeats;
- agents inspect inbox, checkout work, fetch context, work, comment, update status and delegate;
- comments are the shared communication surface;
- parent/child issues and blockers shape work decomposition;
- permissions and org hierarchy constrain actions;
- token/cost usage is recorded and budgeted;
- UI surfaces inbox, issue details, activity, approvals, cost and org state.

Relevant local Paperclip references discovered:

- `packages/shared/src/constants.ts`: status constants and inbox statuses.
- `packages/shared/src/validators/issue.ts`: issue create/update schemas and child issue schema.
- `packages/db/src/schema/issues.ts`: issue schema including checkout/execution fields.
- `packages/db/src/schema/issue_comments.ts`: comment model.
- `packages/db/src/schema/heartbeat_runs.ts`: heartbeat run lifecycle.
- `packages/db/src/schema/cost_events.ts`: cost/token ledger.
- `server/src/services/issues.ts`: issue transitions, checkout and side effects.
- `server/src/services/heartbeat.ts`: wakeup/heartbeat execution model.
- `server/src/services/recovery/service.ts`: stale lock recovery.
- `server/src/services/authorization.ts`: permission decisions.
- `docs/guides/agent-developer/heartbeat-protocol.md`: operational agent contract.
- `docs/guides/board-operator/delegation.md`: delegation pattern.

### Sortie as Technical Reference

Sortie is useful because it is Go-first and shares several constraints with PicoClip:

- single binary;
- SQLite persistence;
- adapter-based integration;
- local workspace isolation;
- bounded concurrency;
- retry/backoff;
- state reconciliation;
- agent/runtime abstraction;
- low operational overhead.

Sortie differs from the target:

- it orchestrates external tracker issues, not an internal multi-agent org;
- it runs one workflow per process;
- it does not provide Paperclip-style org hierarchy, UI, comments/inbox or internal task graph as the primary product;
- it treats the tracker as the human control surface.

Useful Sortie patterns to adapt:

- one authority owns scheduling state;
- separate orchestration state from user-visible task state;
- durable retry queue;
- explicit retryable vs non-retryable errors;
- workspace path containment;
- evented adapter contract;
- normalized agent events and token usage;
- fail-safe file-based agent status signal;
- MCP sidecar for data plane where supported;
- spec-first state machine documentation.

## PicoClip Current State

PicoClip already has:

- `domain.Task` with `Title`, `Prompt`, `ParentID`, `AgentID`, status, checkout fields, token counters.
- `domain.Agent` with hierarchy (`ReportsToID`), capabilities, permissions, skills, runtime type/config/env.
- `domain.Run` with runtime, status and token counters.
- `domain.Message` as task comments/messages.
- `domain.Workspace` for projects.
- `domain.Skill` with built-in/custom type and assignment filters.
- `RuntimeAdapter` interface and adapters for `crush`, `picoclaw`, `claurst`.
- SQLite and memory storage adapters.
- Server-rendered UI pages for dashboard, projects, agents, tasks, runs, skills, settings and activity.
- Agent API endpoints for agents to read tasks/projects/skills and comment/delegate/cancel.
- Runtime install/config/test UI.
- Basic event bus and SSE activity.
- Prompt protocol that instructs agents to satisfy title/description/latest comment and mark done/blocked/delegate.

Key local PicoClip references:

- `internal/core/domain/task.go`
- `internal/core/domain/agent.go`
- `internal/core/domain/run.go`
- `internal/core/domain/message.go`
- `internal/core/services/task_service.go`
- `internal/core/services/runner.go`
- `internal/core/services/dispatcher.go`
- `internal/core/services/scheduler.go`
- `internal/core/services/runtime_manager.go`
- `internal/core/services/capabilities.go`
- `internal/adapters/runtimes/*`
- `internal/adapters/storage/sqlite/*`
- `internal/adapters/web/*`

## Major Differences: Paperclip vs PicoClip vs Sortie

| Area | Paperclip | PicoClip today | Sortie | Recommended PicoClip direction |
| --- | --- | --- | --- | --- |
| Work unit | Issue | Task | External tracker issue | Keep `Task`, present as issue-like |
| Lifecycle | Strict status transitions | Statuses exist, partial rules | Orchestration states + tracker states | Add explicit state machine and transition policy |
| Checkout | Atomic with run ID and locks | Checkout exists, simpler | Atomic claim | Strengthen checkout lock/run ownership |
| Execution model | Heartbeat/wakeup | Scheduler dispatches pending tasks | Poll-dispatch-reconcile | Add heartbeat queue and wake reasons |
| Agent context | Inbox + heartbeat-context | Prompt assembled in runner | Template rendered per issue | Add compact context endpoint/snapshot |
| Comments | Issue comments, thread semantics | Messages/comments | Tracker comments optional | Expand comments/inbox/attention semantics |
| Delegation | Child issues, plans, approval possible | Child tasks/delegate exists | Subagents optional but not core | Add plan decomposition and parent blocking |
| Permissions | Enforced authz grants | Model exists, partial enforcement | Config boundary | Enforce endpoint/action permissions |
| Cost/tokens | Ledger + budgets | Counters + estimates | Budget caps | Add usage events and budgets |
| UI | Dashboard, inbox, issue detail, costs, org | Basic UI plus redesigned details | Dashboard/metrics | Add inbox/attention/org/cost views |
| Runtime | Adapter system | Runtime adapters | AgentAdapter/session/events | Evolve runtime adapter to evented sessions |
| Workspace | Execution environment | Project root path | Per-issue isolated dirs | Add per-task work dirs and hooks |
| Recovery | Stale locks, stranded runs | Basic cancellation limitations | Reconcile/stall detection | Add reconciliation loop |

## Core Principle: Two State Layers

PicoClip should separate:

1. **Task lifecycle state**: visible to humans and agents.
2. **Orchestration state**: internal scheduling/execution state.

Today, `Task.Status`, `NeedsRun`, `CheckoutRunID`, `CheckedOutByAgentID` and `Run.Status` are mixed enough to work, but not enough for robust recovery.

Recommended internal concepts:

```text
Task status:
backlog | todo | in_progress | in_review | blocked | done | cancelled

Execution claim state:
unclaimed | claimed | running | retry_queued | released

Run status:
queued | starting | running | succeeded | failed | timed_out | cancelled | stale
```

This mirrors Sortie's distinction while preserving Paperclip's visible issue lifecycle.

## Area-by-Area Improvement Study

### 1. Task/Issue Lifecycle

Current PicoClip:

- Has Paperclip-like statuses.
- Create defaults to `todo`.
- `NeedsRun` controls scheduler eligibility.
- `UpdateStatus` handles done/blocked/cancelled side effects.
- Comments on done task create follow-up child task.
- `TaskLifecycle` now has an explicit transition matrix for `backlog`, `todo`, `in_progress`, `waiting_next_cycle`, `in_review`, `blocked`, `done` and `cancelled`, with tests covering the matrix, invalid edges and key side effects.

Gaps:

- `NeedsRun` can become a hidden second lifecycle if not formalized.
- `in_review` is present but not deeply used.
- `backlog` semantics are underused.
- Status side effects are centralized in `TaskLifecycle`, but some service-level paths still add procedural effects around it, such as cancellation of the active run and runtime adapter.

Recommended improvements:

1. Add `TaskLifecycle` service or package with transition table:
   - allowed from/to statuses;
   - required comment for blocked/done/cancelled;
   - timestamp side effects;
   - checkout release side effects;
   - wakeup side effects.
2. Preserve simple API, but move status validation out of handlers.
3. Add tests for every transition.
4. Make `in_review` useful:
   - agent can mark `in_review` when work is ready but needs human check;
   - UI shows review queue;
   - comment from user can wake it back to `todo` or create follow-up.
5. Add `backlog` behavior:
   - unassigned or not-ready tasks default to backlog;
   - assignment moves to todo if agent is enabled.

Minimal schema additions:

- `ReviewRequestedAt`
- `BlockedAt`
- maybe `LastHumanMessageAt`, `LastAgentMessageAt`

### 2. Atomic Checkout and Locking

Current PicoClip:

- `Checkout` exists and uses expected statuses.
- Task has checkout fields.
- Runner marks task `in_progress`.

Paperclip target:

- checkout is atomic;
- checkout includes run ID;
- conflicts are explicit and agents do not retry blindly;
- stale locks are swept;
- lock owner and execution run are tracked.

Sortie reference:

- claim state prevents duplicate dispatch;
- reconciliation clears stale claims.

Recommended improvements:

1. Introduce `CheckoutRunID` as mandatory for agent checkout during runs.
2. Add `ExecutionLockedAt` or reuse `StartedAt` more explicitly.
3. Add `LockExpiresAt` or timeout policy.
4. Add stale lock sweeper:
   - if run missing/terminal and task still locked, clear lock;
   - if run silent for too long, mark stale or retry.
5. Expose `409 Conflict` consistently in Agent API.
6. Add `Release` endpoint/operation that clears checkout without completing.

Potential invariant:

```text
A task can have at most one active checkout.
A running run must point to exactly one checked-out task.
A task in terminal status cannot remain checked out.
```

### 3. Heartbeat/Wakeup Execution Model

Current PicoClip:

- scheduler periodically dispatches pending tasks;
- runner executes one task;
- wake task exists.

Paperclip target:

- agents run heartbeats;
- wake reasons include schedule, assignment, user comment, manual wake, approval, retry;
- heartbeat starts by reading agent identity/inbox;
- agent chooses work and checks out.

Sortie reference:

- polling tick reconciles first, validates config, fetches candidates, dispatches.

Recommended PicoClip model:

```text
WakeupRequest:
ID
AgentID
Reason: assignment | comment | manual | retry | schedule | recovery
TaskID optional
Priority
DueAt
Attempts
Status: queued | running | consumed | cancelled
CreatedAt
```

Execution flow:

1. Event creates wakeup request.
2. Scheduler reconciles running runs.
3. Dispatcher picks due wakeup requests by priority.
4. Runner starts heartbeat for agent.
5. Agent sees compact inbox/context and checks out task.
6. Runner records heartbeat outcome.

Why this matters:

- matches Paperclip more closely;
- avoids over-prompting every task immediately;
- lets agent choose among assigned work;
- supports comments and manual wake better;
- makes retry/recovery explicit.

Low-token variant:

- first heartbeat prompt contains protocol + compact inbox only;
- detailed task context is fetched via API or injected only after checkout;
- avoid embedding all skills/messages unless needed.

### 4. Agent Inbox and Attention Model

Current PicoClip:

- agents can list tasks;
- `GET /agent-api/agents/me/inbox-lite` returns compact task items with wake `reason` and an `attention` boolean;
- comment wakeups now surface as `reason="comment"` and `attention=true`;
- dashboard shows some task/run information;
- no rich inbox classification beyond the compact Agent API signal.

Paperclip target:

- inbox includes assigned issues needing attention;
- blocked/review/comment states are surfaced;
- agents prioritize `in_progress`, `in_review`, `todo`.

Recommended improvements:

1. Add `AgentInboxItem` view model:
   - task id/title/status;
   - reason: assigned, checked_out, human_comment, blocked, review_requested, failed_run, delegated_child_done;
   - severity: info/warn/error;
   - last activity timestamp;
   - compact counts: unread comments, failed runs, children open.
2. Add endpoint:
   - `GET /agent-api/agents/me/inbox-lite`
   - `GET /agent-api/tasks/{id}/heartbeat-context`
3. Add human UI page or dashboard block:
   - "Needs attention";
   - "Blocked";
   - "In review";
   - "Failed runs".

Token-saving design:

- `inbox-lite` should be extremely compact.
- `heartbeat-context` should include only selected task and the latest relevant comments/events.

### 5. Comments, Messages and Thread Semantics

Current PicoClip:

- `Message` has role and task id.
- User comment can reopen active/blocked/review work, creates/updates a deduplicated pending `WakeupRequest.reason=comment`, and creates a follow-up child task when the original task is done.
- Comment wakeups carry compact payload metadata such as `message_id`, `from_id` and `to_id` for inbox/heartbeat triage.
- Timeline is chronological.

Paperclip target:

- comments are central communication medium;
- comments have author type, run id, metadata/presentation;
- comments drive wakeups and inbox.

Recommended improvements:

1. Extend `Message`:
   - `CreatedByRunID`
   - `MetadataJSON`
   - `Presentation` or `Kind`: comment/status_update/system_event/delegation/review
   - soft-delete fields eventually.
2. Add unread/seen model for agents/humans if needed later.
3. Continue hardening comment-driven wakeup requests:
   - human comment on active task wakes assignee through a deduplicated pending `WakeupRequest.reason=comment`;
   - comment on done task creates follow-up child, as already started;
   - future work can add per-agent unread/seen state and richer comment kinds.
4. Keep messages concise in prompt:
   - include latest user comment;
   - include last N comments by relevance, not entire thread.

### 6. Delegation and Decomposition

Current PicoClip:

- `Delegate` creates child task.
- ParentID exists.
- UI shows subtasks visually.

Paperclip target:

- delegation can create plans;
- parent can be blocked until children complete;
- managers/coordinators delegate based on org hierarchy and permissions.

Recommended improvements:

1. Add delegation policy fields:
   - `BlockParentUntilDone`;
   - `DelegationReason`;
   - `CreatedByRunID`;
   - `SuggestedAssigneeID` vs actual assignee.
2. Add parent aggregation:
   - parent shows child completion summary;
   - child done can wake parent assignee/coordinator.
3. Add planner flow:
   - agent proposes decomposition comment;
   - optional human approval later;
   - create subtasks in batch.
4. Add manager-chain permission rule:
   - coordinators can delegate to reports;
   - administrators can assign anyone.

Keep it lean:

- no board/kanban needed initially;
- no complex approvals until base delegation is robust.

### 7. Agent Hierarchy and Org Model

Current PicoClip:

- agents have `ReportsToID`, title, capability, tags.
- UI has agent detail.

Paperclip target:

- org chart matters for delegation, wakeups and authority.

Recommended improvements:

1. Add Org page:
   - tree of agents;
   - status per agent;
   - current tasks and running runs.
2. Add derived role/capability indicators:
   - observer, worker, coordinator, operator, administrator.
3. Add manager-chain helpers:
   - `IsManagerOf(managerID, agentID)`;
   - permission scope checks.
4. Add per-agent runtime state summary:
   - last heartbeat;
   - current run;
   - token usage;
   - blocked tasks.

### 8. Permissions and Enforcement

Current PicoClip:

- permissions model is rich;
- enforcement is partial.

Paperclip target:

- authorization decisions are explicit;
- agents operate with scoped powers;
- human/operator has broader powers.

Recommended improvements:

1. Add central `Authorizer` service:
   - `Can(actor, action, resource)`;
   - decision includes reason string.
2. Enforce on admin and agent APIs:
   - task create/update/delegate/cancel;
   - agent create/delete/update;
   - skills manage;
   - settings/runtime manage.
3. Distinguish actors:
   - local admin UI;
   - agent runtime token;
   - internal system.
4. Add API key/JWT later:
   - initially local single-user may bypass human auth;
   - agent API should still carry agent identity/run id.
5. Log denied actions as events.

Low-overhead approach:

- implement as plain Go service with static permission strings;
- avoid complex RBAC engine dependency.

### 9. Runtime/Adapter Architecture

Current PicoClip:

- `RuntimeAdapter.Execute` runs one command and returns combined output.
- Health/config/install are generic.
- Runtime install UI works.

Paperclip target:

- adapters support process metadata, logs, usage, cancel, runtime state.

Sortie reference:

- `AgentAdapter` has session lifecycle, turns, event stream, normalized events.

Recommended evolution:

```go
type RuntimeSession interface {
    ID() string
    Stop(ctx context.Context) error
}

type RuntimeEvent struct {
    Type string
    Message string
    ToolName string
    TokenUsage *TokenUsage
    At time.Time
}
```

Phases:

1. Keep `Execute` for simple adapters.
2. Add optional `ExecuteStream` or `StartSession` interface.
3. Capture stdout/stderr incrementally into run logs.
4. Store process PID/start/last output timestamp.
5. Add cancel that actually kills process tree.
6. Parse real token usage when runtime supports it.

Adapter goals:

- `noop`: test only.
- `crush`: parse usage if available.
- `picoclaw`: parse `UsageInfo` fields if emitted.
- `claurst`: support only when binary compatible; document glibc note.
- future: opencode/codex/claude-code style adapters.

### 10. Workspace Isolation and Hooks

Current PicoClip:

- project workspace folders exist.
- runtime command likely runs from server cwd or default process cwd.

Paperclip/Sortie target:

- agent work happens inside an isolated workspace;
- workspace path is validated;
- hooks prepare repo/branch before run.

Recommended improvements:

1. Add task execution directory:
   - `workspaces/<project-id>/tasks/<task-id>/` or `workspaces/<project-id>/runs/<run-id>/`.
2. Validate path containment.
3. Runtime executes with `cmd.Dir = workspace path`.
4. Add workspace hooks:
   - after_create;
   - before_run;
   - after_run;
   - before_remove.
5. Add UI for workspace root and task workdir.
6. Persist workspace metadata per task/run.

Benefits:

- safer execution;
- better reproducibility;
- retries can resume partial work;
- closer to Sortie's proven model.

### 11. Retry, Backoff, Reconciliation and Recovery

Current PicoClip:

- attempts/max attempts exist;
- scheduler/dispatcher simple;
- cancellation does not always kill external process immediately.

Paperclip/Sortie target:

- retry queue is explicit;
- stale locks/runs are reconciled;
- failure classes decide retry behavior;
- no silent lost work.

Recommended improvements:

1. Add `RetryQueue` or fields on `Run/Task`:
   - `RetryDueAt`;
   - `RetryReason`;
   - `RetryAttempt`;
   - `BackoffUntil`.
2. Classify errors:
   - runtime missing: non-retryable;
   - incompatible binary: non-retryable;
   - timeout: retryable;
   - API/rate limit: retryable;
   - permission denied: non-retryable;
   - agent marked blocked: release/no retry.
3. Add reconciliation tick before dispatch:
   - recover stale running tasks;
   - kill stale processes;
   - clear locks for terminal tasks;
   - requeue eligible work.
4. Add run liveness:
   - `LastOutputAt`;
   - `LastHeartbeatAt`;
   - `ProcessID`;
   - `StallTimeout`.
5. Add admin UI for recovery actions.

### 12. Agent Communication: API, MCP and File Signals

Current PicoClip:

- Agent API exists.
- Prompt lists available APIs.
- No MCP sidecar or file status protocol.

Paperclip target:

- agents call HTTP API.
- run id and auth are injected.

Sortie reference:

- MCP for data plane;
- `.sortie/status` for control plane.

Recommended PicoClip approach:

1. Keep HTTP Agent API as primary and simple.
2. Add optional file signal support:
   - `.picoclip/status` values: `blocked`, `needs-human-review`, `done`.
   - read after runtime exits.
   - advisory, not authoritative.
3. Add optional MCP server later:
   - expose task context, comment, delegate, status, cost budget.
   - useful for Claude Code/OpenCode/Codex style runtimes.
4. Inject env:
   - `PICOCLIP_AGENT_ID`;
   - `PICOCLIP_RUN_ID`;
   - `PICOCLIP_TASK_ID`;
   - `PICOCLIP_API_BASE`;
   - `PICOCLIP_API_TOKEN` eventually;
   - `PICOCLIP_WORKSPACE`.

This preserves compatibility with simple CLIs while enabling modern agents.

### 13. Prompt and Token Overhead Reduction

Current PicoClip:

- runner builds enriched prompt with permissions, skills, messages, available APIs.
- default protocol is already compact and customizable.

Risks:

- skills/messages can grow;
- full context injection can become expensive;
- repeated prompts can waste tokens.

Recommended improvements:

1. Split prompt into layers:
   - stable protocol prompt;
   - compact task summary;
   - latest user request;
   - API affordances;
   - only relevant skills.
2. Add context budgets:
   - max comment chars;
   - max skill chars;
   - max file chars;
   - max event history.
3. Add summarization/cache fields:
   - task context summary;
   - agent memory summary;
   - last run outcome summary.
4. Prefer API retrieval:
   - do not inject full task tree unless needed;
   - agent can fetch more via endpoint.
5. Add continuation prompts:
   - first run gets full compact context;
   - retries/continuations get delta + last failure.

Guiding rule:

```text
Prompt should say what to do now, not serialize the whole database.
```

### 14. Cost and Token Ledger

Current PicoClip:

- Run/Task/Agent token counters exist.
- Tokens are estimated.

Paperclip target:

- cost events ledger;
- provider/model/billing type;
- monthly budgets;
- hard/soft limits.

Recommended improvements:

1. Add `UsageEvent` table:
   - ID, RunID, TaskID, AgentID, ProjectID;
   - Provider, Model, RuntimeID;
   - InputTokens, OutputTokens, CachedInputTokens;
   - Estimated bool;
   - CostMicros or CostCents;
   - CreatedAt.
2. Keep aggregate counters for quick UI.
3. Add budget config:
   - per agent monthly token/cost cap;
   - per project cap;
   - global cap.
4. Add budget actions:
   - warn;
   - disable runtime/agent;
   - block new runs.
5. Parse real usage from runtimes where possible.

PicoClip differentiator:

- make cost tracking optional and lightweight;
- no external billing services;
- estimates allowed but clearly marked.

### 15. Skills System

Current PicoClip:

- built-in/custom skills exist;
- one optional file through UI;
- assignment filters exist.

Paperclip target:

- skills as bundles with `SKILL.md`, files, tools, metadata and catalogs.

Recommended improvements:

1. Support multi-file skills in UI.
2. Add import/export directory format:
   - `SKILL.md`;
   - `files/*`;
   - metadata JSON/YAML.
3. Add skill versioning and modification status.
4. Add permission/tool requirements per skill.
5. Add skill selection by task/project/agent tags.
6. Add prompt budget controls for skill injection.

### 16. UI Roadmap

Current PicoClip UI is already lightweight and improving.

Paperclip-like UI additions:

1. Inbox page:
   - assigned to me/agent;
   - needs attention;
   - blocked;
   - in review;
   - failed runs.
2. Org chart:
   - agent hierarchy;
   - current load/status;
   - manager relationships.
3. Cost/usage page:
   - runs/tasks/agents/projects usage;
   - estimates vs real usage.
4. Recovery page:
   - stale locks;
   - failed runs;
   - runtime health;
   - retry queue.
5. Improved task detail:
   - blocker relations;
   - parent/child progress;
   - run transcript/log streaming;
   - compact activity grouping.
6. Agent detail:
   - inbox;
   - reports;
   - skills;
   - budget;
   - last heartbeat.

Keep UI constraints:

- no heavy SPA;
- server-rendered Templ;
- HTMX for forms and partial refresh;
- SSE for activity/log streaming;
- progressive disclosure for advanced settings.

### 17. External Trackers

Paperclip target is internal issue/task system. Sortie target is external trackers.

Recommendation:

- Do not make external trackers core yet.
- Add tracker adapters later as sync/import/export modules.
- Keep PicoClip internal Task as source of truth.

Potential future interface:

```go
type TrackerAdapter interface {
    Pull(ctx) ([]ExternalIssue, error)
    PushStatus(ctx, task Task) error
    PostComment(ctx, task Task, comment Message) error
}
```

This can import from GitHub/Jira/Linear without surrendering PicoClip's local-first model.

## Proposed Phased Roadmap

### Phase 1: Hardening the Existing Core

Goal: make current PicoClip behavior reliable and Paperclip-compatible enough for local use.

Tasks:

1. Formal task transition matrix.
2. Strong checkout/run ownership invariants.
3. Stale lock/run reconciliation.
4. Agent inbox-lite endpoint.
5. Heartbeat context endpoint.
6. Permission enforcement service for existing endpoints.
7. Runtime process cancellation and liveness metadata.
8. UsageEvent ledger with estimated tokens.

Impact:

- strong correctness;
- lower duplicated work;
- better agent prompts;
- closer Paperclip workflow.

### Phase 2: Heartbeat/Wakeup Engine

Goal: move from task dispatch to agent heartbeat execution.

Tasks:

1. Add `WakeupRequest` table/model.
2. Create wakeups on assignment/comment/manual/retry/schedule.
3. Runner starts agent heartbeat with compact inbox.
4. Agent checks out task through API.
5. Add wake reason and run context env vars.
6. Add retry queue/backoff.

Impact:

- much closer to Paperclip;
- lower token overhead;
- better multi-task agent behavior.

### Phase 3: UI and Operator Control

Goal: expose the Paperclip operational surface.

Tasks:

1. Inbox page.
2. Org chart page.
3. Usage/cost dashboard.
4. Recovery/health dashboard.
5. Task blockers and relation UI.
6. Better run logs/transcripts.

Impact:

- humans can operate the system confidently;
- issues needing attention become visible.

### Phase 4: Advanced Runtime and Workspace Model

Goal: make execution robust and portable.

Tasks:

1. Per-task workspace directories.
2. Workspace lifecycle hooks.
3. Optional `.picoclip/status` file protocol.
4. Optional MCP server sidecar.
5. Evented runtime sessions.
6. Real usage parsing per runtime.
7. Self-review commands/verification loop.

Impact:

- safer execution;
- more agent compatibility;
- better cost/quality control.

### Phase 5: Integrations and Portability

Goal: connect to the ecosystem without bloating core.

Tasks:

1. Skill package import/export.
2. Backup/restore improvements.
3. Optional GitHub/Jira/Linear tracker sync adapters.
4. Systemd/launchctl service docs.
5. Runtime compatibility diagnostics.
6. Release artifacts and installer refinements.

Impact:

- broader adoption;
- still local-first;
- no mandatory external services.

## Implementation Priority Ranking

Highest leverage next steps:

1. **Formal lifecycle/checkout/reconciliation**: prevents correctness bugs.
2. **Inbox-lite + heartbeat-context**: lowers token overhead and matches Paperclip.
3. **WakeupRequest model**: unlocks Paperclip-style heartbeat.
4. **Permission enforcement**: makes capabilities real.
5. **UsageEvent ledger**: makes token/cost tracking reliable.
6. **Runtime cancellation/liveness**: improves operational safety.
7. **Workspace execution isolation**: improves safety and repeatability.

Recommended immediate implementation order:

```text
1. Task transition matrix
2. Run/checkout invariants and stale lock recovery
3. Agent inbox-lite endpoint
4. Heartbeat context endpoint
5. WakeupRequest model
6. Permission enforcement pass
7. UsageEvent ledger
```

## Design Guardrails

PicoClip should preserve these constraints while growing:

- No heavy frontend framework.
- No required external database server.
- No Redis/message queue dependency.
- No hidden global SaaS assumptions.
- No bloated prompts by default.
- No integration-specific concepts in core domain.
- Runtime adapters should be isolated packages.
- Storage should remain behind ports.
- Advanced features should degrade gracefully.

## Token Overhead Strategy

Paperclip-like functionality can become expensive if implemented by serializing too much state into every prompt. PicoClip should explicitly optimize for low token usage.

Rules:

1. The agent prompt should include only current objective, latest user comment, relevant status and available APIs.
2. Historical details should be fetched via API when needed.
3. Skills should be selected and truncated by relevance.
4. Runs should have continuation mode.
5. Context summaries should be persisted and reused.
6. Agent API should offer compact endpoints first.

Suggested compact heartbeat prompt shape:

```text
You are <agent>. Wake reason: <reason>.
Use the PicoClip Agent API.
Check inbox-lite.
If you choose work, checkout exactly one task.
Fetch heartbeat-context for the checked out task.
Work in workspace path.
Comment with result.
Set status done/blocked/in_review or delegate.
```

Then agent fetches detail only after checkout.

## Memory/CPU Overhead Strategy

Keep runtime characteristics lean:

- one process scheduler;
- bounded goroutines;
- SQLite transactions instead of background queues;
- no in-memory giant caches;
- SSE only for active viewers;
- log tailing with bounded buffers;
- prompt context built on demand;
- no always-on browser/websocket SPA.

## Conclusion

PicoClip is already pointed in the right direction. The most important next evolution is to formalize behavior, not add more surface area.

To become Paperclip-like while staying lighter, PicoClip should focus on:

- robust issue/task lifecycle;
- heartbeat/wakeup execution;
- compact agent APIs;
- real permission enforcement;
- durable retry/recovery;
- workspace isolation;
- usage ledger;
- operator UI for attention and recovery.

Sortie validates many of the technical choices already made: Go, SQLite, single binary, adapters, reconciliation, workspace isolation and bounded retries. Paperclip defines the product behavior. PicoClip should combine Paperclip's operating model with Sortie's Go operational discipline and PicoClip's own lean UI/local-first constraints.
