# AGENT_RUNBOOK.md

## Purpose

This runbook describes how a remote agent manages `ocd-smoke-alarm` with **zero operator intervention**:

1. Stop active runtime safely
2. Update deployment to latest approved version
3. Restart service
4. Verify health and regression checks
5. Roll back automatically on failure

This document is optimized for unattended operations on Linux and macOS.

---

## Scope and Assumptions

- The service binary is `ocd-smoke-alarm`.
- Sample config files exists (default: `configs/sample.yaml` or `configs/samples/*.yml` or environment-provided path).
- Agent has permissions to:
  - stop/start process
  - replace binary/artifacts
  - read/write runtime metadata files
  - query health endpoints
- The service can run in:
  - `background` mode (daemon-like)
  - `foreground` mode (Bubble Tea TUI; not used for unattended updates)
- Updates are **agent-controlled only**.

---

## Operational Contract (Must Hold)

The agent MUST follow this contract for every change:

1. Acquire update lock
2. Preflight checks
3. Graceful stop
4. Deploy new artifacts
5. Restart
6. Post-start health verification
7. Regression verification
8. Commit update marker OR rollback

If any required step fails, the agent must transition to rollback workflow.

---

## Files and Runtime Metadata

Recommended paths (customizable):

- Binary:
  - `./bin/ocd-smoke-alarm`
- Config:
  - `./configs/samples/mcp-add-meta-20260408-045454.yaml`
- PID file:
  - `./run/ocd-smoke-alarm.agent-runbook.pid`
- Lock file:
  - `./run/ocd-smoke-alarm.update.lock`
- Active version marker:
  - `./run/version.current`
- Previous version marker:
  - `./run/version.previous`
- Update journal:
  - `./run/update.journal.log`

Lock file must be exclusive; concurrent updates are forbidden.

---

## Health and Readiness Interfaces

Expected endpoints/commands:

- Liveness: `GET /healthz` (process up)
- Readiness: `GET /readyz` (dependencies and monitor loop ready)
- Optional details: `GET /status` (target states, regressions, auth classifications)

Success criteria:

- `/healthz` -> HTTP 200 within startup timeout
- `/readyz` -> HTTP 200 within convergence timeout
- `/status` -> no critical startup faults

---

## Unattended Update Workflow (Primary)

## Phase 1: Acquire Lock and Preflight

1. Acquire lock (`run/ocd-smoke-alarm.update.lock`).
2. Validate update payload integrity (checksum/signature if enabled).
3. Verify current runtime:
   - If running, capture PID and current version marker.
4. Validate config syntax against current schema.
5. Run quick local self-check (if supported): `ocd-smoke-alarm check --config ...`.

Abort and release lock if preflight fails.

## Phase 2: Graceful Stop

1. Send termination signal to active PID.
2. Wait graceful timeout (e.g., 10–30s).
3. If still running, force stop according to policy.
4. Confirm process exit and PID file cleanup.

If stop fails, abort and release lock (do not deploy over active process).

## Phase 3: Deploy

1. Move current binary to rollback slot (or preserve version pointer).
2. Place new binary/artifacts.
3. Update executable permissions.
4. Write pending update journal entry:
   - target version
   - timestamp
   - prior version
   - config hash

## Phase 4: Restart

1. Start service in background mode:
   - `ocd-smoke-alarm serve --mode=background --config <path> --health-addr <addr>`
2. Write new PID file.
3. Start convergence timer.

## Phase 5: Verify

1. Poll `/healthz` until success or timeout.
2. Poll `/readyz` until success or timeout.
3. Query `/status` and check:
   - scheduler active
   - no immediate fatal auth/config errors
4. Run targeted smoke checks (integration-equivalent probes where possible).

If verification fails, go to rollback.

## Phase 6: Commit

1. Write `version.previous` = old version.
2. Write `version.current` = new version.
3. Append success entry to update journal.
4. Release lock.

---

## Rollback Workflow (Mandatory)

Trigger rollback on any failure in deploy/restart/verify:

1. Stop failed new runtime.
2. Restore previous binary/version marker.
3. Restart previous version.
4. Re-run `/healthz` and `/readyz`.
5. Append rollback event to journal with root-cause category:
   - config_invalid
   - startup_failure
   - readiness_timeout
   - regression_spike
   - auth_bootstrap_failure
6. Release lock.
7. Emit critical alert/event.

If rollback restart fails, escalate as `CRITICAL_UNRECOVERABLE` and keep detailed journal for operator follow-up.

---

## Regression and Alert Policy During Updates

Because this monitor is intentionally aggressive:

- Previously healthy target failing post-update is a **regression**.
- Regressions are high-severity and should be elevated quickly.
- During a controlled rollout window, use short warmup tolerance only if configured.
- Never suppress:
  - startup failures
  - auth bootstrap failures
  - total outage of previously healthy services

---

## Security and Secret Handling

- Prefer OS keystore/keychain for secret retrieval.
- Never write raw secrets to logs, status, or journal.
- Redact token-like fields in all structured logs.
- Validate ownership/permissions of runtime directories (`run/`, `bin/`, config files).
- Refuse update if artifact integrity checks fail.

---

## Integration Test Requirements Before/After Update Logic Changes

Any agent modifying lifecycle/update logic must run integration tests covering:

1. Healthy target remains healthy
2. Healthy -> failure emits regression
3. Auth failure classification correctness
4. Aggressive outage escalation behavior
5. Stop-update-restart-recover cycle
6. Rollback on bad deployment
7. Health endpoint stability across restart

If tests fail, do not commit update logic changes.

---

## Failure Taxonomy (Use in Journals and Alerts)

Use standardized categories:

- `LOCK_CONTENTION`
- `PREFLIGHT_CONFIG_INVALID`
- `PREFLIGHT_INTEGRITY_FAILED`
- `STOP_TIMEOUT`
- `DEPLOY_COPY_FAILED`
- `START_FAILED`
- `HEALTH_TIMEOUT`
- `READINESS_TIMEOUT`
- `REGRESSION_DETECTED`
- `ROLLBACK_FAILED`

This keeps automated triage deterministic.

---

## Agent Change Protocol (Token-Constrained Friendly)

When making targeted edits:

1. Change one subsystem per run (e.g., only `internal/ops` + its tests).
2. Prefer additive changes; avoid broad refactors.
3. Update relevant integration test in same run.
4. Leave compact handoff note:

- `DONE:`
- `NEXT:`
- `RISKS:`
- `OPEN_QUESTIONS:`

5. If uncertain, add instrumentation first, then behavior change.

---

## Recommended Cadence for Unattended Deployments

- Staged updates over fixed windows (avoid continuous blind rollout).
- Post-deploy verification window (e.g., 2–5 minutes) before considering successful.
- Automatic rollback immediately on critical readiness/regression failures.
- Keep last known good artifact locally available for fast recovery.

---

## Minimal Operator Recovery Guide (Break-Glass)

Only if unattended recovery fails:

1. Inspect `run/update.journal.log`
2. Validate `version.current` and `version.previous`
3. Start previous binary manually with last known good config
4. Verify `/healthz` and `/readyz`
5. Resume normal agent-managed workflow

This path should be rare and only used for unrecoverable failures.

---

## Versioning and Compatibility

- Keep config schema versioned.
- Agents must perform schema compatibility checks during preflight.
- Do not auto-migrate destructive config changes without explicit migration logic.
- Prefer backward-compatible additions.

---

## Exit Criteria for “Safe Unattended Operation”

This runbook is considered correctly implemented when:

- Agent can perform stop-update-restart with no manual steps.
- Failed deployments are rolled back automatically.
- Health and readiness checks gate success.
- Regression detection remains active and high-signal.
- Integration lifecycle tests pass consistently on Linux/macOS.
