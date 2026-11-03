# features/skill-pads.feature
# Next-steps record — pads stack, and acceptance is not trust
# Dependency: features/rotation-commit.feature (a commit carries stacks)
# Dependency: features/honeypot-nodes.feature   (the route is the declared path)
# Step definitions: features/step_definitions/skill_pads_steps.go (to be created)
#
# Layered clusters. Each layer is a triple: A is ingress, B is A's backup, C stamps
# pads. A flood means a layer stamps FEWER pads; a later layer prepares context and
# stamps MORE. Pads combine the way a stack of pancakes combines: order is
# information, two identical pads are two entries, and pressure stops the stack
# short rather than removing pads from the middle.
#
# Two decisions are kept apart throughout:
#   accept the request  — admit it into the flow, cheap, unconditional, durable
#   trust the request   — decide it was worth acting on, slower, revisable
# C may durably queue accepted items until a slower method can decide trust.
#
# What the implementation has today: PadStack (order, depth, prefix truncation) and
# PendingQueue (accept now, resolve later). What it does not have is a proof that a
# path was traversed — the mirror verifies declarations about rotations, not receipts
# of passage.

@rotation @pads @optional
Feature: Skill pads — a request accumulates a stack, and trust is decided later

  Rule: Pads stack, they do not set
    A request crossing ingress then a context layer carries pads in that order. The
    order, the repetition and the provenance are all part of the request.

    Scenario: Order changes the request
      Given a request is stamped by ingress then by context
      When it is compared with a request stamped context then ingress
      Then the two stacks differ
      And each pad names its layer, its queue and its position

    Scenario: The same pad twice is not the same as once
      Given a layer stamps the same pad twice
      When the stack is inspected
      Then it has two entries
      And the top entry is the later one

  Rule: A flood shortens the stack
    Under pressure a layer stamps fewer pads. The request is weaker, not malformed:
    every result is a well-formed prefix of what a quiet run would have produced.

    Scenario: A flooded request is a prefix of a quiet one
      Given a quiet run produces a five-pad stack
      When the same request runs under flood
      Then it carries fewer pads
      And each pad it does carry is the same pad at the same position
      And the request is still accepted

  Rule: Accepting is not trusting
    Acceptance admits the request and decides nothing about it. Trust is a separate
    decision, made against a slower method, possibly much later.

    Scenario: An accepted request is queued without a verdict
      Given a request arrives on the ingress layer
      When it is accepted
      Then it is on C's durable queue
      And no verdict has been recorded about it
      And accepting it did not depend on the slow method

    Scenario: Trust is decided when the slower method can decide it
      Given accepted requests are on the durable queue
      When the slower method resolves an item
      Then the item is scored at that moment, not at acceptance
      And a false answer is recorded against the resolver

    Scenario: A compromised resolver cannot refuse acceptance
      Given the slower method is biased
      When requests are accepted
      Then every request is still accepted
      And the bias appears only as untrusted items after resolution

  Rule: A route is a declaration of a path
    A route states where a request should go, not what a caller may do. Capability
    lives in the stack; the route carries the path.

    Scenario: The declaration does not carry capability
      Given a route declares the path for a tool
      When a request carries no pads
      Then the request is still accepted
      And it is weaker for trust, not refused

  Rule: A mirror proves the path was traversed
    The mirror's evidence is that the events actually passed through the declared
    queues in the declared order — a receipt of passage, not a node's announcement
    that a rotation occurred.

    Scenario: A path with a missing hop is unproven
      Given a route declares ingress then context
      And the context layer emitted no pad for a request
      When the mirror checks the traversal
      Then the path is unproven
      And the request remains accepted but untrusted

    Scenario: A declaration matches its traversal
      Given a route declares ingress then context
      And both layers emitted pads in that order
      When the mirror checks the traversal
      Then the path is proven
      And the proof is recorded against the request

  Rule: Layers emit events onto new queues
    A layer is a queue; stamping is an event emitted onto the next queue. The
    topology grows while requests are in flight.

    Scenario: A new layer joins mid-flight
      Given a request is queued on the ingress layer
      When a context layer is added and begins emitting pads
      Then the request's stack grows on the next hop
      And the pads it already carried are unchanged

  Rule: Node tensions rotate into one another
    Each node holds a tension; rotation maps one node's tension onto its neighbour's.
    The symmetry is what makes a rotation verifiable rather than merely announced.

    Scenario: A rotation maps tensions pairwise
      Given nodes A, B and C each hold a tension
      When the cluster rotates
      Then each node's tension maps to the next
      And a node whose tension does not map is the discrepancy
