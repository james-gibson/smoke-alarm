# features/route-rotation.feature
# Next-steps record — a route prefix belongs to an epoch, not to the process
# Dependency: features/honeypot-nodes.feature (the route is the honeypot surface)
# Dependency: features/trust-rungs.feature   (rung gates who may hold a route)
# Step definitions: features/step_definitions/route_rotation_steps.go (to be created)
#
# A tool proxy used to draw one 32-bit prefix at startup and keep it forever. That
# priced guessing (entropy) and priced nothing else: a prefix observed once — a
# proxy log, a cached capability list, a hop that read the body, a pre-stamped
# skill artifact — stayed a valid route for the life of the process. Replay was
# unbounded because nothing expired.
#
# The prefix is now derived from (instance nonce, epoch) instead of drawn. The
# entropy is unchanged for a caller who does not hold the nonce, so enumeration
# still costs what it cost. What changes is that a listed prefix names one epoch,
# and the epoch closes.
#
# The rotation is pre-planned and unspoken: both sides derive the same prefix from
# the same schedule, so the route moves without anyone announcing where it went.
# The mirror node (james-gibson/lab-safety#9) is what later *notifies* neighbors;
# it is not what makes the rotation happen, and the trail has to hold without it.
#
# Two readings must never collapse into one:
#
#   stale-but-honest — a prefix the instance retired, or a stamp it issued and
#                      burned. The client is behind the rotation. A refresh
#                      problem, priced at the rotation tier (2).
#
#   enumerating      — a prefix that matches no epoch of this instance. A guess,
#                      priced at the boundary tier (16) or the fuzz tier (8),
#                      depending on shape.
#
# If those two shared a reason string or a distance cost, rotation itself would
# become the attack surface it was introduced to close: an honest client would
# look like a prober, and a prober could hide in the honest traffic.

@rotation @optional
Feature: Route rotation — a listed prefix expires with its epoch
  A route prefix is a capability for one epoch, not an identity for one process.

  # ── the substitution ───────────────────────────────────────────────────────

  Rule: The prefix is derived, not drawn
    A drawn prefix cannot expire without being replaced by another draw, which
    makes "the route moved" and "the route was re-rolled" the same event. Deriving
    it from (nonce, epoch) makes the route a schedule instead of a state.

    Scenario: Two nodes with the same nonce derive the same prefix at the same epoch
      Given instance nonces "n" and "n"
      When both derive the prefix for epoch 4
      Then the two prefixes are identical

    Scenario: Two nodes with different nonces derive different prefixes at the same epoch
      Given instance nonces "n1" and "n2"
      When both derive the prefix for epoch 4
      Then the two prefixes differ

    Scenario: Consecutive epochs of one instance do not share a prefix
      Given an instance deriving prefixes for epochs 4 and 5
      Then the two prefixes differ

    Scenario: The prefix keeps the width the drawn prefix had
      Given a derived prefix
      Then it carries 32 bits of entropy between its markers

  # ── the schedule is shared, the rotation is silent ──────────────────────────

  Rule: Both sides compute the route without being told
    The schedule is (nonce, base epoch, origin, period). A client holding it knows
    where the route is at any instant, which is what lets rotation be an unspoken
    event rather than a notification the cluster has to survive.

    Scenario: A client holding the schedule names the current route
      Given a client holding the schedule for a route
      When the client derives the prefix for the epoch the clock says is current
      Then the derived name resolves against the route

    Scenario: A client computes the route it will need before the rotation happens
      Given a client holding the schedule
      When the client derives the prefix for the next epoch
      Then the prefix differs from the current one
      And the route has not moved yet

  # ── expiry ─────────────────────────────────────────────────────────────────

  Rule: A prefix stops being the route when its epoch closes
    Expiry is dated at the boundary the epoch crossed, not at the moment the node
    noticed. A lazy rotation stamped with "now" would hand a leaked prefix one
    free call after every boundary, because the retire and the call would always
    happen together.

    Scenario: A leaked prefix works only inside its epoch
      Given a route at epoch 0
      And a prefix observed by a hop that read the body
      When the epoch closes and the notification budget passes
      Then a call carrying the observed prefix is refused
      And the reason is "stale-route-epoch"

    Scenario: A prefix from several epochs back is refused at the boundary it crossed
      Given a route at epoch 0 with no traffic
      When three epochs pass before the next call arrives
      Then the retirement is dated at the end of epoch 0
      And the call is refused as "stale-route-epoch"

    Scenario: A client one epoch behind, inside the grace window, still resolves
      Given a route at epoch 0
      And a prefix issued at epoch 0
      When the epoch closes and the call arrives inside the notification budget
      Then the call resolves
      And nothing is recorded against the client

  # ── attribution ────────────────────────────────────────────────────────────

  Rule: Staleness and enumeration do not share a price
    Rotation is a cost of the safe trail, not an attack signal. It has to stay
    below the tier a guess occupies, or rotation punishes the clients that follow
    it.

    Scenario: A retired-epoch prefix costs the rotation tier
      Given a route that has rotated away from an epoch
      When a call arrives carrying that epoch's prefix, past grace
      Then the reason is "stale-route-epoch"
      And the call is not counted as fuzz
      And it costs 2

    Scenario: A prefix matching no epoch of the instance costs the enumeration tier
      Given a route
      When a call arrives carrying a prefix-shaped name the instance never derived
      Then the reason is "fuzzed-or-wrong-prefix"
      And it costs 8

    Scenario: An issued stamp stays attributable after its rotation
      Given a skill stamped at epoch 0
      When the epoch closes and the stamp is presented past grace
      Then the reason is "burned-skill-route"
      And the call is credited to the skill it was stamped for

  # ── skew ───────────────────────────────────────────────────────────────────

  Rule: A fast clock is skew, not a probe
    The safe map is only a safe map if the agents holding it are not punished for
    ordinary clock drift. A prefix one epoch ahead is the same refresh problem
    with the sign flipped.

    Scenario: A prefix one epoch ahead is attributed as skew
      Given a route at epoch 3
      When a call arrives carrying the prefix for epoch 4
      Then the reason is "ahead-route-epoch"
      And the call is not counted as fuzz

    Scenario: A prefix well ahead of the schedule is not a clock
      Given a route at epoch 3
      When a call arrives carrying the prefix for epoch 6
      Then the reason is "fuzzed-or-wrong-prefix"

  # ── bounds ─────────────────────────────────────────────────────────────────

  Rule: Only a bounded number of closed epochs stay attributable
    The ledger of retired prefixes is what lets a stale client be recognised. It
    is bounded on purpose: a prefix from outside it is no longer a capability the
    instance vouches for, so it belongs in the enumeration tier.

    Scenario: A prefix older than the retained window reads as a guess
      Given a route that has retired more epochs than the window holds
      When a call arrives carrying the oldest retired prefix
      Then the reason is "fuzzed-or-wrong-prefix"

  # ── the clock ──────────────────────────────────────────────────────────────

  Rule: Rotation happens on traffic, not on a timer nobody owns
    The rotation clock is lazy: a route with no traffic has no route at risk, and
    the first call after an epoch closes is the moment it is. This keeps the
    schedule in one place (the schedule) instead of in a goroutine that has to
    agree with it.

    Scenario: The first call after an epoch closes moves the route first
      Given a route whose epoch closed while it was idle
      When a call arrives
      Then the route rotates before the call is judged
      And the call is judged against the new epoch

    Scenario: An explicit rotation does not fight the schedule
      Given a route rotated on instruction from an observer
      When half an epoch passes
      Then the route is still in the epoch the rotation opened
      And the schedule describes that epoch, not the one the clock predicted

    Scenario: Metrics carry the epoch the route is speaking from
      Given a route that has rotated
      When its metrics are read
      Then the epoch and the rotation count are reported

  # ── the ledger the epoch cannot carry ──────────────────────────────────────

  Rule: Adoption carries the stamps, not just the epoch
    A standby promoted into a route must attribute the stamps its predecessor
    issued, or a failover turns every client holding an old stamp into a fuzz
    positive.

    Scenario: A promoted standby attributes the predecessor's stamps
      Given a route whose predecessor issued stamps at epoch 1 and 2
      When a standby adopts the ledger
      Then a call carrying either stamp is attributed
      And it is not counted as fuzz

  # ── replay is still open ───────────────────────────────────────────────────

  Rule: Rotation bounds replay, it does not detect it
    Rotation makes a leaked route worth one epoch instead of one process
    lifetime. It does not make a stamp one-time: a stamp redeemed twice inside its
    own epoch still resolves twice. That is a separate accounting problem, and
    this feature states the bound rather than claiming the detection.

    Scenario: A stamp redeemed twice inside one epoch still resolves twice
      Given a stamp issued at the current epoch
      When it is presented twice before the epoch closes
      Then both calls resolve
