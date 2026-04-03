# features/alerts.feature
# Canon record — last audited: 2026-03-25
# Exercises: alerts config block, LogNotifier, DesktopNotifier, dedupe, severity policy
# Code: internal/alerts/notifier.go
# Step definitions: features/step_definitions/alerts_steps.go
# see: features/known-state.feature (regression flag on AlertEvent)
# see: features/oauth-mock.feature (token redaction in alert messages)
#
# The alerts block is present in all sample configs. Key fields:
#   aggressive, notify_on_regression_immediately, retry_before_escalation,
#   dedupe_window, cooldown, severity (per-state), sinks.log, sinks.os_notification

@alerts @core
Feature: Alert Routing and Severity Policy
  As an operator
  I want alert events to be routed to the correct sinks with severity filtering and deduplication
  So that I receive actionable notifications without noise or secret leakage

  Background:
    Given the ocd-smoke-alarm binary is installed

  # ── LogNotifier ───────────────────────────────────────────────────────────

  Scenario: a DEGRADED event is logged at warn level
    Given a LogNotifier with min_severity "info"
    When an alert event is emitted with state "degraded" and severity "warn"
    Then the log output contains a "warn" level entry
    And the log entry contains "target_id"
    And the log entry contains "state"

  Scenario: a REGRESSION alert is logged at error level
    Given a LogNotifier with min_severity "info"
    When an alert event is emitted with regression true and severity "critical"
    Then the log output contains an "error" level entry

  Scenario: an event below the minimum severity threshold is not logged
    Given a LogNotifier with min_severity "warn"
    When an alert event is emitted with severity "info"
    Then no log entry is written

  Scenario: log notifier deduplicates identical events within the dedupe window
    Given a LogNotifier with dedupe_window "2m"
    When 3 identical alert events are emitted within 1 minute
    Then only 1 log entry is written

  Scenario: log notifier emits repeated events after the dedupe window expires
    Given a LogNotifier with dedupe_window "2m"
    And an alert event was last emitted 3 minutes ago
    When the same alert event is emitted again
    Then a new log entry is written

  # ── alert message sanitization ────────────────────────────────────────────

  Scenario: "Bearer " prefix in alert message is redacted
    Given an alert event with message "token: Bearer abc123xyz"
    When the LogNotifier processes the event
    Then the log entry message contains "Bearer ****"
    And the log entry message does not contain "abc123xyz"

  Scenario: "access_token" in alert message is redacted
    Given an alert event with message "access_token=secret123"
    When the LogNotifier processes the event
    Then the log entry message contains "access_token_redacted"
    And the log entry message does not contain "secret123"

  Scenario: "refresh_token" in alert message is redacted
    Given an alert event with message "refresh_token=refreshsecret"
    When the LogNotifier processes the event
    Then the log entry message contains "refresh_token_redacted"

  # ── DesktopNotifier ───────────────────────────────────────────────────────

  Scenario: DesktopNotifier calls osascript on macOS for a critical alert
    Given a DesktopNotifier with os_notification enabled
    And the platform is "darwin"
    And "osascript" is available on PATH
    When an alert event is emitted with severity "critical"
    Then osascript is invoked with a display notification command
    And the notification title contains "CRITICAL"

  Scenario: DesktopNotifier calls notify-send on Linux for a critical alert
    Given a DesktopNotifier with os_notification enabled
    And the platform is "linux"
    And "notify-send" is available on PATH
    When an alert event is emitted with severity "critical"
    Then notify-send is invoked with urgency "critical"

  Scenario: DesktopNotifier appends "REGRESSION" to the title when regression is true
    Given a DesktopNotifier with os_notification enabled
    And the platform is "darwin"
    When an alert event is emitted with regression true
    Then the notification title contains "REGRESSION"

  Scenario: DesktopNotifier deduplications follow the same dedupe_window logic as LogNotifier
    Given a DesktopNotifier with dedupe_window "1m"
    When 2 identical alert events are emitted within 30 seconds
    Then osascript is invoked exactly once

  Scenario: DesktopNotifier returns ErrNotifierUnsupported on unsupported platforms
    Given a DesktopNotifier with os_notification enabled
    And the platform is "windows"
    When an alert event is emitted
    Then the error is "desktop notifier unsupported on this platform"

  Scenario: DesktopNotifier returns ErrNotifierUnavailable when osascript is not on PATH
    Given a DesktopNotifier with os_notification enabled
    And the platform is "darwin"
    And "osascript" is NOT available on PATH
    When an alert event is emitted
    Then the error contains "osascript not found"

  # ── severity rank ordering ────────────────────────────────────────────────

  Scenario Outline: severity filtering allows events at or above the minimum
    Given a notifier with min_severity "<min-severity>"
    When an alert event is emitted with severity "<event-severity>"
    Then the event is dispatched

    Examples:
      | min-severity | event-severity |
      | info         | info           |
      | info         | warn           |
      | info         | critical       |
      | warn         | warn           |
      | warn         | critical       |
      | critical     | critical       |

  Scenario Outline: severity filtering suppresses events below the minimum
    Given a notifier with min_severity "<min-severity>"
    When an alert event is emitted with severity "<event-severity>"
    Then the event is suppressed

    Examples:
      | min-severity | event-severity |
      | warn         | info           |
      | critical     | warn           |
      | critical     | info           |

  # ── NotifierGroup fan-out ─────────────────────────────────────────────────

  Scenario: NotifierGroup delivers to all configured notifiers
    Given a NotifierGroup with a LogNotifier and a DesktopNotifier
    When an alert event is emitted
    Then both notifiers receive the event

  Scenario: NotifierGroup continues delivery when one notifier errors
    Given a NotifierGroup with a failing notifier and a healthy LogNotifier
    When an alert event is emitted
    Then the healthy LogNotifier still receives and logs the event
    And the combined error includes the failing notifier's error

  # ── config-level alert policy ─────────────────────────────────────────────

  Scenario Outline: per-state severity is taken from the alerts.severity config block
    Given the config file "<config-path>" has alerts.severity."<state>" set to "<configured-severity>"
    When a probe result produces state "<state>"
    Then the alert event severity is "<expected-severity>"

    Examples:
      | config-path             | state      | configured-severity | expected-severity |
      | configs/sample.yaml     | healthy    | info                | info              |
      | configs/sample.yaml     | degraded   | warn                | warn              |
      | configs/sample.yaml     | regression | critical            | critical          |
      | configs/sample.yaml     | outage     | critical            | critical          |

  Scenario: notify_on_regression_immediately bypasses retry_before_escalation
    Given "notify_on_regression_immediately" is true in config "configs/sample.yaml"
    And "retry_before_escalation" is 1
    When the first probe result for a regression is received
    Then an alert is emitted immediately without waiting for a retry
