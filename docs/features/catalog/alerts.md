---
title: Alert Routing and Severity Policy
description: LogNotifier, DesktopNotifier, dedupe and severity filtering, secret redaction, NotifierGroup fan-out, and config-level per-state severity policy.
tags:
  - smoke-alarm
  - feature
  - ops-safety
  - alerts
  - notifications
  - gherkin
source: smoke-alarm/features/alerts.feature
subsystem: ops-safety
gherkin_tags:
  - "@alerts"
  - "@core"
scenario_count: 20
---
> **Source:** `features/alerts.feature` · **Subsystem:** [Operations & Safety](../subsystems/ops-safety.md) · **Index:** [Feature catalog](../index.md)

Alert events are routed to the correct sinks with severity filtering and deduplication, so operators receive actionable notifications without noise or secret leakage. The `alerts` config block appears in all sample configs and carries `aggressive`, `notify_on_regression_immediately`, `retry_before_escalation`, `dedupe_window`, `cooldown`, per-state `severity`, `sinks.log`, and `sinks.os_notification`.

## Scenario map

**LogNotifier** — logs `degraded`/`warn` at warn (message contains `target_id` and `state`), `regression`/`critical` at error; suppresses events below `min_severity`; deduplicates identical events within `dedupe_window` (3 events in 1 minute → 1 entry) and emits again after the window expires.

**Alert message sanitization** — `Bearer <token>` → `Bearer ****`; `access_token=...` → `access_token_redacted`; `refresh_token=...` → `refresh_token_redacted` (raw values never logged).

**DesktopNotifier** — calls `osascript` on macOS (title contains "CRITICAL") and `notify-send` on Linux (urgency "critical"); appends "REGRESSION" when `regression` is true; shares the dedupe logic (2 events in 30 s → one invocation); returns `ErrNotifierUnsupported` on unsupported platforms ("desktop notifier unsupported on this platform"); returns `ErrNotifierUnavailable` when `osascript` is missing ("osascript not found").

**Severity rank ordering** — events at or above `min_severity` dispatch; events below are suppressed (table-driven examples across info/warn/critical).

**NotifierGroup fan-out** — delivers to all configured notifiers; continues delivery when one errors, and the combined error includes the failing notifier's error.

**Config-level alert policy** — per-state severity comes from `alerts.severity` (`healthy`→info, `degraded`→warn, `regression`→critical, `outage`→critical); `notify_on_regression_immediately` bypasses `retry_before_escalation`, emitting on the first regression result.

## Cross-references

- [Known-State Regression Detection](known-state.md) — the regression flag on `AlertEvent`
- [OAuth Mock Redirect Endpoint](oauth-mock.md) — token redaction in alert messages
- [Probe Engine](engine.md) — alert emission on regression/outage
- [Operations & Safety subsystem](../subsystems/ops-safety.md)