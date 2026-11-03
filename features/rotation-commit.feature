# features/rotation-commit.feature
# Next-steps record — decomposing the commit as far as the model allows
# Dependency: features/honeypot-nodes.feature (the route is the honeypot surface)
# Dependency: features/trust-rungs.feature   (rung gates who may commit)
# Step definitions: features/step_definitions/rotation_commit_steps.go (to be created)
#
# Topology under test: nodes A and B are redundant stores, rotationally observed by
# node C. C commits the decision; A and B hold it. C is authoritative, so the stores
# are not there to outvote C — they are there to catch C.
#
# The implementation in internal/mcp_faults covers the happy path of that: Commit()
# verifies each store independently, RotationCommit.Check() raises a discrepancy
# against a store's own record, and Commit() is deterministic. The Rules below are
# the parts it does NOT yet cover, written as the smallest behaviours that would
# close each gap. Each Rule names the hole rather than the fix.

@rotation @optional
Feature: Rotation commits — the committer's decision is a bounded, auditable object
  A commit is the smallest unit C can decide. Splitting it exposes which of its
  fields are load-bearing and which are conveniences that only work while nothing
  goes wrong.

  # ── Identity ────────────────────────────────────────────────────────────────

  Rule: A commit is about exactly one route
    A commit that does not carry its route can be replayed against another project's
    route that happens to share a tool name.

    Scenario: A store rejects a commit for another route
      Given stores A and B serve the route "lab/isOdd"
      And node C commits a rotation for the route "lab/isEven"
      When store A checks the commit against its own record
      Then the discrepancy is "committed-for-another-route"
      And the commit is not applied

    Scenario: Route identity includes the grace policy
      Given the cluster grace ceiling changes from 1s to 2s
      When the route id is re-derived
      Then every stamp issued under the old id is unattributable
      And the rotation is a re-identification, not a rotation

  # ── Claimant identity ───────────────────────────────────────────────────────

  Rule: The voice in the commit is the node that was observed
    A claim filed under one node's name but carrying another node's id would let a
    store answer for its peer.

    Scenario: A claim whose NodeID does not match the observation is rejected
      Given node C observed a rotation by node A
      And the claim filed for node A carries NodeID "node-b"
      When C commits
      Then node A is rejected with state "claim-identity-mismatch"
      And node B is not credited with A's rotation

    Scenario: Only registered replicas can be committed
      Given node C observes a rotation by an unregistered node X
      When C commits the route
      Then node X is rejected with state "unauthorized-replica"
      And no grace is derived from X's claim

  # ── Sequence ────────────────────────────────────────────────────────────────

  Rule: Rotations are sequenced, not timestamped
    A store may rotate twice inside one clock tick. Equality on a wall-clock instant
    cannot tell those apart, and a sparse clock cannot either.

    Scenario: Two rotations in one tick are distinguishable
      Given store A rotates twice without the clock advancing
      And node C commits only the second rotation
      When store A checks the commit
      Then the discrepancy is "committed-different-rotation"
      And the commit carries a sequence number, not only an instant

    Scenario: A replayed commit cannot roll a store back
      Given node C commits sequence 7 for a route
      And node C commits sequence 8 for the same route
      When sequence 7 is delivered to store A afterwards
      Then store A rejects it as "stale-sequence"
      And the applied state remains sequence 8

    Scenario: A stale observation set cannot be committed late
      Given node C's observations are older than the freshness bound
      When C commits
      Then the commit is refused with "observations-stale"
      And no rotation is committed for the route

  # ── Grace ───────────────────────────────────────────────────────────────────

  Rule: One store cannot narrow the route's grace
    The commit takes the smallest accepted grace, so a store announcing grace zero
    collapses the window for every client on the route — a denial of service on
    notification, priced as if it were honesty.

    Scenario: A zero-grace claim does not collapse the route's window
      Given stores A and B both verify their rotations
      And node B claims a grace of 0
      When node C commits
      Then the committed grace is at least the cluster floor
      And node B is recorded as having claimed below the floor

  # ── What the commit covers ──────────────────────────────────────────────────

  Rule: The commit covers attribution, not only rotation
    Whether a client is scored as stale (2) or as fuzzing (8) depends on the
    attribution ledger. If the commit covers rotations only, two stores can hold
    divergent ledgers under one commit and raise no discrepancy.

    Scenario: Divergent ledgers are a discrepancy
      Given store A and store B hold the same committed rotation
      And store A knows a burned stamp that store B does not
      When store A checks the commit
      Then the discrepancy is "ledger-diverged"
      And the commit is not considered complete until the ledgers agree

  Rule: A commit needs a quorum of verifying stores
    One accepted claim among three observed stores currently produces a commit
    indistinguishable from unanimity apart from a list nothing reads.

    Scenario: A single verifying store is not a commit
      Given three stores are observed for the route
      And only one store's rotation verifies
      When node C commits
      Then the commit is marked "degraded" rather than committed
      And the decision is escalated instead of applied

  # ── Authenticity and escalation ─────────────────────────────────────────────

  Rule: A store can tell C's commit from a forgery
    "CommittedBy" is a string. Nothing binds the decision to the committer, so any
    node can author a commit that the stores will hold.

    Scenario: A forged commit is refused
      Given a commit claiming CommittedBy "node-c" that node C never produced
      When store A receives it
      Then store A refuses it as "unsigned-commit"
      And store A does not apply it

    Scenario: A discrepancy has a destination
      Given store A raises "committed-different-rotation" against node C
      When the discrepancy is recorded
      Then it is delivered to the node that may arbitrate it
      And it is not silently logged and dropped

  # ── Expiry independent of rotation ──────────────────────────────────────────

  Rule: A skill expires even if nobody rotates
    A stamp is currently retired only by rotation, and nothing in the engine calls
    Rotate(). An issued-but-unused stamp is therefore valid forever.

    Scenario: An unused skill expires on its own clock
      Given a skill was issued and never presented
      And the skill's lifetime has elapsed without any rotation
      When the skill is presented
      Then it is attributable staleness, not a live capability
