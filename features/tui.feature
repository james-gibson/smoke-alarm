# features/tui.feature
# Canon record — last audited: 2026-03-26
# Exercises: Bubble Tea dashboard — Elm Architecture contract, rendering, keyboard nav
# Code: internal/ui/ui.go
# Step definitions: features/step_definitions/tui_steps.go
# see: features/ops.feature (runtime dispatches to runForeground which calls ui.Run)
# see: features/known-state.feature (target states surfaced in status table)
# see: features/alerts.feature (events rendered in events pane)
#
# ARCHITECTURE CONSTRAINTS (from CLAUDE.md):
#   - State mutations ONLY in Update() — never in View() or Init()
#   - No side effects in View() — View is a pure function of model state
#   - Async work via tea.Cmd only — never goroutines launched directly from Update
#   - Two distinct UI concerns: this file covers the Bubble Tea agentic dashboard only
#     (NOT the stdio proxy / MCP message-flow UI which is a separate planned concern)
#
# ELM ARCHITECTURE SUMMARY:
#   Init()   → returns initial tickCmd + loadCmd
#   Update() → handles tickMsg, loadMsg, tea.WindowSizeMsg, tea.KeyMsg
#   View()   → renders full TUI string from current model state (pure, no I/O)
#   Run()    → calls tea.NewProgram(dashboard).Run() — entry point from ops.Runtime
#
# MESSAGE TYPES:
#   tickMsg  — periodic clock tick driving refresh
#   loadMsg  — carries snapshot from SnapshotProvider.Snapshot()
#
# KEYBOARD BINDINGS:
#   q / esc     → quit
#   up / k      → scroll selection up
#   down / j    → scroll selection down
#   tab         → toggle events pane

@tui @optional
Feature: Bubble Tea Dashboard — Elm Architecture and Rendering Contract
  As an operator running ocd-smoke-alarm in foreground mode
  I want a live terminal dashboard that refreshes automatically and responds to keyboard input
  So that I can monitor target health, inspect events, and see topology at a glance

  Background:
    Given the ocd-smoke-alarm binary is installed
    And service.mode is "foreground" in the config

  # ── Init ──────────────────────────────────────────────────────────────────

  Scenario: Init returns a tick command and a load command
    Given a Dashboard model is constructed with NewDashboard
    When Init is called
    Then the returned tea.Cmd contains a tickCmd
    And the returned tea.Cmd contains a loadCmd

  Scenario: Init does not mutate model state
    Given a Dashboard model at initial state
    When Init is called
    Then the model state is unchanged after Init returns

  # ── Update: tickMsg ───────────────────────────────────────────────────────

  Scenario: receiving a tickMsg schedules a new load command
    Given a Dashboard model is running
    When Update receives a tickMsg
    Then the returned tea.Cmd is a loadCmd that will call SnapshotProvider.Snapshot()
    And a new tickCmd is scheduled for the next refresh_interval

  Scenario: tickMsg does not mutate the model's targets or events directly
    Given the model has a known set of target states
    When Update receives a tickMsg
    Then the target states are unchanged until the loadMsg arrives

  # ── Update: loadMsg ───────────────────────────────────────────────────────

  Scenario: receiving a loadMsg replaces the model's snapshot with the new one
    Given a Dashboard model with a stale snapshot
    When Update receives a loadMsg containing a fresher snapshot
    Then the model's snapshot is replaced with the loadMsg contents
    And the model's last_updated time is updated

  Scenario: a loadMsg with no targets results in an empty status table
    When Update receives a loadMsg with 0 targets
    Then the model's target list is empty
    And View renders the status table section with no rows

  # ── Update: keyboard input ────────────────────────────────────────────────

  Scenario: pressing "q" triggers a quit command
    When Update receives a tea.KeyMsg for key "q"
    Then the returned tea.Cmd is tea.Quit

  Scenario: pressing "esc" triggers a quit command
    When Update receives a tea.KeyMsg for key "esc"
    Then the returned tea.Cmd is tea.Quit

  Scenario: pressing "down" or "j" increments the selected row index
    Given the model has 3 targets and selected index 0
    When Update receives a tea.KeyMsg for key "down"
    Then the selected index becomes 1

  Scenario: pressing "up" or "k" decrements the selected row index
    Given the model has 3 targets and selected index 2
    When Update receives a tea.KeyMsg for key "up"
    Then the selected index becomes 1

  Scenario: selection does not go below 0
    Given the model has 1 target and selected index 0
    When Update receives a tea.KeyMsg for key "up"
    Then the selected index remains 0

  Scenario: selection does not exceed the last target index
    Given the model has 2 targets and selected index 1
    When Update receives a tea.KeyMsg for key "down"
    Then the selected index remains 1

  Scenario: pressing "tab" toggles the events pane visibility
    Given the events pane is hidden
    When Update receives a tea.KeyMsg for key "tab"
    Then the events pane becomes visible

  Scenario: pressing "tab" again hides the events pane
    Given the events pane is visible
    When Update receives a tea.KeyMsg for key "tab"
    Then the events pane becomes hidden

  # ── Update: window resize ─────────────────────────────────────────────────

  Scenario: receiving a tea.WindowSizeMsg updates the model's width and height
    Given the current terminal width is 80 and height is 24
    When Update receives a tea.WindowSizeMsg with width 120 and height 40
    Then the model width is 120
    And the model height is 40

  Scenario: View adapts its layout after a window resize
    Given the terminal is resized to width 120
    When View is called
    Then the rendered output width does not exceed 120 characters per line

  # ── View: contract ────────────────────────────────────────────────────────

  Scenario: View is a pure function of model state
    Given two Dashboard models with identical state
    When View is called on each
    Then both return identical output strings

  Scenario: View does not perform I/O or launch goroutines
    Given a Dashboard model with populated state
    When View is called
    Then no file reads, network calls, or goroutine launches occur during View

  # ── View: status table ────────────────────────────────────────────────────

  Scenario: the status table renders one row per target in the snapshot
    Given the model snapshot contains 3 targets
    When View is called
    Then the status table section contains 3 data rows

  Scenario: each status row displays target id, state, severity, failure count, and latency
    Given a snapshot with target id "mcp-primary", state "degraded", and latency 250ms
    When View is called
    Then the status table row for that target contains the id, "degraded", and "250ms"

  Scenario: the selected target row is visually distinguished from non-selected rows
    Given the model has 3 targets and selected index 1
    When View is called
    Then the second row has a different visual style than the first and third rows

  # ── View: selected details pane ───────────────────────────────────────────

  Scenario: the details pane renders the selected target's full status information
    Given the model has target "mcp-primary" selected
    When View is called
    Then the details pane contains the target's endpoint, protocol, and transport
    And the details pane contains the target's last-checked timestamp

  Scenario: the details pane is empty when no target is selected
    Given the model has 0 targets
    When View is called
    Then the details pane renders as empty or with a placeholder message

  # ── View: events pane ─────────────────────────────────────────────────────

  Scenario: the events pane renders recent alert events when visible
    Given the events pane is visible
    And the model snapshot contains 3 events
    When View is called
    Then the events section lists all 3 events with their timestamp and message

  Scenario: the events pane is absent from the rendered output when hidden
    Given the events pane is hidden
    When View is called
    Then the rendered output does not contain an events section

  Scenario: the events pane truncates to the max_events limit
    Given Options.MaxEvents is 5
    And the snapshot contains 10 events
    When View is called
    Then only 5 events are rendered in the events pane

  # ── View: topology pane ───────────────────────────────────────────────────

  Scenario: the topology pane aggregates protocol and transport counts from the snapshot
    Given the snapshot contains 2 MCP targets and 1 ACP target
    When View is called
    Then the topology pane shows "MCP: 2" and "ACP: 1"

  # ── View: demo mode state machine ─────────────────────────────────────────

  Scenario: the demo state machine is rendered when Options.Demo is true
    Given a Dashboard constructed with Options.Demo true
    When View is called
    Then the rendered output contains an ASCII state machine diagram

  Scenario: the demo state machine is absent when Options.Demo is false
    Given a Dashboard constructed with Options.Demo false
    When View is called
    Then the rendered output does not contain a state machine diagram

  # ── SnapshotProvider interface ────────────────────────────────────────────

  Scenario: the Dashboard accepts any SnapshotProvider implementation
    Given a mock SnapshotProvider that returns a fixed snapshot
    When the Dashboard is constructed with that provider
    And a loadMsg cycle runs
    Then the model snapshot matches the mock provider's output

  Scenario: a nil snapshot from the provider leaves the model state unchanged
    Given a SnapshotProvider that returns nil
    When Update receives the resulting loadMsg
    Then the previous model snapshot is retained
    And no panic occurs

  # ── Run ───────────────────────────────────────────────────────────────────

  Scenario: Run blocks until the user presses q or esc
    Given a Dashboard is started via Run
    When the user presses "q"
    Then Run returns without error

  Scenario: Run returns an error when the terminal is unavailable
    Given no TTY is available
    When Run is called
    Then Run returns a non-nil error
    And no panic occurs
