# features/certification-gate.feature
# Next-steps record — isotope certification gate for cluster discovery
# Dependency: isotope-transit.feature (isotope_id flows through /status)
# Dependency: federation.feature (InstanceRecord, membership, slot election)
# Dependency: chaos-window.feature (classification: registered-chaos / unregistered)
# Step definitions: features/step_definitions/certification_gate_steps.go (to be created)
#
# An instance is "certified" if it can demonstrate isotope transit capability —
# i.e. its probe responses carry isotope IDs that have been verified against
# their declared feature bindings. Uncertified instances are excluded from full
# federation membership and are not routed to for skill invocations.
#
# Certification status is carried in InstanceRecord.Meta["isotope_certified"]
# and is re-evaluated on each heartbeat. An instance self-certifies by running
# the isotope transit pipeline; the introducer does not independently verify.
#
# Design choice: the introducer trusts the certification claim in the heartbeat
# and tombstones instances whose claim is revoked or absent for >N cycles.

@certification-gate @optional
Feature: Isotope Certification Gate for Cluster Discovery
  As an operator running a federation mesh with chaos isotope support
  I want uncertified instances to be excluded from full cluster membership
  So that only instances that have demonstrated isotope transit capability
  can participate in chaos classification and MCP skill routing

  Background:
    Given the ocd-smoke-alarm binary is installed
    And federation is enabled in the config
    And the introducer is running and serving /introductions

  # ── certification claim in InstanceRecord ───────────────────────────────────

  Scenario: a certified instance includes isotope_certified=true in its InstanceRecord
    Given instance "inst-b" has successfully carried at least one verified isotope
    When inst-b sends a POST to /introductions
    Then the InstanceRecord includes Meta["isotope_certified"] = "true"

  Scenario: an uncertified instance includes isotope_certified=false in its InstanceRecord
    Given instance "inst-b" has never carried a verified isotope
    When inst-b sends a POST to /introductions
    Then the InstanceRecord includes Meta["isotope_certified"] = "false"

  Scenario: certification status is included in heartbeat records
    Given instance "inst-b" becomes certified between introduction and next heartbeat
    When inst-b sends its next POST to /heartbeats
    Then the InstanceRecord includes Meta["isotope_certified"] = "true"

  # ── introducer acceptance policy ────────────────────────────────────────────

  Scenario: the introducer accepts a certified instance as a full peer
    Given inst-b presents Meta["isotope_certified"] = "true" in its introduction
    When the introducer processes the introduction
    Then inst-b is added to the registry with role "peer"
    And inst-b appears in GET /membership with certification status "certified"

  Scenario: the introducer accepts but restricts an uncertified instance
    Given inst-b presents Meta["isotope_certified"] = "false" in its introduction
    When the introducer processes the introduction
    Then inst-b is added to the registry with role "peer-uncertified"
    And inst-b appears in GET /membership with certification status "uncertified"
    And inst-b is excluded from federated skill routing

  Scenario: an uncertified instance is not included in the routing mesh for skill invocations
    Given inst-b is registered with role "peer-uncertified"
    And inst-b has skill "adhd.chaos.register-window" in its InstanceRecord
    When a JSON-RPC call arrives for "[inst-b-id] adhd.chaos.register-window"
    Then the call is rejected with error "peer uncertified: [inst-b-id]"
    And no proxy request is sent to inst-b

  Scenario: an uncertified instance that certifies on a later heartbeat is promoted to full peer
    Given inst-b is registered with role "peer-uncertified"
    When inst-b sends a heartbeat with Meta["isotope_certified"] = "true"
    Then inst-b is promoted to role "peer" in the registry
    And inst-b is now included in federated skill routing

  # ── membership visibility ───────────────────────────────────────────────────

  Scenario: GET /membership marks each peer's certification status
    Given a mesh with 1 certified peer and 1 uncertified peer
    When a GET request is sent to /membership
    Then each entry in the "peers" array contains an "isotope_certified" field
    And the certified peer shows isotope_certified = true
    And the uncertified peer shows isotope_certified = false

  Scenario: a certified instance can query the list of uncertified peers
    Given the introducer has 3 peers, 1 of which is uncertified
    When a GET request is sent to /membership?filter=uncertified
    Then the response contains exactly 1 peer
    And that peer has isotope_certified = false

  # ── revocation ──────────────────────────────────────────────────────────────
  # An instance that loses its certification (e.g. its probe pipeline breaks)
  # must report isotope_certified=false on the next heartbeat. The introducer
  # then demotes it from "peer" to "peer-uncertified".

  Scenario: an instance revokes its certification by sending isotope_certified=false in a heartbeat
    Given inst-b is registered as a full peer with isotope_certified = true
    When inst-b sends a heartbeat with Meta["isotope_certified"] = "false"
    Then inst-b is demoted to role "peer-uncertified"
    And a "warn" log entry is written containing "peer certification revoked: [inst-b-id]"
    And inst-b is excluded from federated skill routing

  Scenario: an instance that sends 3 consecutive heartbeats with no isotope_certified field is demoted
    Given inst-b sends heartbeats with Meta missing the isotope_certified key
    When 3 consecutive heartbeats are received with no isotope_certified field
    Then inst-b is demoted to "peer-uncertified"
    And a "warn" log entry is written containing "certification claim absent"

  # ── scope compliance failures ───────────────────────────────────────────────
  # An isotope-variation failure (same target, different isotope in same window)
  # is a scope compliance failure. Repeated violations revoke certification.

  Scenario: a single isotope-variation failure does not revoke certification
    Given inst-b is a certified peer
    And a single isotope-variation failure is recorded for inst-b's probe endpoint
    When inst-b sends its next heartbeat
    Then inst-b remains certified
    And the variation failure is noted in the membership record

  Scenario: three isotope-variation failures within a monitoring window revoke certification
    Given inst-b is a certified peer
    And 3 isotope-variation failures are recorded within a 5-minute window
    When inst-b sends its next heartbeat
    Then inst-b declares Meta["isotope_certified"] = "false"
    And the introducer demotes inst-b to "peer-uncertified"

  # ── plaintext isotopes and certification ────────────────────────────────────
  # In dev mode, plaintext isotopes count toward certification but are marked
  # as dev-mode only. Production introducers may refuse to accept
  # plaintext-certified peers for skill routing in production clusters.

  Scenario: a dev-mode instance certified via plaintext isotopes is marked "dev-certified"
    Given inst-b is running with allow_plaintext_isotopes=true
    And its probe responses carry "plaintext:adhd/mdns-discovery:001"
    When inst-b sends an introduction
    Then the InstanceRecord includes Meta["isotope_certified"] = "dev-certified"

  Scenario: a production introducer restricts dev-certified peers to the same policy as uncertified
    Given the introducer is running in production mode (allow_plaintext_isotopes=false)
    And inst-b presents Meta["isotope_certified"] = "dev-certified"
    When the introducer processes the introduction
    Then inst-b is registered as "peer-uncertified"
    And a "info" log entry is written containing "dev-certified peer restricted in production"

  # ── ADHD-side view of certification gate ───────────────────────────────────
  # ADHD discovers smoke-alarm instances via mDNS or federation. It reads the
  # /.well-known/smoke-alarm.json selfdescription to determine certification.
  # Uncertified alarms are still displayed but flagged in the dashboard.

  Scenario: ADHD treats an alarm with isotope_certified=true as a trusted target source
    Given a smoke-alarm advertises itself via mDNS with isotope_certified=true in its selfdescription
    When ADHD adds it to the cluster
    Then the alarm's targets are displayed normally
    And the alarm is eligible for chaos isotope classification

  Scenario: ADHD flags an alarm with isotope_certified=false in the dashboard
    Given a smoke-alarm advertises itself with isotope_certified=false
    When ADHD adds it to the cluster
    Then the alarm's smoke: lights are shown with a "uncertified" annotation
    And no chaos isotope suppression is applied to that alarm's targets
