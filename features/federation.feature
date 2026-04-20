# features/federation.feature
# Canon record — last audited: 2026-03-26
# Exercises: federation subsystem — slot election, registry, server, client, poller
# Code: internal/federation/{slots,registry,server,client,federation}.go
# Step definitions: features/step_definitions/federation_steps.go
# see: features/selfdescription.feature (capabilities.federation field)
# see: features/known-state.feature (target status consumed by poller)
#
# ARCHITECTURE CONSTRAINTS (from CLAUDE.md):
#   - Fan-out is valid; cycles are forbidden
#   - Every routed message must carry a routing trace
#   - Routing logic belongs in internal/routing/ (planned) — NOT in internal/federation/
#
# RESOLVED IMPLEMENTATION GAPS:
#   TF-FED-1 DONE: federation.go fmt.Printf replaced with slog.Debug (2026-03-26)
#   TF-FED-2 DONE: ReportToUpstream marshals body, uses NewRequestWithContext (2026-03-26)
#   TF-FED-3 DONE: configs/samples/federation-mesh.yaml added (2026-03-26)
#   TF-FED-4 DONE: DetectRank/SortEndpoints use net.SplitHostPort (2026-03-26)
#
# Config block (from integration test, not yet in sample configs):
#   federation:
#     enabled: true
#     base_port: <N>
#     max_port: <M>
#     poll_interval: "2s"
#     announce_interval: "1s"
#     heartbeat_interval: "1s"
#     heartbeat_timeout: "3s"
@federation @optional
Feature: Federation Mesh — Slot Election, Membership, and Status Fan-Out
  As an operator running multiple ocd-smoke-alarm instances
  I want them to form a local mesh, elect a single introducer, and exchange membership and target status
  So that I can monitor distributed deployments from a single aggregated view without manual configuration

  Background:
    Given the ocd-smoke-alarm binary is installed
    And federation is enabled in the config
  # ── slot election ─────────────────────────────────────────────────────────
  # First process to bind base_port wins the introducer role.
  # All others claim the next available port and become followers.

  Scenario: the first instance to start claims base_port and becomes the introducer
    Given no other instance is running on the federation port range
    When an instance starts with base_port 5100 and max_port 5103
    Then the instance binds port 5100
    And the instance identity role is "introducer"
    And the identity is persisted to "<state_dir>/federation/identity.json"

  Scenario: a second instance claims the next available port and becomes a follower
    Given an introducer is already bound on port 5100
    When a second instance starts with the same base_port and max_port
    Then the second instance binds port 5100 + 1
    And the second instance identity role is "follower"

  Scenario: ClaimSlot returns ErrNoFreeSlots when all ports in the range are occupied
    Given all ports from base_port to max_port are already bound
    When ClaimSlot is called
    Then the error is "federation: no free local slots available"

  Scenario: ClaimSlot defaults to base_port 5100 and slot count 4 when not configured
    Given a config with no federation.base_port field
    When ClaimSlot is called
    Then ports 5100 through 5103 are the candidates
  # ── instance identity ─────────────────────────────────────────────────────

  Scenario: instance ID is deterministic for the same hostname, service, state_dir, and port
    Given two ClaimSlot calls with identical hostname, service_name, state_dir, and port
    When both instance IDs are computed
    Then both IDs are equal

  Scenario: instance ID differs when the port changes
    Given two ClaimSlot calls with identical hostname and service_name but different ports
    Then the two instance IDs are different

  Scenario: a previous identity is tried first to restore slot stability across restarts
    Given a persisted identity.json exists with port 5100 in the candidate range
    When ClaimSlot is called and port 5100 is free
    Then the instance claims port 5100 before trying other candidates
  # ── slot lock ─────────────────────────────────────────────────────────────

  Scenario: concurrent ClaimSlot calls from different processes do not double-claim a port
    Given two processes attempt ClaimSlot simultaneously on the same port range
    Then exactly one process succeeds as introducer
    And the other process becomes a follower or receives ErrNoFreeSlots

  Scenario: a stale slot lock from a dead process is recovered automatically
    Given a slot.lock file exists containing the PID of a process that is no longer running
    When ClaimSlot is called
    Then the stale lock is removed
    And ClaimSlot proceeds normally
  # ── introducer server ─────────────────────────────────────────────────────

  Scenario: introducer server binds on the claimed listener from SlotClaim
    Given the introducer holds a SlotClaim with listener on port 5100
    When the federation server starts
    Then POST /introductions is served on that port
    And POST /heartbeats is served on that port
    And GET /membership is served on that port

  Scenario: POST /introductions adds the peer to the registry and returns current membership
    Given the federation server is running as introducer
    When a follower POSTs to /introductions with a valid InstanceRecord
    Then the response status code is 200
    And the response body contains "peers"
    And the peer appears in the registry

  Scenario: POST /introductions rejects a request with an empty record.id
    Given the federation server is running as introducer
    When a POST is sent to /introductions with an empty "id" field
    Then the response status code is 400
    And the response body contains "record.id is required"

  Scenario: POST /introductions rejects non-POST methods
    When a GET request is sent to /introductions
    Then the response status code is 405

  Scenario: POST /heartbeats refreshes the peer's LastSeenAt and returns current membership
    Given a follower is already registered with the introducer
    When the follower POSTs to /heartbeats with its InstanceRecord
    Then the response status code is 200
    And the peer's last_seen_at is updated in the registry

  Scenario: GET /membership returns the current registry snapshot
    Given the federation server has 2 registered peers
    When a GET request is sent to /membership
    Then the response status code is 200
    And the response body contains "self"
    And the response body contains "peers"
    And the "peers" array length is 2
  # ── age-out ───────────────────────────────────────────────────────────────

  Scenario: a peer that stops heartbeating is removed after heartbeat_timeout elapses
    Given a follower is registered with the introducer
    And "heartbeat_timeout" is set to "3s"
    When the follower sends no heartbeat for 4 seconds
    Then the follower is removed from the registry
    And the removal reason is "heartbeat_timeout"
    And the removal is reflected in GET /membership

  Scenario: a peer that resumes heartbeating before timeout is not removed
    Given a follower is registered with the introducer
    And "heartbeat_timeout" is set to "3s"
    When the follower sends a heartbeat at 2 seconds
    Then the follower remains in the registry after 4 seconds
  # ── peer cap ──────────────────────────────────────────────────────────────

  Scenario: registry silently drops a new peer when MaxPeers is reached
    Given the registry already contains 10 peers at the MaxPeers limit
    When Upsert is called with a new peer record
    Then the registry peer count does not increase
    And no error is returned

  Scenario: registry ignores Upsert calls for the local instance's own ID
    When Upsert is called with a record whose ID matches the registry's own identity
    Then the registry peer count does not change
    And no event is fired
  # ── follower client ───────────────────────────────────────────────────────

  Scenario: follower client sends an introduction immediately on Start
    Given a follower client is configured with an introducer URL
    When Start is called
    Then a POST to /introductions is sent before the first announce_interval elapses

  Scenario: follower client only sends heartbeats after a successful introduction
    Given a follower client has not yet successfully introduced itself
    When the heartbeat_interval elapses
    Then no POST to /heartbeats is sent

  Scenario: follower client applies the returned membership to its local registry
    Given the introducer's introduction response contains 2 peer records
    When the follower processes the introduction response
    Then both peers are upserted into the follower's local registry
    And the follower's registry snapshot is saved to disk

  Scenario: follower client logs a warning when an introduction request fails
    Given the introducer URL is unreachable
    When the follower attempts to send an introduction
    Then a "warn" log entry is written containing "federation introduction failed"
    And the client retries on the next announce_interval

  Scenario: follower client logs a warning when a heartbeat request fails
    Given the follower is introduced and the introducer becomes unreachable
    When the heartbeat_interval elapses
    Then a "warn" log entry is written containing "federation heartbeat failed"
  # ── registry snapshot persistence ────────────────────────────────────────

  Scenario: registry snapshot is written atomically via temp-file rename
    When SaveSnapshot is called
    Then the file "<state_dir>/federation/registry.json" exists
    And no temporary file remains after the write

  Scenario: registry snapshot contains self, introducer_id, peers, version, and generated_at
    When SaveSnapshot is called
    Then the snapshot JSON contains "self"
    And the snapshot JSON contains "introducer_id"
    And the snapshot JSON contains "peers"
    And the snapshot JSON contains "version"
    And the snapshot JSON contains "generated_at"

  Scenario: registry snapshot version increments on each Upsert or Remove
    Given the registry is at version 5
    When a peer is upserted
    Then the registry version is 5 + 1
  # ── registry events ───────────────────────────────────────────────────────

  Scenario: Upsert fires an EventAdded callback for a new peer
    Given the registry has no peer with id "peer-001"
    When Upsert is called with a record with id "peer-001"
    Then the OnChange callback receives an event with type "added"

  Scenario: Upsert fires an EventUpdated callback for an existing peer
    Given the registry already has a peer with id "peer-001"
    When Upsert is called again with the same id
    Then the OnChange callback receives an event with type "updated"

  Scenario: Remove fires an EventRemoved callback
    Given the registry has a peer with id "peer-001"
    When Remove is called with that id
    Then the OnChange callback receives an event with type "removed"
  # ── poller fan-out ────────────────────────────────────────────────────────
  # Fan-out is valid. Cycles are forbidden. See CLAUDE.md routing constraints.

  Scenario: poller fetches target status from each downstream endpoint
    Given a Poller configured with downstream endpoints ["localhost:8091", "localhost:8092"]
    When a poll cycle runs
    Then GET http://localhost:8091/status is requested
    And GET http://localhost:8092/status is requested

  Scenario: poller namespaces target IDs with the source endpoint
    Given a downstream at "localhost:8091" returns a target with id "mcp-local"
    When a poll cycle runs
    Then the aggregated target ID is "[localhost:8091] mcp-local"

  Scenario: poller reports an error target when a downstream endpoint is unreachable
    Given a downstream endpoint "localhost:9999" is unreachable
    When a poll cycle runs
    Then a target with id "federation-error" is included in the update
    And the target state is "unhealthy"

  Scenario: poller calls the updateFn with all aggregated targets after each poll cycle
    Given a Poller with 2 downstream endpoints each returning 3 targets
    When a poll cycle runs
    Then updateFn is called with 6 targets

  Scenario: SortEndpoints orders by port number ascending
    Given endpoints ["localhost:8093", "localhost:8091", "localhost:8092"]
    When SortEndpoints is called
    Then the result is ["localhost:8091", "localhost:8092", "localhost:8093"]

  Scenario: no cycle exists when a downstream is also a peer of the introducer
    Given instance A is the introducer with downstream [B]
    And instance B is a follower with downstream [C]
    And instance C has no downstream
    When the topology is validated
    Then no routing cycle is detected
    And every message carries a routing trace

  Scenario: a cycle is rejected when a downstream references an ancestor
    Given instance A has downstream [B] and instance B has downstream [A]
    When the topology is validated
    Then a cycle error is returned
  # ── full lifecycle (integration) ──────────────────────────────────────────

  Scenario: three instances form a mesh — introducer plus two followers
    Given no instances are running on the federation port range
    When instance 1 starts with federation enabled
    Then instance 1 binds base_port and becomes the introducer
    When instance 2 starts with the same port range
    Then instance 2 joins as a follower
    And instance 2 appears in GET /membership on the introducer
    When instance 3 starts with the same port range
    Then instance 3 joins as a follower
    And both instance 2 and instance 3 appear in GET /membership

  Scenario: stopping a follower causes it to be removed from the introducer's membership view
    Given a mesh of 3 instances (1 introducer + 2 followers) is running
    And "heartbeat_timeout" is set to "3s"
    When follower instance 2 is stopped
    Then within 5 seconds instance 2 is absent from GET /membership
    And instance 3 remains present in GET /membership
