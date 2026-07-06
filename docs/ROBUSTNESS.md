# Robustness, Recovery, and Failure Learning

_Read this in [Portuguese / Português](ROBUSTNESS.pt-BR.md)._

PicoClip is intentionally small, but it should still behave like an operational system: failures must be visible, retry decisions must be explicit, and recovery should avoid making a bad situation worse.

This document explains the current robustness model and the direction for future hardening.

## Design goals

PicoClip robustness work follows these principles:

- **Fail visibly**: important failures should create persistent events, not only log lines.
- **Recover conservatively**: recovery should unlock work safely without creating duplicate active runs.
- **Avoid retry storms**: retry should use backoff and must not bypass its own schedule.
- **Learn from failures**: retry and recovery decisions should carry structured metadata explaining what happened and why the system reacted.
- **Keep local-first simplicity**: robustness features should not require external queues, databases, or services.

## Execution lifecycle overview

The simplified execution flow is:

1. A task is created and marked runnable.
2. The dispatcher claims one runnable task atomically.
3. The runner creates a run and locks the task to that run.
4. A runtime adapter executes the work.
5. The run ends as completed, failed, canceled, or timed out.
6. The reconciler periodically repairs stale state and processes scheduled wakeups.

The important safety rule is that a task should not have more than one active checkout/run at the same time.

## Locks and stale lock recovery

When a task is checked out for execution, PicoClip stores lock metadata on the task, including the active run ID and lock expiration data.

The lock recovery service sweeps stale locks and clears expired checkout state. If the expired lock belongs to a run that is still marked running, the run is closed as timed out and a recovery event is persisted.

This prevents tasks from being permanently stuck after a crash, process kill, or lost worker.

## Stalled run detection

A run can define a stall timeout. If a running run has not produced output before the timeout window, the reconciler treats it as stalled.

Current behavior:

- mark the run as `timeout`;
- persist a `run.timeout` event;
- ask the runtime manager to cancel the process/session;
- unlock the task;
- either schedule retry with backoff, block the task when max attempts are exhausted, or schedule the next continuous-task cycle.

Timeouts are counted as attention-worthy activity in the UI because they usually require inspection.

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

This metadata is intentionally duplicated into the activity event so humans and future automation can understand why the retry was scheduled.

## Activity events used for diagnosis

Important robustness events include:

| Event | Meaning |
| --- | --- |
| `run.timeout` | A run stopped making progress and was closed as timed out. |
| `run.recovered` | PicoClip repaired stale/orphaned run state. |
| `retry.scheduled` | PicoClip scheduled a retry and recorded why, when, and with what backoff. |
| `budget.blocked` | Execution was blocked by a budget constraint. |
| `driver.missing` | Required runtime/driver was unavailable. |

The Activity page turns these into human-readable messages. For example, a retry event is presented as PicoClip learning from a timeout and scheduling retry after a specific number of seconds.

## Continuous tasks

Continuous tasks are not retried the same way one-shot tasks are. When a continuous task finishes or is recovered, PicoClip schedules the next cycle according to the task loop delay, unless the task was canceled, completed, or paused.

This keeps recurring work predictable and prevents recovery from turning a continuous loop into a tight retry loop.

## Current limitations

The system is stronger than before, but still experimental. Known gaps:

- Retry classification is still basic. Timeout retries are treated as retryable, but deterministic errors are not yet fully separated into retryable vs non-retryable categories.
- The UI does not yet have a dedicated recovery dashboard for stale locks, retry queue, runtime health, or orphaned runs.
- Runtime liveness is still mostly inferred from output/heartbeat state rather than a complete streaming runtime event model.
- Windows process-tree cancellation still needs Job Object support for parity with Unix process-group cancellation.
- Metrics are visible through events/logs, but aggregate reliability metrics are still limited.

## Operational checklist

When investigating a stuck or failed task:

1. Open the task detail page and inspect the latest run.
2. Open Activity and look for `run.timeout`, `run.recovered`, `retry.scheduled`, `driver.missing`, or `budget.blocked`.
3. Check whether a retry wakeup is pending and whether its `DueAt` is in the future.
4. Confirm whether `MaxAttempts` has been reached.
5. Check runtime configuration and driver availability if the error suggests missing runtime support.
6. Run diagnostics through the diagnostics API/page to inspect storage, runtime path, workspace path, and configured runtime health.

## Developer checklist for robustness changes

When changing recovery, retry, cancellation, scheduling, or dispatcher behavior:

1. Write or update a regression test first.
2. Confirm the test fails for the expected reason.
3. Implement the smallest behavior change.
4. Run the focused package tests.
5. Run `make check` before merging.
6. Confirm new failure paths create clear events or diagnostics.
7. Avoid adding retry behavior without a cap, backoff, and an event explaining the decision.

## Next hardening steps

Recommended next work:

1. Add explicit retry classification: `retryable`, `non_retryable`, and `unknown`.
2. Persist `retry.skipped` or `task.blocked` events when PicoClip intentionally refuses to retry.
3. Expose the retry queue and recovery state in the UI/API.
4. Add aggregate reliability counters: timeouts, recoveries, scheduled retries, skipped retries, and exhausted attempts.
5. Expand runtime events so liveness is based on structured signals, not only output timing.
