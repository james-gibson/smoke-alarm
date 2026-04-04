# features/mdns-discovery.feature
# Canon record — last audited: 2026-04-03
# Exercises: internal/mdns — mDNS peer browsing for cluster membership
# Source: internal/mdns/browser.go
# Step definitions: features/step_definitions/mdns_steps.go
# see: features/mdns.feature (advertisement — the outbound complement to this browse)
# see: features/routing.feature (routing table populated from discovered peers)

@mdns @core
Feature: mDNS Peer Discovery
  As an ocd-smoke-alarm instance on a local network
  I want to continuously browse for other smoke-alarm instances via mDNS
  So that the cluster routing table stays current without manual configuration

  Background:
    Given the ocd-smoke-alarm binary is installed
    And mDNS peer browsing is enabled in the config

  # ── initial browse ───────────────────────────────────────────────────────────

  Scenario: browser discovers a peer already advertising at startup
    Given a peer is advertising "_smoke-alarm._tcp" on the local network
    When the browser starts
    Then the peer is added to the cluster routing table
    And the peer's health state is "unknown"

  Scenario: browser extracts priority from TXT record
    Given a peer with TXT record "priority=3"
    When the peer is discovered
    Then the peer is assigned priority 3 in the routing table

  Scenario: browser assigns default priority when TXT record omits it
    Given a peer with no "priority" field in its TXT records
    When the peer is discovered
    Then the peer is assigned the default priority of 99

  Scenario: browser extracts independent mcp-priority and acp-priority when declared
    Given a peer with TXT records "mcp-priority=1" and "acp-priority=2"
    When the peer is discovered
    Then the peer's MCP priority is 1
    And the peer's ACP priority is 2

  Scenario: browser discovers multiple peers during initial browse
    Given 3 peers are advertising "_smoke-alarm._tcp" with priorities 1, 2, and 3
    When the browser starts
    Then all 3 peers are in the routing table
    And they are ordered by priority ascending

  # ── continuous browse ────────────────────────────────────────────────────────

  Scenario: browser adds a peer that comes online after startup
    Given the browser has been running for 10 seconds
    And no peer named "smoke-alarm-late" is in the routing table
    When "smoke-alarm-late" announces "_smoke-alarm._tcp"
    Then "smoke-alarm-late" is added to the routing table
    And the routing table update occurs without restarting the browser

  Scenario: browse loop remains open for the lifetime of the process
    Given the browser is started
    When 60 seconds elapse with no peer activity
    Then the browse loop is still active
    And a peer announcing after 60 seconds is still discovered

  Scenario: browse does not close after the first result batch
    Given 2 peers are discovered in the initial browse window
    When the initial browse window closes
    Then the browser continues listening for new announcements
    And a third peer announcing after the window is discovered

  # ── peer departure ───────────────────────────────────────────────────────────

  Scenario: browser removes a peer that deregisters its mDNS record
    Given a peer "smoke-alarm-primary" is in the routing table
    When "smoke-alarm-primary" deregisters its "_smoke-alarm._tcp" record
    Then "smoke-alarm-primary" is removed from the routing table immediately

  Scenario: browser marks a peer unreachable when its mDNS TTL expires without renewal
    Given a peer "smoke-alarm-secondary" is in the routing table
    When the peer's mDNS TTL expires with no renewal
    Then the peer's health state transitions to "unreachable"
    And it is removed from the active routing paths

  # ── context cancellation ─────────────────────────────────────────────────────

  Scenario: cancelling the context stops the browse loop
    Given the browser is running with a cancellable context
    When the context is cancelled
    Then the browse loop exits cleanly
    And no new peers are added after cancellation
