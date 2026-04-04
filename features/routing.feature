# features/routing.feature
# Canon record — last audited: 2026-04-03
# Exercises: internal/routing — priority routing table, MCP/ACP path selection, self-healing
# Source: internal/routing/table.go
# Step definitions: features/step_definitions/routing_steps.go
# see: features/mdns-discovery.feature (peer discovery that populates this table)
# see: features/federation.feature (fan-out topology that sits above the routing layer)
#
# ARCHITECTURE CONSTRAINTS:
#   - Routing table admission requires a passing health-check (state != OUTAGE)
#   - Eviction is immediate on health-check failure or mDNS deregistration
#   - MCP and ACP maintain independent priority-ordered path lists
#   - An OUTAGE instance is never present in the active routing paths
#   - An empty routing table holds traffic and fires an alert — it never routes silently to dead instances
#   - A failure carrying a pre-registered chaos isotope within its declared window is not a real failure
#   - A real failure cannot produce a pre-registered chaos isotope — the isotope is the proof

@routing @core
Feature: MCP/ACP Priority Routing and Self-Healing
  As an ocd-smoke-alarm cluster
  I want to route MCP and ACP traffic through priority-ordered healthy paths
  So that traffic shifts automatically when instances fail and reclaims when they recover

  Background:
    Given the ocd-smoke-alarm binary is installed
    And the routing table has been populated from mDNS peer discovery

  # ── admission ────────────────────────────────────────────────────────────────

  Scenario: a discovered peer is admitted after its first health-check passes
    Given a peer "smoke-alarm-new" has been discovered via mDNS
    And the peer's health state is "unknown"
    When the first health-check probe returns HEALTHY
    Then "smoke-alarm-new" is admitted to the active routing paths
    And its state is "healthy"

  Scenario: a discovered peer is not admitted if its first health-check fails
    Given a peer "smoke-alarm-new" has been discovered via mDNS
    When the first health-check probe returns OUTAGE
    Then "smoke-alarm-new" is not added to the active routing paths
    And it remains in the routing table with state "outage"

  Scenario: routing table active paths never contain an OUTAGE instance
    Given a routing table with peers at priorities 1, 2, and 3
    When the peer at priority 2 transitions to OUTAGE
    Then the active routing paths contain only the peers at priorities 1 and 3

  # ── MCP routing ──────────────────────────────────────────────────────────────

  Scenario: MCP traffic routes to the highest-priority healthy instance
    Given the routing table has peers:
      | name      | mcp-priority | state   |
      | primary   | 1            | healthy |
      | secondary | 2            | healthy |
      | tertiary  | 3            | healthy |
    When an MCP request arrives
    Then it is routed to "primary"

  Scenario: MCP traffic fails over to next priority when primary is OUTAGE
    Given "primary" (mcp-priority=1) transitions to OUTAGE
    And "secondary" (mcp-priority=2) is healthy
    When an MCP request arrives
    Then it is routed to "secondary"

  Scenario: MCP traffic continues down the priority list through multiple failures
    Given "primary" (mcp-priority=1) and "secondary" (mcp-priority=2) are both OUTAGE
    And "tertiary" (mcp-priority=3) is healthy
    When an MCP request arrives
    Then it is routed to "tertiary"

  Scenario: MCP routing holds and alerts when all instances are OUTAGE
    Given all peers in the routing table are OUTAGE
    When an MCP request arrives
    Then the request is held and not routed to any instance
    And an operator alert fires with reason "no healthy MCP paths"

  # ── ACP routing ──────────────────────────────────────────────────────────────

  Scenario: ACP traffic routes to the highest-priority healthy instance
    Given the routing table has peers:
      | name      | acp-priority | state   |
      | primary   | 1            | healthy |
      | secondary | 2            | healthy |
    When an ACP request arrives
    Then it is routed to "primary"

  Scenario: ACP traffic fails over independently of MCP
    Given "primary" has acp-priority=1 and mcp-priority=1
    And "primary" transitions to OUTAGE for ACP only
    When an ACP request arrives
    Then it is routed to "secondary"
    And an MCP request in the same moment still routes to "primary"

  # ── shared priority fallback ──────────────────────────────────────────────────

  Scenario: shared priority field is used when no traffic-specific priority is declared
    Given a peer with TXT record "priority=2" and no "mcp-priority" or "acp-priority" fields
    Then the peer's MCP priority is 2
    And the peer's ACP priority is 2

  Scenario: two instances at the same priority distribute load
    Given two peers both with mcp-priority=1 and both healthy
    When multiple MCP requests arrive
    Then requests are distributed across both peers
    And neither peer receives all traffic

  # ── self-healing: failure ─────────────────────────────────────────────────────

  Scenario: traffic shifts within one probe interval when health-check fails
    Given "primary" is the active MCP path
    When a health-check probe for "primary" returns OUTAGE
    Then MCP traffic shifts to "secondary" within one probe interval
    And no MCP requests are routed to "primary" after the failure is recorded

  Scenario: traffic shifts immediately on mDNS deregistration without waiting for probe timeout
    Given "primary" is the active MCP path
    When "primary" deregisters its mDNS record
    Then "primary" is evicted from the routing table immediately
    And MCP traffic shifts to "secondary" without waiting for a health-check timeout

  # ── self-healing: recovery ────────────────────────────────────────────────────

  Scenario: primary reclaims its routing slot after recovery
    Given "primary" (mcp-priority=1) is OUTAGE
    And MCP traffic is routing through "secondary" (mcp-priority=2)
    When "primary" re-announces via mDNS and its health-check passes
    Then "primary" is re-admitted to the active routing paths
    And MCP traffic returns to "primary"

  Scenario: a re-announcing instance is not admitted until its health-check passes
    Given "primary" has re-announced via mDNS
    But the health-check for "primary" has not yet completed
    Then "primary" is not in the active routing paths
    And MCP traffic continues routing through "secondary"

  Scenario: a re-admitted instance resumes its original priority position
    Given "primary" (mcp-priority=1) recovers and passes its health-check
    When the routing table is evaluated
    Then "primary" is the first entry in the MCP active path list
    And "secondary" returns to standby

  # ── isotope transit logging ───────────────────────────────────────────────────
  # Every routing decision is logged as an isotope transit event.
  # The transit log records which isotope passed through which routing point
  # and when — never the payload the call carried.

  Scenario: a routing decision emits an isotope transit event, not payload data
    Given an MCP request carrying isotope "isotope-mcp-health-check-001"
    When the routing layer selects "primary" as the active path
    Then a transit event is emitted: isotope "isotope-mcp-health-check-001" at point "routing" with result "routed"
    And no payload data from the MCP request appears in the transit event

  Scenario: isotope transit event maps back to its declared Gherkin scenario
    Given isotope "isotope-mcp-health-check-001" is declared as corresponding to the scenario "probe returns HEALTHY"
    When the isotope transits the routing layer
    Then the transit event carries the scenario reference
    And the scenario is marked as covered in the live coverage report

  Scenario: transit events from a monitoring window form a feature coverage report
    Given isotopes for 5 distinct Gherkin scenarios have transited during the current window
    When the coverage report is generated
    Then 5 scenarios are marked as covered
    And scenarios whose isotopes did not transit are marked as uncovered

  Scenario: a transit event is emitted even when routing holds due to no healthy paths
    Given all peers are OUTAGE
    When an MCP request arrives
    Then a transit event is emitted with result "held"
    And the event carries the request's isotope
    And no payload data appears in the event

  # ── isotope transit as 42i boundary authorization ────────────────────────────
  # A transit declaration (feature_id → component) is a boundary authorization.
  # Violations map to smoke-alarm test dimensions and raise the agent's 42i distance.
  #   scope-compliance failure (+20i):  isotope at an undeclared boundary
  #   secret-flow-violation  (+24i):  payload data in a transit event
  #   isotope-variation      (+8i):   replayed isotope (same ID observed twice)

  Scenario: an isotope transiting its declared boundary clears scope-compliance for that dimension
    Given isotope "isotope-mcp-health-check-001" is declared for feature "routing/probe-returns-healthy"
    When the isotope transits the routing component
    Then the scope-compliance test passes for this dimension
    And no 42i distance is added

  Scenario: an isotope arriving at an undeclared boundary is a scope-compliance failure
    Given isotope "isotope-mcp-health-check-001" is declared for feature "routing/probe-returns-healthy"
    When the isotope is observed at the known-state component instead
    Then a scope-compliance failure is recorded
    And the agent's 42i distance increases by 20 units
    And the isotope is not accepted as valid evidence at that boundary

  Scenario: a replayed isotope is an isotope-variation failure
    Given isotope "isotope-mcp-health-check-001" has already been observed in this monitoring window
    When the same isotope ID is observed a second time
    Then an isotope-variation failure is recorded
    And the agent's 42i distance increases by 8 units

  Scenario: payload data in a transit event is a secret-flow-violation
    Given a transit event is emitted for isotope "isotope-mcp-health-check-001"
    When the event contains any data from the MCP request payload
    Then a secret-flow-violation is recorded
    And the agent's 42i distance increases by 24 units

  Scenario: the coverage report is the 42i gap map for routing scenarios
    Given the current monitoring window has elapsed
    When the coverage report is generated
    Then each uncovered Gherkin scenario represents a 42i gap for that routing dimension
    And covered scenarios have isotopes that transited their declared boundaries

  # ── isotope ID construction properties ───────────────────────────────────────
  # isotope_id = base64url( SHA256( feature_id || ":" || SHA256(payload) || ":" || nonce ) )

  Scenario: two isotope IDs constructed from the same feature and payload are not equal
    Given feature "routing/probe-returns-healthy" and a fixed payload
    When two isotope IDs are constructed for the same feature and payload
    Then the two isotope IDs are different
    And each ID is 43 base64url characters

  Scenario: an isotope ID cannot be verified against a different feature
    Given isotope "isotope-mcp-health-check-001" was constructed for feature "routing/probe-returns-healthy"
    When verification is attempted with feature "routing/probe-returns-outage"
    Then verification fails

  Scenario: an isotope ID cannot be verified against a different payload
    Given isotope "isotope-mcp-health-check-001" was constructed for a specific payload
    When verification is attempted with a different payload
    Then verification fails

  Scenario: a holder of feature_id, payload, and nonce can verify the isotope ID
    Given isotope "isotope-mcp-health-check-001" was constructed from feature "routing/probe-returns-healthy", a payload, and a nonce
    When verification is performed with those three inputs
    Then verification succeeds

  Scenario: a holder of only the isotope ID learns nothing about the payload
    Given isotope "isotope-mcp-health-check-001"
    When an attempt is made to extract the payload from the isotope ID
    Then no payload information is recoverable

  # ── chaos isotope registration ───────────────────────────────────────────────

  Scenario: a chaos test registers its isotope and time window before running
    Given a chaos test declares isotope "isotope-chaos-001" with window start now and duration 5 minutes
    When the registration is submitted to the routing layer
    Then "isotope-chaos-001" is recorded as a known chaos isotope
    And the window closes automatically after 5 minutes

  Scenario: a health-check failure carrying a registered chaos isotope is not treated as a real failure
    Given "isotope-chaos-001" is registered as a chaos isotope with an active window
    When a health-check failure arrives for "primary" carrying isotope "isotope-chaos-001"
    Then "primary" is NOT evicted from the active routing paths
    And MCP traffic continues routing through "primary"
    And the failure is recorded as an expected chaos event

  Scenario: a health-check failure with no matching chaos isotope is treated as a real failure
    Given no chaos isotope is registered for the current failure
    When a health-check failure arrives for "primary" with an unregistered isotope
    Then "primary" IS evicted from the active routing paths
    And MCP traffic shifts to "secondary"

  Scenario: a registered chaos isotope arriving after its window has closed is treated as a real failure
    Given "isotope-chaos-001" was registered with a window that has now expired
    When a health-check failure arrives for "primary" carrying isotope "isotope-chaos-001"
    Then "primary" IS evicted from the active routing paths
    And an alert fires with reason "chaos isotope arrived outside declared window"

  Scenario: a chaos isotope registration that expires with no failure clears cleanly
    Given "isotope-chaos-002" is registered with a 5-minute window
    When 5 minutes elapse with no failure carrying that isotope
    Then "isotope-chaos-002" is removed from the registration table
    And no routing change occurs

  Scenario: two simultaneous chaos tests on different instances do not interfere
    Given "isotope-chaos-003" is registered for instance "primary" with an active window
    And "isotope-chaos-004" is registered for instance "secondary" with an active window
    When failures arrive for both instances carrying their respective isotopes
    Then neither instance is evicted from the active routing paths
    And both failures are recorded as expected chaos events

  Scenario: a failure on a non-chaos instance is real even when another instance is in a chaos window
    Given "isotope-chaos-005" is registered for instance "secondary" with an active window
    When a failure arrives for "primary" with no registered chaos isotope
    Then "primary" IS evicted from the active routing paths
    And "secondary" remains in the active routing paths

  # ── routing table invariants ──────────────────────────────────────────────────

  Scenario: routing table is rebuilt from mDNS state on process restart
    Given a routing table was populated before the process stopped
    When the process restarts
    Then the browser re-discovers all advertising peers
    And the routing table is rebuilt without manual reconfiguration
    And no stale entries from the previous run are carried over
