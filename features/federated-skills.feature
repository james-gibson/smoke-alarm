# features/federated-skills.feature
# Canon record — last audited: 2026-03-26
# Exercises: federated skill identity, skill announcement via InstanceRecord, skill routing
# Code: internal/federation/, internal/skills/ (federation awareness not yet implemented)
# Step definitions: features/step_definitions/federated_skills_steps.go
# see: features/federation.feature (InstanceRecord, membership, slot election)
# see: features/skill-system.feature (ValidateSkillFile, FindSkills — local)
#
# IMPLEMENTATION NOTE:
#   InstanceRecord.Meta map[string]string is the current extension point.
#   Skill announcement may use Meta["skills"] or a dedicated Skills []string field
#   on InstanceRecord — the choice is an implementation decision.
#   This feature specs the BEHAVIOUR regardless of wire representation.
#
# NAMESPACING CONVENTION (consistent with target ID fan-out in federation.go):
#   Local skill:     "open-the-pickle-jar"
#   Federated skill: "[instance-id] open-the-pickle-jar"
#   where instance-id is the 16-hex-char identity from ClaimSlot.
#
# ROUTING CONSTRAINT (from CLAUDE.md):
#   Fan-out is valid. Cycles are forbidden. Every routed skill invocation
#   carries a routing trace. An invocation arriving at instance B that
#   originated at instance B is rejected as a cycle.

@federated-skills @optional
Feature: Federated Skill Identity and Discovery
  As an operator or agent working across a federation mesh
  I want each instance to announce its available skills as part of its membership identity
  So that skills can be discovered, routed, and invoked across instances with unambiguous namespacing

  Background:
    Given a federation mesh is running with at least 2 instances
    And each instance has at least one skill installed

  # ── skill announcement ────────────────────────────────────────────────────

  Scenario: an instance includes its skill inventory in its InstanceRecord on introduction
    Given instance "inst-a" has skills ["open-the-pickle-jar", "start-here"] installed
    When inst-a sends a POST to /introductions
    Then the InstanceRecord in the request body contains the skill inventory for inst-a

  Scenario: an instance includes its skill inventory in heartbeat records
    Given instance "inst-a" has skills ["open-the-pickle-jar"] installed
    When inst-a sends a POST to /heartbeats
    Then the InstanceRecord in the request body contains the skill inventory for inst-a

  Scenario: adding a new skill to an instance is reflected in the next heartbeat
    Given instance "inst-a" is running with skill "start-here" installed
    When the skill "open-the-pickle-jar" is added to inst-a
    And inst-a sends its next heartbeat
    Then the heartbeat InstanceRecord contains both "start-here" and "open-the-pickle-jar"

  # ── federated skill namespacing ───────────────────────────────────────────

  Scenario: a skill from a remote peer is identified as "[instance-id] skill-name"
    Given instance "inst-b" has identity id "abc123def456abcd" and skill "start-here"
    When the introducer aggregates skill inventories from its peers
    Then the federated skill id is "[abc123def456abcd] start-here"

  Scenario: a local skill on the introducer retains its unqualified name
    Given the introducer "inst-a" has skill "open-the-pickle-jar" installed locally
    Then the skill id is "open-the-pickle-jar" without a namespace prefix

  Scenario: two peers with the same skill name produce distinct federated skill IDs
    Given instance "inst-b" (id "aaaa000000000001") has skill "start-here"
    And instance "inst-c" (id "bbbb000000000002") has skill "start-here"
    When the introducer aggregates skill inventories
    Then the federated skill IDs are:
      | federated-skill-id                    |
      | [aaaa000000000001] start-here         |
      | [bbbb000000000002] start-here         |

  # ── membership API includes skill inventory ───────────────────────────────

  Scenario: GET /membership includes skill inventory for each peer
    Given 2 follower peers each with distinct skill sets
    When a GET request is sent to /membership on the introducer
    Then each entry in the "peers" array contains a skill inventory field
    And the skill inventory for each peer lists that peer's installed skills

  Scenario: GET /membership skill inventory is empty for a peer with no skills installed
    Given a follower peer with no skills installed
    When a GET request is sent to /membership
    Then that peer's skill inventory field is an empty list

  # ── federated skill lookup ────────────────────────────────────────────────

  Scenario: a skill can be looked up by its federated ID to find its hosting instance
    Given the federated skill registry contains "[abc123def456abcd] open-the-pickle-jar"
    When a lookup is performed for federated skill id "[abc123def456abcd] open-the-pickle-jar"
    Then the result identifies instance "abc123def456abcd" as the host
    And the local skill name is "open-the-pickle-jar"

  Scenario: looking up an unknown federated skill ID returns not-found
    When a lookup is performed for federated skill id "[unknown000000000] no-such-skill"
    Then the result is not found

  # ── skill routing and routing trace ──────────────────────────────────────

  Scenario: invoking a federated skill routes the request to the correct instance
    Given the local instance is "inst-a" (id "aaaa000000000001")
    And the federated skill "[bbbb000000000002] open-the-pickle-jar" is hosted on inst-b
    When the agent on inst-a invokes "[bbbb000000000002] open-the-pickle-jar"
    Then the invocation is routed to inst-b
    And the routing trace contains ["aaaa000000000001", "bbbb000000000002"]

  Scenario: a routed skill invocation carries a routing trace through each hop
    Given a 3-instance mesh: inst-a → inst-b → inst-c (linear fan-out, no cycle)
    When inst-a invokes a skill hosted on inst-c via inst-b
    Then the routing trace on arrival at inst-c is ["inst-a-id", "inst-b-id", "inst-c-id"]

  Scenario: a skill invocation is rejected when the routing trace contains the local instance ID
    Given instance "inst-b" receives a skill invocation
    And the routing trace already contains "inst-b"'s own instance ID
    When the invocation is processed
    Then the invocation is rejected with a cycle error
    And a "warn" log entry is written containing "routing cycle detected"

  Scenario: a direct cycle A→B→A is rejected at the second hop
    Given instance A invokes a skill on instance B
    And instance B attempts to route the same invocation back to instance A
    When instance A receives the re-routed invocation
    Then the invocation is rejected
    And the error identifies both instance IDs in the cycle

  # ── open-the-pickle-jar in federated context ──────────────────────────────

  Scenario: open-the-pickle-jar invoked directly on a federated instance summarises that instance's skills
    Given the agent is operating on instance "inst-b" within a federation mesh
    When open-the-pickle-jar is invoked with no scope argument on inst-b
    Then the output lists skills installed on inst-b
    And the output identifies inst-b by its instance ID and role
    And the output notes which skills are also present on peer instances

  Scenario: open-the-pickle-jar targeted at a federated instance ID audits that instance's skills
    Given the local instance is the introducer
    And instance "inst-b" (id "bbbb000000000002") is a registered peer with skill "open-the-pickle-jar"
    When open-the-pickle-jar is invoked with scope "[bbbb000000000002]"
    Then the invocation is routed to inst-b
    And the resulting Gherkin audit covers inst-b's skill domain
    And the output includes inst-b's instance ID in the audit header

  Scenario: open-the-pickle-jar audit output distinguishes local vs federated skill coverage
    Given a federation mesh where inst-a has features/federation.feature and inst-b does not
    When open-the-pickle-jar is invoked on inst-a with scope "federation"
    Then the audit result notes that federation.feature exists locally on inst-a
    And notes that inst-b has no local federation.feature

  # ── peer departure removes federated skills ───────────────────────────────

  Scenario: skills from a peer are removed from the federated registry when the peer ages out
    Given instance "inst-b" is registered with skills ["open-the-pickle-jar"]
    And "heartbeat_timeout" is set to "3s"
    When inst-b stops heartbeating for 4 seconds
    Then "[inst-b-id] open-the-pickle-jar" is no longer present in the federated skill registry
    And a registry event of type "removed" is fired for inst-b's skill entries

  Scenario: skills from a peer are removed when the peer is explicitly removed from the registry
    Given instance "inst-b" is registered with skills ["start-here", "demo-capabilities"]
    When Remove is called for inst-b's ID
    Then both "[inst-b-id] start-here" and "[inst-b-id] demo-capabilities" are removed
    And the federated skill count decreases by 2

  # ── selfdescription reflects federated skills ─────────────────────────────
  # see: features/selfdescription.feature

  Scenario: the self-description document lists locally installed skills
    Given the instance has skills ["open-the-pickle-jar", "start-here"] installed
    When a GET request is sent to /.well-known/smoke-alarm.json
    Then the document contains a "skills" field listing the local skill names

  Scenario: the self-description document lists federated skill IDs from peer instances
    Given the introducer has 2 peers each with 1 skill
    When a GET request is sent to /.well-known/smoke-alarm.json on the introducer
    Then the document contains a "federated_skills" field with 2 entries
    And each entry follows the "[instance-id] skill-name" format
