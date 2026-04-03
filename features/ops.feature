# features/ops.feature
# Canon record — last audited: 2026-03-26
# Exercises: ops subsystem — runtime lifecycle, update/rollback orchestration
# Code: internal/ops/runtime.go, internal/ops/update.go
# Step definitions: features/step_definitions/ops_steps.go
# see: features/federation.feature (slot election, instance identity)
# see: features/known-state.feature (health state consumed by verify step)
# see: features/hosted-server.feature (/healthz, /readyz used in health verification)
#
# ARCHITECTURE NOTE:
#   Runtime orchestrates foreground/background mode dispatch and bootstraps engine,
#   health server, alerts, and discovery. LifecycleController orchestrates the
#   lock → stop → deploy → restart → verify → commit sequence with rollback on failure.
#
# UPDATE WORKFLOW SEQUENCE:
#   1. Acquire lock (require_lock=true by default)
#   2. Execute stop_command
#   3. Execute start_command (deploy step)
#   4. Poll /healthz until healthy or max_wait_for_healthy elapses
#   5. Execute verify_command
#   6. Commit (clear lock); on any failure after stop → rollback
#
# JOURNAL:
#   Every phase transition writes a JournalEntry with step name, outcome, and timestamp.
#   Journal is persisted before the lock is released.

@ops @optional
Feature: Ops — Runtime Lifecycle and Update Orchestration
  As an operator or remote agent managing ocd-smoke-alarm instances
  I want the runtime to enforce a safe, auditable lifecycle for foreground/background modes
  and a lock-gated update workflow with automatic rollback
  So that concurrent instances are prevented, updates are safe, and failures are recoverable

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/sample.yaml" exists

  # ── runtime construction ───────────────────────────────────────────────────

  Scenario: NewRuntime returns a Runtime from a validated config
    Given a validated config loaded from "configs/sample.yaml"
    When NewRuntime is called
    Then the Runtime is initialized with the config's engine, health, and alert settings

  Scenario: NewRuntime sets the log level from config
    Given a config with log_level "debug"
    When NewRuntime is called
    Then the runtime logger uses that log level

  # ── lock file ─────────────────────────────────────────────────────────────

  Scenario: Run acquires the lock file before starting
    Given a lock_file path configured as "/tmp/ocd-smoke-alarm.lock"
    When Run is called
    Then the lock file is created before the engine starts
    And the lock file contains the current process PID

  Scenario: Run fails fast when another process holds the lock
    Given a lock file at "/tmp/ocd-smoke-alarm.lock" exists containing an active PID
    When Run is called
    Then the error message contains "lock"
    And the runtime exits without starting the engine

  Scenario: Run removes the lock file on clean shutdown
    Given the runtime is running and holds the lock
    When the context is cancelled
    Then the lock file is removed after shutdown completes

  # ── mode dispatch ─────────────────────────────────────────────────────────

  Scenario: Run dispatches to foreground mode when service.mode is "foreground"
    Given a config with service.mode "foreground"
    When Run is called
    Then the Bubble Tea dashboard is started
    And the process blocks until the dashboard quits

  Scenario: Run dispatches to background mode when service.mode is "background"
    Given a config with service.mode "background"
    When Run is called
    Then no Bubble Tea dashboard is started
    And the process blocks until the context is cancelled

  # ── bootstrap ─────────────────────────────────────────────────────────────

  Scenario: bootstrap initialises the engine, health server, notifier, and discovery
    Given a valid config with health.enabled true
    When bootstrap is called
    Then the engine is running
    And the health server is bound to the configured listen_addr
    And the notifier is wired to the engine's event channel
    And the discovery subsystem is running if discovery.enabled is true

  Scenario: bootstrap does not start discovery when discovery.enabled is false
    Given a config with discovery.enabled false
    When bootstrap is called
    Then the discovery subsystem is not started

  # ── health sync loop ──────────────────────────────────────────────────────

  Scenario: syncHealthLoop writes the current engine snapshot to the health server every second
    Given the runtime is running
    When 2 seconds elapse
    Then the health server snapshot is updated at least twice
    And the snapshot reflects the engine's most recent target states

  # ── graceful shutdown ─────────────────────────────────────────────────────

  Scenario: runtime shuts down within the configured graceful_shutdown_timeout
    Given runtime.graceful_shutdown_timeout is set to "5s"
    When the context is cancelled
    Then all subsystems stop within that timeout
    And no goroutine is left running after the timeout

  # ── LifecycleController construction ──────────────────────────────────────

  Scenario: LifecycleController is constructed from a Plan
    Given a Plan with stop_command "systemctl stop ocd", start_command "systemctl start ocd", and verify_command "curl /healthz"
    When NewLifecycleController is called with that Plan
    Then the controller holds the plan configuration

  # ── Execute workflow: happy path ──────────────────────────────────────────

  Scenario: Execute runs lock → stop → deploy → restart → verify → commit in order
    Given a LifecycleController with valid stop, start, and verify commands
    When Execute is called
    Then the journal records a step entry for "lock"
    And then a step entry for "stop"
    And then a step entry for "deploy"
    And then a step entry for "restart"
    And then a step entry for "verify"
    And then a step entry for "commit"
    And the UpdateResult.Success is true

  Scenario: Execute acquires the ops lock before running stop_command
    Given remote_agent.safety.require_lock is true
    When Execute is called
    Then the lock is held before stop_command runs
    And the lock is released after commit

  Scenario: Execute skips lock acquisition when require_lock is false
    Given remote_agent.safety.require_lock is false
    When Execute is called
    Then no lock file is written
    And the workflow proceeds directly to stop

  # ── health verification polling ───────────────────────────────────────────

  Scenario: Execute polls /healthz after restart until the instance reports healthy
    Given the restart command succeeds
    And the instance becomes healthy after 2 poll attempts
    When Execute reaches the verify step
    Then /healthz is polled until a 200 response is received
    And the verify_command runs only after /healthz returns healthy

  Scenario: Execute marks the update as failed when /healthz does not return healthy within max_wait_for_healthy
    Given remote_agent.update.max_wait_for_healthy is "30s"
    And the instance never becomes healthy
    When Execute runs the verify step
    Then the UpdateResult.Success is false
    And the failure reason contains "health check timeout"

  # ── rollback ──────────────────────────────────────────────────────────────

  Scenario: Execute triggers rollback when the verify step fails
    Given the verify_command exits with a non-zero status
    And remote_agent.update.rollback_on_failure is true
    When Execute runs
    Then the journal contains a "rollback" step entry
    And the UpdateResult.RolledBack is true

  Scenario: Execute does not roll back when rollback_on_failure is false
    Given the verify_command fails
    And remote_agent.update.rollback_on_failure is false
    When Execute runs
    Then no rollback step is recorded in the journal
    And the UpdateResult.Success is false

  Scenario: rollback runs the stop_command then start_command to restore the previous version
    Given the previous version was running before the update began
    When rollback executes
    Then the stop_command is run to stop the failed version
    And the start_command is run to restore the previous version

  # ── journal ───────────────────────────────────────────────────────────────

  Scenario: each journal entry contains step name, outcome, timestamp, and duration
    When Execute completes
    Then each JournalEntry has a non-empty step field
    And each JournalEntry has an outcome of "ok" or "failed"
    And each JournalEntry has a valid RFC3339 timestamp
    And each JournalEntry has a positive duration

  Scenario: journal is persisted before the ops lock is released
    When Execute completes
    Then the journal file exists on disk
    And the lock is still held at the moment the journal is written

  # ── version tracking ──────────────────────────────────────────────────────

  Scenario: UpdateResult contains the previous and new version strings
    Given the running instance reports version "1.2.3" via /status
    And the update deploys version "1.2.4"
    When Execute completes successfully
    Then the UpdateResult.PreviousVersion is the first version
    And the UpdateResult.NewVersion is the second version

  # ── command execution ─────────────────────────────────────────────────────

  Scenario: stop_command is executed via the shell with the configured environment
    Given a stop_command "systemctl stop ocd-smoke-alarm"
    When the stop phase runs
    Then the command is executed by the CommandRunner
    And the exit code is captured in the journal

  Scenario: a missing stop_command is treated as a no-op
    Given a Plan with an empty stop_command
    When Execute runs the stop phase
    Then no command is executed
    And the journal records stop as "skipped"

  Scenario: a missing verify_command skips the verify phase
    Given a Plan with an empty verify_command
    When Execute runs the verify phase
    Then no command is executed
    And the journal records verify as "skipped"
    And health polling still occurs if a control_endpoint is configured
