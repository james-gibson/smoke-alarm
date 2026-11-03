---
title: Ops — Runtime Lifecycle and Update Orchestration
description: Runtime construction, lock file, mode dispatch, bootstrap, health sync, graceful shutdown, and the LifecycleController lock→stop→deploy→restart→verify→commit workflow with rollback and journal.
tags:
  - smoke-alarm
  - feature
  - ops-safety
  - ops
  - lifecycle
  - gherkin
source: smoke-alarm/features/ops.feature
subsystem: ops-safety
gherkin_tags:
  - "@ops"
  - "@optional"
scenario_count: 24
---
> **Source:** `features/ops.feature` · **Subsystem:** [Operations & Safety](../subsystems/ops-safety.md) · **Index:** [Feature catalog](../index.md)

The ops subsystem (`internal/ops/runtime.go`, `internal/ops/update.go`) enforces a safe, auditable lifecycle. **Runtime** orchestrates foreground/background mode dispatch and bootstraps the engine, health server, alerts, and discovery. **LifecycleController** orchestrates the lock → stop → deploy → restart → verify → commit sequence with rollback on failure. Every phase transition writes a `JournalEntry` (step, outcome, timestamp) persisted before the lock is released.

## Scenario map

**Runtime construction** — `NewRuntime` is built from a validated config with its engine/health/alert settings; the logger uses the config `log_level`.

**Lock file** — `Run` creates the lock (with the current PID) before the engine starts; fails fast with a "lock" error when another process holds it; removes the lock on clean shutdown.

**Mode dispatch** — `foreground` starts the Bubble Tea dashboard and blocks until it quits; `background` starts no dashboard and blocks until context cancel.

**Bootstrap** — initializes the engine and binds the health server to `listen_addr`, wires the notifier to the engine event channel, and starts discovery when enabled (not when disabled).

**Health sync loop** — writes the engine snapshot to the health server at least twice over 2 seconds.

**Graceful shutdown** — all subsystems stop within `graceful_shutdown_timeout`; no goroutine remains after the timeout.

**LifecycleController** — constructed from a `Plan` (stop/start/verify commands).

**Execute workflow (happy path)** — journal order is lock → stop → deploy → restart → verify → commit, with `UpdateResult.Success` true; the lock is acquired before `stop_command` (when `require_lock`) and released after commit; skipped when `require_lock` is false.

**Health verification polling** — polls `/healthz` after restart until 200, running `verify_command` only after healthy; a timeout within `max_wait_for_healthy` marks the update failed ("health check timeout").

**Rollback** — a failing verify triggers rollback (journal "rollback", `RolledBack` true) when `rollback_on_failure` is true; no rollback when false (`Success` false); rollback runs `stop_command` then `start_command` to restore the previous version.

**Journal** — each entry has step, outcome (`ok`/`failed`), a valid RFC3339 timestamp, and a positive duration; the journal is persisted before the lock is released.

**Version tracking** — `UpdateResult` carries `PreviousVersion` and `NewVersion`.

**Command execution** — `stop_command` runs via the `CommandRunner` with the configured environment and its exit code is captured; a missing `stop_command` is a no-op journaled "skipped"; a missing `verify_command` skips verify (journaled "skipped") but health polling still occurs if a `control_endpoint` is configured.

## Cross-references

- [Federation Mesh](federation.md) — slot election and instance identity
- [Known-State Regression Detection](known-state.md) — health state consumed by verify
- [Hosted Server](hosted-server.md) — `/healthz` / `/readyz` used in verification
- [Operations & Safety subsystem](../subsystems/ops-safety.md)