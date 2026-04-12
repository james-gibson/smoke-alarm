# features/skill-certification.feature
# Next-steps record — dynamic skill certification via the 42i trust model
# Dependency: trust-rungs.feature        (rung assignment and distance ceilings)
# Dependency: federated-skills.feature   (skill identity, namespacing, routing)
# Dependency: certification-gate.feature (instance-level certification baseline)
# Step definitions: features/step_definitions/skill_certification_steps.go (to be created)
#
# Skills are managed like isotopes: they are certified externally by the smoke-alarm,
# not self-declared by the instance hosting them. A skill has a certified rung
# determined by observed evidence at three levels:
#
#   Existence  — the skill is reachable and responds to a probe call (rung ≥ 1)
#   Equality   — the skill produces equal output for equal inputs (rung ≥ 2)
#   Isotope    — skill execution is traceable via an isotope transit to a Gherkin
#               scenario, proving the capability runs in the real system (rung ≥ 3)
#
# Skills are also "distanced" from the certifying alarm:
#   distance = number of proxy hops between the skill and the smoke-alarm that certified it
#   Each hop adds to the skill's 42i distance, making it less trusted.
#   A skill at distance 0 (hosted directly on the certified instance) has maximum trust.
#   A skill proxied through 3 hops is further from the alarm and carries higher distance.
#
# Skill-variation failure: if equal inputs produce non-equal outputs across invocations,
# this is a skill-variation failure. Each failure adds 8 to the skill's 42i distance,
# analogous to isotope-variation. Sufficient failures demote the skill's rung.
#
# Dynamic certification: skill rung is re-evaluated on each heartbeat. A skill that
# was rung 3 can be demoted to rung 1 if equality certification lapses or the
# isotope transit that proved it stops arriving.

@skill-certification @optional
Feature: Dynamic Skill Certification via 42i Trust Model
  As a smoke-alarm maintaining a federation mesh
  I want to certify each skill based on observed evidence (existence, equality, isotope)
  So that agents and callers receive only as much trust as the skill has demonstrated

  Background:
    Given the ocd-smoke-alarm binary is installed
    And federation is enabled with at least one peer instance
    And the peer has at least one skill installed

  # ── existence certification (rung 1) ─────────────────────────────────────────

  Scenario: a skill is existence-certified when it responds to a probe call
    Given instance "inst-b" announces skill "open-the-pickle-jar" in its InstanceRecord
    When the smoke-alarm sends a probe call to "open-the-pickle-jar" with empty params
    And the skill responds within the probe timeout
    Then the skill is assigned rung 1 (existence-certified) in the skill registry
    And the skill is marked reachable in GET /membership

  Scenario: a skill that does not respond to a probe remains at rung 0
    Given instance "inst-b" announces skill "ghost-skill" in its InstanceRecord
    When the probe call to "ghost-skill" times out
    Then the skill remains at rung 0 (declared only)
    And the skill is excluded from federated skill routing

  Scenario: existence certification is re-evaluated on each heartbeat cycle
    Given "open-the-pickle-jar" is existence-certified at rung 1
    When the next heartbeat cycle's probe call fails
    Then the skill is demoted to rung 0
    And a "warn" log entry is written: "skill existence probe failed: open-the-pickle-jar"

  Scenario: a skill at rung 0 is not included in tools/list responses to callers
    Given "ghost-skill" is at rung 0 (probe has never succeeded)
    When a tools/list call arrives
    Then "ghost-skill" is absent from the response
    And no caller learns of its existence

  # ── equality certification (rung 2) ─────────────────────────────────────────
  # Equality: the skill returns the same output for the same input.
  # The smoke-alarm runs two probe calls with identical params and compares results.

  Scenario: a skill is equality-certified when two probe calls with identical inputs return identical outputs
    Given "open-the-pickle-jar" is existence-certified at rung 1
    When the smoke-alarm sends two probe calls with identical params
    And both responses are byte-equal
    Then the skill is promoted to rung 2 (equality-certified)

  Scenario: a skill fails equality certification when identical inputs produce different outputs
    Given "open-the-pickle-jar" is at rung 1
    When two probe calls with identical params return different outputs
    Then a skill-variation failure is recorded for "open-the-pickle-jar"
    And the skill's 42i distance increases by 8
    And the skill remains at rung 1 (equality not certified)

  Scenario: three skill-variation failures within a monitoring window demote to rung 0
    Given "open-the-pickle-jar" is at rung 2 with 3 variation failures in a 5-minute window
    When the fourth failure arrives
    Then the skill is demoted to rung 0
    And a "warn" log entry is written: "skill variation failures exceeded threshold: open-the-pickle-jar"

  Scenario: equality is re-evaluated periodically and can lapse
    Given "open-the-pickle-jar" was equality-certified at rung 2
    When the next equality probe cycle returns non-equal outputs
    Then a skill-variation failure is recorded
    And if distance exceeds the rung-2 ceiling (80), the skill is demoted to rung 1

  # ── isotope certification (rung 3+) ─────────────────────────────────────────
  # A skill is isotope-certified when its execution produces an isotope transit
  # that maps to a declared Gherkin scenario, proving the capability runs in the
  # real system and not just as a probe stub.

  Scenario: a skill is isotope-certified when its execution carries a verifiable isotope
    Given "open-the-pickle-jar" is equality-certified at rung 2
    When an invocation of "open-the-pickle-jar" produces an isotope in its response
    And the isotope verifies against its declared feature binding
    Then the skill is promoted to rung 3 (isotope-certified)
    And the certifying isotope is recorded as the skill's feature binding

  Scenario: the isotope in a skill response is treated as a transit event
    Given "open-the-pickle-jar" returns isotope "iso-abc-001" in its result
    When the smoke-alarm processes the skill response
    Then a transit event is recorded: isotope "iso-abc-001" observed via skill "open-the-pickle-jar"
    And the transit maps to the Gherkin scenario declared in the feature binding

  Scenario: a skill at rung 3 whose isotope stops appearing is demoted to rung 2
    Given "open-the-pickle-jar" is isotope-certified at rung 3
    When 3 consecutive invocations return no isotope in the response
    Then the skill is demoted to rung 2 (equality still holds, but isotope transit lapsed)
    And a "warn" log entry is written: "skill isotope transit lapsed: open-the-pickle-jar"

  # ── skill distance from the certifying alarm ────────────────────────────────
  # A skill's distance is the number of proxy hops between the skill and the
  # smoke-alarm that certified it. Distance adds to the skill's 42i distance.
  # Skills closer to the alarm are more trusted.

  Scenario: a skill hosted directly on a certified instance has distance 0
    Given inst-b is directly connected to the smoke-alarm (no proxy hops)
    And "open-the-pickle-jar" is hosted on inst-b
    When the skill's distance is evaluated
    Then the distance is 0
    And no distance penalty is added to the skill's 42i distance

  Scenario: a skill proxied through one hop has distance 1
    Given inst-b hosts "open-the-pickle-jar"
    And the smoke-alarm reaches inst-b via inst-a (one proxy hop)
    When the skill's distance is evaluated
    Then the distance is 1
    And the skill's 42i distance includes a hop penalty of 8

  Scenario: a skill proxied through N hops has distance N and cumulative penalty
    Given "open-the-pickle-jar" is 3 hops from the smoke-alarm
    Then the skill's 42i distance penalty is 3 × 8 = 24
    And the effective rung ceiling is reduced accordingly

  Scenario: two instances hosting the same skill — the closer instance is preferred for routing
    Given inst-b (distance 1) and inst-c (distance 3) both host "open-the-pickle-jar"
    And both are equality-certified at rung 2
    When a call arrives for "open-the-pickle-jar"
    Then inst-b is preferred (lower distance = lower 42i distance = more trusted)

  # ── skill rung in tools/list and routing ────────────────────────────────────

  Scenario: tools/list includes the certified rung for each skill
    Given the skill registry contains:
      | skill                  | rung | distance |
      | open-the-pickle-jar    | 3    | 0        |
      | start-here             | 2    | 1        |
      | ghost-skill            | 0    | 0        |
    When a tools/list call arrives from a caller at rung 3
    Then the response includes open-the-pickle-jar (rung 3) and start-here (rung 2)
    And ghost-skill is absent (rung 0 — existence not verified)

  Scenario: caller at rung 1 can only invoke skills at rung ≤ 1
    Given caller is certified at rung 1
    And skill "open-the-pickle-jar" is at rung 3
    When the caller invokes "open-the-pickle-jar"
    Then the call is rejected with error code -32003
    And the error message is "insufficient trust rung: skill requires 3, caller has 1"

  Scenario: skill rung gates the content of the response, not just invocability
    Given skill "open-the-pickle-jar" at rung 3 returns:
      | field            | min_rung |
      | skill_inventory  | 0        |
      | coverage_map     | 2        |
      | isotope_log      | 3        |
    And caller is at rung 2
    When the skill executes
    Then skill_inventory and coverage_map are returned
    And isotope_log is omitted

  # ── dynamic certification in heartbeats ─────────────────────────────────────
  # Skill certification is re-evaluated on each heartbeat. The rung can advance
  # or retreat based on the latest probe results. Callers always see the
  # current certified rung, not the rung at time of last call.

  Scenario: skill rung advances from 1 to 2 during a heartbeat cycle
    Given "open-the-pickle-jar" is at rung 1 at the start of a heartbeat cycle
    And the equality probe returns equal outputs
    When the heartbeat cycle completes
    Then "open-the-pickle-jar" is at rung 2
    And the next tools/list reflects the updated rung

  Scenario: skill rung retreats from 3 to 2 during a heartbeat cycle
    Given "open-the-pickle-jar" is at rung 3
    And the isotope transit probe returns no isotope
    When the heartbeat cycle completes
    Then "open-the-pickle-jar" is at rung 2
    And callers at rung 2 can still invoke it
    And callers expecting rung-3 field isotope_log receive an omitted field

  Scenario: rapid rung changes within a single monitoring window are smoothed
    Given "open-the-pickle-jar" alternates between rung 2 and rung 1 on each cycle
    When 5 cycles are observed
    Then the reported rung is the minimum observed in the window
    And no caller sees a rung higher than what was consistently sustained
