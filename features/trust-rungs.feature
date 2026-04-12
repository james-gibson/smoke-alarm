# features/trust-rungs.feature
# Next-steps record — 42i trust rungs: certification levels and value-read authorization
# Dependency: isotope-transit.feature (isotopes must flow before rungs can advance)
# Dependency: certification-gate.feature (certified/uncertified baseline)
# Dependency: chaos-window.feature (rung 3 requires active chaos classification)
# Step definitions: features/step_definitions/trust_rung_steps.go (to be created)
#
# Trust rungs are a coarse bucketing of 42i distance:
#
#   Rung 0 — uncertified        isotope_certified=false; no isotope transit observed
#   Rung 1 — dev-certified      plaintext isotopes flowing; allow_plaintext_isotopes=true
#   Rung 2 — transit-certified  cryptographic isotopes verified against feature bindings
#   Rung 3 — chaos-certified    chaos windows active; suppression confirmed working
#   Rung 4 — peer-certified     inter-peer transit verified; probeIsotopeRegistration passed
#
# Each rung unlocks additional readable fields in /status responses, JSON-RPC results,
# and skill outputs. This is the encryption stand-in: no crypto yet — authorization
# gating at the rung boundary does the work. When encryption is added, each rung
# will correspond to a key tier. The wire protocol and gating logic do not change.
#
# Conservation principle:
#   Untouched data maintains its classification — it does not decay or drift.
#   Only interaction (read, request, manipulate) shifts 42i distance, usually
#   upward (less permissive). Successful verification heals distance (downward).
#   A rung is a distance ceiling, not a fixed label — exceed the ceiling and
#   you are demoted. Higher rungs have larger ceilings (rung 6 → 220, like HP).
#
# "Can a value be read" is answered by:
#   server looks up caller's certified rung in its own registry (by instance ID or connection identity)
#   caller_rung >= value.min_rung → return the value
#   caller_rung <  value.min_rung → omit the field (or return empty/null)
#
# The caller does NOT declare its own rung. The server is the authority.
# A binary may believe itself to be rung 6 internally. If the smoke-alarm's
# registry has it at rung 1, it receives rung-1 data. The binary's self-assessment
# is never the input to the authorization decision.
#
# JSON-RPC responses are filtered on the server side before transmission.
# The caller never learns that a field was omitted — the response is structurally valid.

@trust-rungs @optional
Feature: 42i Trust Rungs — Certification Levels and Read Authorization
  As a smoke-alarm instance or ADHD dashboard requesting values from a peer
  I want my certified trust rung to determine which fields and outputs I can receive
  So that authorization is derived from demonstrated isotope capability
  rather than pre-shared credentials or static ACLs

  Background:
    Given the ocd-smoke-alarm binary is installed
    And federation is enabled in the config

  # ── rung assignment ─────────────────────────────────────────────────────────

  Scenario: an instance with no isotope activity is assigned rung 0
    Given instance "inst-b" has never emitted or verified an isotope
    When the trust rung for "inst-b" is evaluated
    Then inst-b is assigned rung 0 (uncertified)

  Scenario: an instance with plaintext isotopes flowing is assigned rung 1
    Given instance "inst-b" has allow_plaintext_isotopes=true
    And "inst-b" probe responses carry at least one "plaintext:" isotope
    When the trust rung for "inst-b" is evaluated
    Then inst-b is assigned rung 1 (dev-certified)

  Scenario: an instance with verified cryptographic isotopes is assigned rung 2
    Given instance "inst-b" has carried at least one isotope whose ID verifies against its declared feature binding (cryptographic algorithm)
    When the trust rung for "inst-b" is evaluated
    Then inst-b is assigned rung 2 (transit-certified)

  Scenario: an instance with active chaos windows and confirmed suppression is assigned rung 3
    Given instance "inst-b" is rung 2
    And at least one chaos window has been registered and a suppression decision confirmed
    When the trust rung for "inst-b" is evaluated
    Then inst-b is assigned rung 3 (chaos-certified)

  Scenario: an instance whose inter-peer isotope registration was observed is assigned rung 4
    Given instance "inst-b" is rung 3
    And a probeIsotopeRegistration call to inst-b returned a non-empty isotope list
    When the trust rung for "inst-b" is evaluated
    Then inst-b is assigned rung 4 (peer-certified)

  Scenario: 42i distance decrease advances the trust rung
    Given inst-b is at rung 2 with a 42i distance of 80
    When additional isotope transits reduce the 42i distance below the rung-3 threshold
    Then inst-b is promoted to rung 3

  Scenario: a scope compliance failure (isotope-variation) increases 42i distance by 8
    Given inst-b is at rung 3 with a 42i distance of 40
    When an isotope-variation failure is recorded for inst-b
    Then the 42i distance increases by 8
    And if the distance crosses the rung-3 floor, inst-b is demoted to rung 2

  # ── rung is carried in InstanceRecord ──────────────────────────────────────

  Scenario: an instance declares its trust rung in Meta["trust_rung"] on introduction
    Given inst-b has evaluated its rung as 2
    When inst-b sends a POST to /introductions
    Then the InstanceRecord includes Meta["trust_rung"] = "2"

  Scenario: the trust rung is updated in each heartbeat
    Given inst-b advances from rung 2 to rung 3 between heartbeats
    When inst-b sends its next POST to /heartbeats
    Then the InstanceRecord includes Meta["trust_rung"] = "3"

  Scenario: the introducer computes the rung from observed evidence, not from the declared value
    Given inst-b declares Meta["trust_rung"] = "6" in its introduction
    But the introducer has only observed rung-1 evidence for inst-b (plaintext isotopes only)
    When the introducer processes the introduction
    Then inst-b is stored in the registry with trust_rung = 1 (evidence-derived)
    And the declared value of 6 is ignored

  # ── value-read authorization ────────────────────────────────────────────────
  # Every readable value has a min_rung annotation. The server filters responses
  # before transmission based on the caller's declared rung.

  Scenario: a caller certified at rung 0 reading /status receives target existence and health only
    Given inst-b is registered in the introducer's registry with trust_rung=0
    When inst-b sends GET /status (identified by its instance ID)
    Then the response includes: target id, state (healthy/unhealthy)
    And the response omits: isotope_id, isotope_classification, message, details

  Scenario: a caller certified at rung 1 reading /status also receives plaintext isotope_id
    Given inst-b is registered with trust_rung=1
    And the target carries a "plaintext:" isotope
    When inst-b sends GET /status
    Then the response includes isotope_id (plaintext form)
    And isotope_classification is still omitted (min_rung 3)

  Scenario: a caller certified at rung 2 reading /status receives isotope_id (canonical form)
    Given inst-b is registered with trust_rung=2
    And the target carries a canonical 43-char isotope_id
    When inst-b sends GET /status
    Then the response includes isotope_id
    And isotope_classification is still omitted (min_rung 3)

  Scenario: a caller certified at rung 3 reading /status receives isotope_classification
    Given inst-b is registered with trust_rung=3
    When inst-b sends GET /status
    Then the response includes isotope_id and isotope_classification

  Scenario: a caller certified at rung 4 reading /membership receives full routing trace fields
    Given inst-b is registered with trust_rung=4
    When inst-b sends GET /membership
    Then the response includes the full peer registry with routing trace metadata
    And each peer entry includes their individual trust_rung

  Scenario: a caller below the required rung receives the field omitted, not an error
    Given inst-b is registered with trust_rung=1
    When inst-b requests /status for a target with isotope_classification "registered-chaos"
    Then the response is HTTP 200
    And the isotope_classification field is absent from the JSON
    And no error or explanation is given for the omission
    And inst-b's self-belief about its own rung is irrelevant to the filtering decision

  # ── JSON-RPC skill output filtering ────────────────────────────────────────
  # Skill results are annotated with a min_rung. The proxy or local handler
  # strips output fields that exceed the caller's rung before returning the result.

  Scenario: a caller certified at rung 0 invoking a rung-3 skill is rejected
    Given skill "adhd.chaos.register-window" requires min_rung=3
    And inst-b is registered with trust_rung=0
    When the JSON-RPC call arrives from inst-b (identified by instance ID)
    Then the call is rejected with a JSON-RPC error code -32003
    And the error message is "insufficient trust rung: need 3, have 0"
    And no skill execution occurs
    And inst-b's self-belief about its rung does not affect the outcome

  Scenario: a caller certified at rung 3 invoking a rung-3 skill receives the full result
    Given skill "adhd.chaos.register-window" requires min_rung=3
    And inst-b is registered with trust_rung=3
    When the JSON-RPC call arrives from inst-b
    Then the skill executes normally
    And the full result is returned

  Scenario: a skill result with mixed-rung fields returns partial output to a mid-rung caller
    Given skill "open-the-pickle-jar" returns:
      | field               | min_rung |
      | skill_list          | 0        |
      | coverage_percent    | 2        |
      | isotope_transit_log | 3        |
    And inst-b is registered with trust_rung=2
    When the skill result is assembled for inst-b
    Then skill_list and coverage_percent are included
    And isotope_transit_log is omitted

  # ── proxying preserves caller identity ─────────────────────────────────────
  # When inst-a proxies a JSON-RPC call to inst-b, it forwards the *original
  # caller's instance ID* so inst-b looks up that caller's certified rung directly.
  # An intermediary cannot grant the caller a higher rung than the registry holds.

  Scenario: the proxy forwards the original caller's instance ID, not its own
    Given inst-a (rung 4) is proxying a call from inst-c (rung 1)
    When inst-a forwards the call to inst-b
    Then the forwarded request carries inst-c's instance ID as the originating caller
    And inst-b looks up inst-c in its registry (rung 1) and filters the response accordingly
    And inst-b does not use inst-a's rung (4) for any filtering decision

  Scenario: an intermediary cannot escalate a caller's rung by proxying under its own identity
    Given inst-a (rung 4) proxies a call for inst-c (rung 0) but presents itself as the caller
    When inst-b receives the call
    Then inst-b detects the originating-caller header is absent or forged
    And a "warn" log entry is written: "proxy forwarded without originating-caller identity"
    And the call is processed at the lowest known rung for the connection

  # ── data access shifts distance ─────────────────────────────────────────────
  # Untouched data maintains its classification — it does not decay or drift.
  # Only interaction (read, request, manipulate) shifts 42i distance, usually
  # in the less-permissive direction (distance up). Successful verification
  # (isotope transit confirmed, chaos suppression correct) heals distance (down).
  #
  # This creates natural entropy: access costs something; re-certification is
  # required to maintain a rung over time. A high rung reached through deep
  # verification has a large distance ceiling — it can absorb many access events
  # before demotion. A low rung has a small ceiling and is demoted quickly.

  Scenario: untouched data does not shift the caller's 42i distance
    Given inst-b has not requested any values from the smoke-alarm
    When time passes and no interaction occurs
    Then inst-b's 42i distance is unchanged
    And inst-b's rung is unchanged

  Scenario: reading data at or below the caller's rung incurs no distance cost
    Given inst-b is certified at rung 3
    And the requested field has min_rung=2
    When inst-b reads the field
    Then inst-b's 42i distance does not increase

  Scenario: successfully verifying an isotope transit reduces 42i distance
    Given inst-b has a 42i distance of 60
    When an isotope transit is confirmed (feature binding verified)
    Then inst-b's 42i distance decreases
    And if the new distance falls below a rung-advancement threshold, inst-b is promoted

  Scenario: a scope compliance failure (isotope variation) increases distance by 8
    Given inst-b has a 42i distance of 35
    When an isotope-variation failure is recorded for inst-b
    Then inst-b's 42i distance becomes 43
    And if 43 exceeds the rung-3 ceiling (40), inst-b is demoted to rung 2

  Scenario: manipulating a value (write, mutation) incurs a larger distance cost than reading
    Given inst-b is certified at rung 3 and writes to a chaos window registration
    When the write completes
    Then inst-b's 42i distance increases by a write-cost amount (implementation-defined)
    And the distance increase is larger than a read-cost for the same rung level

  # ── rung thresholds and distance ceilings ───────────────────────────────────
  # Higher rungs have larger distance ceilings — they can sustain more access
  # events before demotion. This rewards deep certification history.
  # Ceilings roughly double per rung, making high rungs disproportionately resilient.
  # Operators may configure thresholds in the smoke-alarm config.

  Scenario: default rung thresholds map to 42i distance ceilings
    Then the rung assignment table is:
      | rung | name              | 42i_distance_ceiling | condition                              |
      | 0    | uncertified       | ∞                    | no isotopes observed                   |
      | 1    | dev-certified     | ∞                    | plaintext isotopes only                |
      | 2    | transit-verified  | 80                   | cryptographic isotopes verified        |
      | 3    | chaos-certified   | 40                   | chaos suppression confirmed            |
      | 4    | peer-certified    | 16                   | inter-peer registration observed       |
      | 5    | mesh-certified    | 110                  | all mesh peers have certified each other |
      | 6    | temporally-stable | 220                  | transits verified across multiple time windows |

  Scenario: a rung-6 entity can sustain up to 220 distance before demotion to rung 5
    Given inst-b is certified at rung 6 with a current 42i distance of 180
    When a series of scope compliance failures adds 48 distance
    Then inst-b's distance reaches 228 and exceeds the rung-6 ceiling of 220
    And inst-b is demoted to rung 5 (ceiling: 110)
    And since 228 also exceeds 110, inst-b is demoted further to rung 4

  Scenario: a rung-2 entity with the same distance event is demoted sooner
    Given inst-b is certified at rung 2 with a 42i distance of 70
    When a scope compliance failure adds 8 distance (total: 78)
    Then inst-b remains at rung 2 (78 < ceiling of 80)
    When a second failure adds 8 more (total: 86)
    Then inst-b is demoted to rung 1 (86 > 80)
    And the contrast illustrates why higher rungs are more resilient to access costs
