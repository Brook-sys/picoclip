# Robustness, Recovery, and Failure Learning

_Read this in [Portuguese / Português](ROBUSTNESS.pt-BR.md)._

PicoClip is intentionally small, but it should still behave like an operational system: failures must be visible, retry decisions must be explicit, and recovery must avoid making a bad situation worse.

This document describes the current reliability model for scheduler, dispatcher, runner, reconciler, locks, retries, wakeups and cancellation. For the wider architecture map, see [Project Map](PROJECT_MAP.md). For the current product state, see [Current State](CURRENT_STATE.md).

## Design goals

Robustness work follows these principles:

- **Fail visibly**: important failures should create persistent events, not only log lines.
- **Claim conservatively**: tasks should only be claimed when PicoClip can actually start work.
- **Recover conservatively**: recovery should unlock work safely without creating duplicate active runs.
- **Avoid retry storms**: retry should use backoff and must not bypass its own schedule.
- **Learn from failures**: retry and recovery decisions should carry structured metadata explaining what happened and why the system reacted.
- **Keep local-first simplicity**: robustness features should not require external queues, databases, or services.

## Execution lifecycle overview

The current simplified execution flow is:

1. A task is created and marked runnable with `NeedsRun=true`.
2. The scheduler runs the reconciler before dispatching new work.
3. The reconciler activates due continuous tasks, processes wakeups, detects stalls and recovers orphaned state.
4. The dispatcher waits for a concurrency slot.
5. Only after a slot is available, the dispatcher claims one runnable task atomically.
6. `ClaimNextRunnable` creates a run and writes task checkout/lock metadata.
7. The runner reloads task and agent context, checks budgets and runtime availability, builds the prompt and executes the runtime.
8. Runtime output updates run output and heartbeat metadata.
9. The runner finalizes the run/task, blocks it, schedules retry, or schedules the next continuous cycle.
10. Later reconciler passes repair stale locks, stalled runs, orphaned runs and due wakeups.

The main safety rule is: a task should not have more than one active checkout/run at the same time.

## Dispatcher concurrency safety

The dispatcher uses a bounded semaphore to respect `maxConcurrentRuns`.

Important current behavior:

- the dispatcher acquires a concurrency slot **before** calling `ClaimNextRunnable`;
- if the context is canceled before a slot is available, no task is claimed;
- if claim returns `ErrNoPendingTasks` or another error, the slot is released immediately;
- once a goroutine starts the runner, the slot is released only after `runner.Run` returns.

This prevents a task from being marked `in_progress`/checked out and a run from being created when no runner capacity exists. A regression test protects this behavior: `TestDispatcherDoesNotClaimTaskWhenConcurrencySlotUnavailable`.

## Locks and stale lock recovery

When a task is checked out for execution, PicoClip stores lock metadata on the task:

- active run ID;
- checked-out agent;
- lock start timestamp;
- lock expiration timestamp.

The lock recovery service sweeps stale locks and clears expired checkout state. If the expired lock belongs to a run that is still marked running, the run is closed as timed out and a recovery event is persisted.

This prevents tasks from staying permanently stuck after a crash, process kill, lost worker, or interrupted scheduler cycle.

## Stalled run detection

A run can define a stall timeout. If a running run has not produced output before the timeout window, the reconciler treats it as stalled.

Current behavior:

- mark the run as `timeout`;
- persist a `run.timeout` event;
- persist structured runtime liveness events (`runtime.stalled`, then `runtime.cancel_requested` and either `runtime.cancel_succeeded` or `runtime.cancel_failed` when a canceler is configured);
- ask the runtime manager to cancel the process/session;
- unlock the task;
- either schedule retry with backoff, block the task when max attempts are exhausted, or schedule the next continuous-task cycle.

Timeouts are attention-worthy activity in the UI because they usually require inspection.

## Retry scheduling and backoff

Retries are scheduled through wakeups. A retry wakeup is a durable request saying: this task may run again, but only after `DueAt`.

Current backoff formula:

```text
attempt 1 -> 30 seconds
attempt 2 -> 60 seconds
attempt 3 -> 120 seconds
attempt 4 -> 240 seconds
attempt N -> capped at 300 seconds / 5 minutes
```

A key safety property is that the task is **not** left immediately runnable while waiting for retry. The task stays `NeedsRun=false` until the wakeup is due and processed. This prevents the dispatcher from bypassing the backoff and re-running the task immediately.

## Retry metadata

Retry wakeups include structured payload data:

```text
previous_run_id
attempt
backoff_seconds
retryable
reason
```

For timeout recovery, `reason` is currently `run_timeout` and `retryable=true`.

Before creating a timeout retry, the reconciler checks whether a pending retry wakeup for the same `previous_run_id` already exists. This keeps recovery idempotent across repeated sweeps and prevents duplicate retry wakeups/events for the same failed run.

This metadata is intentionally duplicated into the activity event so humans and future automation can understand why the retry was scheduled.

## Activity events used for diagnosis

Important robustness events include:

| Event | Meaning |
| --- | --- |
| `run.timeout` | A run stopped making progress and was closed as timed out. |
| `run.recovered` | PicoClip repaired stale/orphaned run state. |
| `runtime.started` | Runner began executing a configured runtime for a run. |
| `runtime.process_started` | Adapter reported the OS process/session identifier for the runtime. |
| `runtime.heartbeat` | Adapter produced output; payload includes byte counts instead of full output to avoid noisy persisted events. |
| `runtime.completed` | Runtime execution finished with the final run status. |
| `runtime.timeout` | Runner handled a direct runtime timeout. |
| `runtime.stalled` | Reconciler detected missing output before the stall timeout. |
| `runtime.cancel_requested` | PicoClip requested cancellation of a stalled runtime run. |
| `runtime.cancel_succeeded` / `runtime.cancel_failed` | Runtime cancellation returned success or failure. |
| `retry.scheduled` | PicoClip scheduled a retry and recorded why, when, and with what backoff. |
| `budget.blocked` | Execution was blocked by a budget constraint. |
| `driver.missing` | Required runtime/driver was unavailable. |

Runtime liveness event payloads are intentionally compact. They include stable fields such as `runtime_id`, `phase`, `status`, `pid`, `stdout_bytes`, `stderr_bytes`, `reason`, and cancellation `error` when relevant. Full output still lives on the run/output stream; persisted heartbeat events use byte counts so frequent output does not spam Activity with large payloads.

The Activity page turns these into human-readable messages. For example, a retry event is presented as PicoClip learning from a timeout and scheduling retry after a specific number of seconds.

## Continuous tasks

Continuous tasks are not retried the same way one-shot tasks are. When a continuous task finishes or is recovered, PicoClip schedules the next cycle according to the task loop delay, unless the task was canceled, completed, or paused.

If a continuous task lock expires while a run is still active, recovery closes the run as timed out, clears the checkout, and moves the task to `waiting_next_cycle` with a new `LoopNextRunAt`. It does **not** create an immediate recovery wakeup and does **not** set `NeedsRun=true`. The task only becomes runnable when the next loop cycle is due and the reconciler activates it.

This keeps recurring work predictable and prevents recovery from turning a continuous loop into a tight retry loop.

## Cancellation model

Cancellation is routed through services and runtime adapters:

- task cancellation goes through `TaskLifecycle` when applicable;
- active checkout/lock state is cleared;
- the active run is closed as `canceled`;
- `RuntimeManager.CancelRun` forwards cancellation to the active adapter;
- Unix runtime adapters start subprocesses in their own process group and cancel the group with SIGTERM followed by SIGKILL when needed.

Known gap: Windows process-tree cancellation still needs Job Objects for parity with Unix process groups.

## Operational checklist

When investigating a stuck or failed task:

1. Open the task detail page and inspect the latest run.
2. Open Activity and look for `run.timeout`, `run.recovered`, `retry.scheduled`, `driver.missing`, or `budget.blocked`.
3. Check whether a retry wakeup is pending and whether its `DueAt` is in the future.
4. Confirm whether `MaxAttempts` has been reached.
5. Check runtime configuration and driver availability if the error suggests missing runtime support.
6. Use the diagnostics page or `/api/diagnostics` to inspect storage, runtime path, workspace path and configured runtime health.
7. If a task looks runnable but is not being picked up, check dispatcher capacity and whether a previous run still owns checkout/lock metadata.

## Developer checklist for robustness changes

When changing recovery, retry, cancellation, scheduling, dispatcher, runner or runtime behavior:

1. Read this document and [Development Guide](DEVELOPMENT.md).
2. Write or update a regression test first.
3. Confirm the test fails for the expected reason.
4. Implement the smallest behavior change.
5. Run focused package tests, for example:

   ```sh
   go test ./internal/core/services -run 'TestReconciler|TestStalledRun|TestDispatcher|TestLockRecovery' -count=1
   ```

6. Run `make check` before considering the change complete.
7. Confirm new failure paths create clear events or diagnostics.
8. Avoid adding retry behavior without a cap, backoff, and an event explaining the decision.
9. Update this document whenever the scheduler/dispatcher/runner/reconciler contract changes.

## Current limitations

The system is stronger than before, but still experimental. Known gaps:

- Retry classification is still basic. Timeout retries are treated as retryable, but deterministic errors are not yet fully separated into retryable vs non-retryable categories.
- There is no dedicated recovery dashboard for stale locks, retry queue, runtime health, or orphaned runs.
- Runtime liveness now has structured run-level events for start, process start, output heartbeats, direct timeout handling, stalled detection and cancellation results, but aggregate diagnostics and UI-specific liveness summaries are still limited.
- Windows process-tree cancellation still needs Job Object support for parity with Unix process-group cancellation.
- Metrics are visible through events/logs, but aggregate reliability metrics are still limited.

## Next hardening steps

Recommended next work:

1. Add explicit retry classification: `retryable`, `non_retryable`, and `unknown`.
2. Persist `retry.skipped` or `task.blocked` events when PicoClip intentionally refuses to retry.
3. Expose the retry queue and recovery state in the UI/API.
4. Add aggregate reliability counters: timeouts, recoveries, scheduled retries, skipped retries, exhausted attempts and currently locked tasks.
5. Surface runtime liveness events in compact diagnostics and UI summaries so agents can quickly explain whether a run is alive, stalled, canceling or timed out.
6. Add Windows Job Objects support for runtime process-tree cancellation.
