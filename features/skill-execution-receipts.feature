# features/skill-execution-receipts.feature
# Conceptual unification: isotopes are receipts of skill execution on remote targets.
# Dependency: isotope-construction.feature  (the receipt format)
# Dependency: isotope-transit.feature       (receipt flows through /status)
# Dependency: skill-certification.feature   (skills have rungs; receipts certify them)
# Dependency: federation.feature            (remote agents reach targets via mesh)
#
# The isotope ID construction algorithm already encodes this:
#
#   isotope_id = base64url( SHA256( feature_id || ":" || SHA256(payload) || ":" || nonce ) )
#
#   feature_id  = the Gherkin scenario that describes the skill being executed
#   payload     = the skill's execution result (the probe/test response body)
#   nonce       = per-execution random value; each receipt is unique
#
# Seeing an isotope in transit is proof that:
#   - the skill named by feature_id was executed
#   - it produced the payload whose hash is committed in the ID
#   - it ran at least once (nonce ensures non-replay)
#
# Remote agents or smoke-alarms executing skills against other smoke-alarms
# generate isotopes as a natural consequence of execution. The receipt travels
# back through the target's /status response — no separate reporting step exists.
# The isotope IS the receipt. Transit IS the proof.

@skill-receipts @isotope @core
Feature: Isotopes as Skill Execution Receipts
  As a smoke-alarm observing isotopes transiting through health-check responses
  I want each isotope to be interpretable as a receipt of skill execution
  So that observing an isotope in /status proves a named skill ran on that target
  without requiring a separate reporting or confirmation channel

  # ── receipt structure ───────────────────────────────────────────────────────

  Scenario: an isotope committed from skill execution encodes the skill's Gherkin scenario
    Given a skill whose Gherkin scenario is "adhd/open-the-pickle-jar"
    When the skill executes against a target and produces a result payload
    Then the isotope is constructed as: base64url( SHA256( "adhd/open-the-pickle-jar" || ":" || SHA256(payload) || ":" || nonce ) )
    And observing this isotope proves "adhd/open-the-pickle-jar" ran and produced that payload

  Scenario: the receipt commits to the result without exposing it
    Given isotope "iso-abc-001" was produced by skill "adhd/open-the-pickle-jar"
    When an observer sees "iso-abc-001" in transit
    Then the observer knows the skill ran and produced a specific result
    And the observer cannot recover the result payload from the isotope ID alone
    And the commit is one-way: the payload cannot be reverse-engineered

  Scenario: two executions of the same skill with the same result produce different receipts
    Given skill "adhd/open-the-pickle-jar" executes twice against the same target
    And both executions produce identical result payloads
    When two isotopes are constructed (different nonces)
    Then the two isotope IDs are not equal
    And each receipt is independently unique
    And neither receipt can be replayed as proof of a third execution

  Scenario: two executions producing different results produce different receipts
    Given skill "adhd/open-the-pickle-jar" executes twice with different result payloads
    Then the two isotope IDs are not equal
    And the difference in receipts is evidence of non-determinism in the skill's target

  # ── receipt transit ─────────────────────────────────────────────────────────
  # The receipt travels back through the target's /status response automatically.
  # No separate reporting is needed — the probe pipeline is the delivery channel.

  Scenario: a skill executed by a remote agent produces a receipt that transits through /status
    Given remote agent "adhd-headless" executes skill "adhd/mdns-discovery" against target "t1"
    And the skill produces a receipt isotope "iso-skill-001"
    When the target "t1" carries "iso-skill-001" in its next probe response
    And the smoke-alarm polls "t1"
    Then /status shows isotope_id "iso-skill-001" for "t1"
    And the transit is recorded: skill "adhd/mdns-discovery" executed on "t1"
    And no separate reporting call is required from the agent

  Scenario: multiple remote agents executing the same skill on the same target produce distinct receipts in transit
    Given "adhd-a" executes "adhd/mdns-discovery" on "t1" producing receipt "iso-a-001"
    And "adhd-b" executes "adhd/mdns-discovery" on "t1" producing receipt "iso-b-001"
    When both receipts transit through "t1"'s /status in separate probe cycles
    Then both transit events are recorded independently
    And each receipt identifies a distinct execution by a distinct agent

  # ── smoke-alarms testing other smoke-alarms ─────────────────────────────────
  # A smoke-alarm can execute skills against peer smoke-alarm instances.
  # The receipts that result certify that the peer supports those skills.
  # This is cross-alarm certification via the receipt mechanism.

  Scenario: smoke-alarm A executes a skill against smoke-alarm B and receives a receipt
    Given smoke-alarm A has skill "adhd/chaos-isotopes" in its skill registry
    When A executes "adhd/chaos-isotopes" against B's probe endpoint
    And B's probe response carries the resulting receipt isotope
    Then A records the transit: skill "adhd/chaos-isotopes" certified on B
    And B's skill rung for "adhd/chaos-isotopes" advances based on the receipt

  Scenario: a receipt from a cross-alarm skill execution advances the target's skill rung
    Given "adhd/open-the-pickle-jar" on smoke-alarm B is at rung 1 (existence only)
    When smoke-alarm A executes "adhd/open-the-pickle-jar" against B and observes the receipt in transit
    Then A records: skill executed, receipt verified against feature binding
    And the skill's certified rung on B advances toward rung 3

  Scenario: smoke-alarm B's /status carrying a receipt proves B processed the skill correctly
    Given A sends a skill execution request to B
    When B's /status carries the receipt isotope in the next probe cycle
    Then A has proof that B received the request, processed it, and produced a committed result
    And no acknowledgement from B is required — the receipt in /status is the acknowledgement

  # ── receipt verification ────────────────────────────────────────────────────
  # A receipt can be verified if the verifier knows the feature_id, the payload,
  # and the nonce. If any component differs, verification fails.
  # This is identical to isotope verification from isotope-construction.feature.

  Scenario: a receipt verifies correctly when feature_id, payload, and nonce all match
    Given receipt "iso-skill-001" was constructed from:
      | feature_id | adhd/open-the-pickle-jar |
      | payload    | <execution result body>  |
      | nonce      | <random value>           |
    When verification is attempted with those three values
    Then verification succeeds
    And the receipt is confirmed as genuine proof of that skill execution

  Scenario: a forged receipt fails verification
    Given an observer constructs a fake receipt claiming "adhd/open-the-pickle-jar" ran
    But the observer does not know the nonce used in the original execution
    When the fake receipt is submitted as proof
    Then verification fails
    And the receipt is rejected as unverifiable

  Scenario: a receipt with a mismatched feature_id fails verification
    Given receipt "iso-skill-001" was constructed with feature_id "adhd/open-the-pickle-jar"
    When verification is attempted claiming feature_id "adhd/mdns-discovery"
    Then verification fails
    And the mismatch indicates the receipt came from a different skill than claimed

  # ── chain of receipts as certification evidence ─────────────────────────────
  # A sequence of receipts for the same skill on the same target builds
  # cumulative certification evidence. Each receipt is independent proof
  # of one execution. The accumulation reduces 42i distance.

  Scenario: each verified receipt for a skill reduces its 42i distance
    Given "adhd/open-the-pickle-jar" on target B has 42i distance 60
    When a new receipt for that skill is observed in transit and verified
    Then the 42i distance decreases
    And if distance falls below the rung-3 threshold, the skill advances to rung 3

  Scenario: a receipt for a skill that has never been seen before creates a new transit record
    Given "adhd/warroom-builder" has never been observed on any target
    When its first receipt appears in /status for target "t1"
    Then a new transit record is created for skill "adhd/warroom-builder" on "t1"
    And the skill enters the certification pipeline at rung 1 (existence via receipt)

  Scenario: a receipt from a rung-0 (uncertified) agent is recorded but not used to advance rung
    Given the remote agent that executed the skill is certified at rung 0 by the smoke-alarm
    When the receipt transits through /status
    Then the transit is recorded
    And the receipt does not advance the skill's certified rung
    And a note is attached: "receipt from uncertified agent: rung advancement withheld"

  # ── receipts in chaos scenarios ─────────────────────────────────────────────
  # Chaos skill executions also produce receipts. The receipt from a chaos skill
  # is exactly the chaos isotope: it proves the chaos scenario ran and produced
  # a specific (failing) result. The chaos window registration is separate, but
  # the receipt is what transits through /status during the failure.

  Scenario: a chaos skill execution produces a receipt that transits as the chaos isotope
    Given skill "adhd/chaos-isotopes" is executed as a planned failure scenario
    And the skill result payload is the probe failure response
    When the isotope is constructed from feature_id + failure_payload + nonce
    Then the resulting isotope is the chaos receipt
    And when it transits through /status with a failing probe, it is classified as "registered-chaos"
    And the chaos window registration refers to this isotope by its receipt ID

  Scenario: observing a chaos receipt in /status proves the chaos skill ran intentionally
    Given receipt "iso-chaos-001" is in transit with isotope_classification "registered-chaos"
    When the smoke-alarm processes the transit
    Then it records: chaos skill executed intentionally on this target
    And the receipt distinguishes this failure from an unregistered (unexpected) failure

  # ── the elegant statement ────────────────────────────────────────────────────
  # Remote agents or smoke-alarms executing skills against targets generate
  # isotopes as a natural consequence of execution.
  # The receipt travels back through /status automatically.
  # Observing the receipt in transit proves the skill ran.
  # There is no separate confirmation channel.
  # The isotope IS the receipt. Transit IS the proof.

  Scenario: the receipt model requires no separate confirmation protocol
    Given any skill execution on any remote target
    When the receipt isotope appears in that target's /status
    Then proof of execution is established
    And no callback, acknowledgement, or reporting endpoint is required
    And the certifying smoke-alarm receives the proof through its normal polling loop
