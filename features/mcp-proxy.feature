# features/mcp-proxy.feature
# Next-steps record — MCP JSON-RPC message proxying across federation mesh
# Dependency: federated-skills.feature (skill routing infrastructure)
# Dependency: federation.feature (slot election, InstanceRecord, membership)
# Step definitions: features/step_definitions/mcp_proxy_steps.go (to be created)
#
# The smoke-alarm acts as an MCP proxy: when a JSON-RPC call arrives for a
# federated skill ([instance-id] skill-name), it is forwarded to the instance
# that hosts that skill. The response is proxied back to the caller transparently.
#
# This enables AI agents and test harnesses to address any skill in the mesh
# from a single entry point without knowing which instance hosts it.

@mcp-proxy @optional
Feature: MCP JSON-RPC Proxying Across Federation Mesh
  As an AI agent or test harness connected to one instance in a federation mesh
  I want to invoke skills hosted on any peer instance via a single JSON-RPC endpoint
  So that I do not need to know the topology of the mesh or maintain multiple connections

  # The demo cluster is the first production use of this capability.
  # The smoke-alarm hosting MCP on its federation slot acts as the entry point.
  # Routing traces from federated-skills.feature carry through the proxy layer.

  Background:
    Given a federation mesh is running with at least 2 instances
    And each instance has at least one MCP skill installed
    And the introducer instance is serving MCP on its HTTP port

  # ── transparent proxying ────────────────────────────────────────────────────

  Scenario: a JSON-RPC call for a local skill is handled directly without proxying
    Given the introducer "inst-a" has skill "open-the-pickle-jar" installed locally
    When a JSON-RPC call arrives for method "open-the-pickle-jar"
    Then inst-a handles the call directly
    And no outbound HTTP request is sent to a peer

  Scenario: a JSON-RPC call for a federated skill is forwarded to the hosting instance
    Given instance "inst-b" (id "bbbb000000000002") hosts skill "start-here"
    When a JSON-RPC call arrives at inst-a for method "[bbbb000000000002] start-here"
    Then inst-a sends a POST to inst-b's MCP endpoint with the JSON-RPC payload
    And the response from inst-b is returned to the original caller unchanged

  Scenario: proxy request carries the original JSON-RPC id so the caller can correlate it
    Given a JSON-RPC call arrives with id "req-42"
    When the call is proxied to inst-b
    Then the response returned to the caller has id "req-42"

  Scenario: proxy adds a routing trace header before forwarding
    Given a JSON-RPC call arrives at inst-a for a skill on inst-b
    When inst-a proxies the call
    Then the forwarded HTTP request includes an "X-Routing-Trace" header
    And the header value contains inst-a's instance ID

  Scenario: proxy rejects a call when the routing trace already contains the local instance ID
    Given a JSON-RPC call arrives at inst-a with an "X-Routing-Trace" that already includes inst-a's ID
    When the call is processed
    Then the call is rejected with a JSON-RPC error code -32600
    And the error message contains "routing cycle detected"

  # ── multi-hop proxying ──────────────────────────────────────────────────────

  Scenario: a skill on a third instance is reachable via two-hop proxy
    Given a 3-instance mesh: inst-a → inst-b → inst-c (linear, no cycle)
    And inst-c hosts skill "demo-capabilities"
    When a JSON-RPC call arrives at inst-a for "[inst-c-id] demo-capabilities"
    Then inst-a routes the call to inst-b (which knows inst-c from its registry)
    And inst-b forwards the call to inst-c
    And the final routing trace on arrival at inst-c contains all three instance IDs

  Scenario: proxy selects a direct route when a peer is reachable and skip intermediate hops
    Given inst-a has inst-c in its own registry directly
    When inst-a proxies a call for a skill on inst-c
    Then the call is sent directly to inst-c without routing through inst-b

  # ── error handling ──────────────────────────────────────────────────────────

  Scenario: proxy returns a JSON-RPC error when the target instance is unreachable
    Given inst-b is in the registry but its HTTP port is not responding
    When a JSON-RPC call for a skill on inst-b arrives at inst-a
    Then inst-a returns a JSON-RPC error with code -32001
    And the error message contains "peer unreachable" and inst-b's instance ID

  Scenario: proxy returns a JSON-RPC error when the federated skill ID is not in the registry
    When a JSON-RPC call arrives for method "[cccc000000000003] no-such-skill"
    Then the error code is -32601 (method not found)
    And the error message contains "[cccc000000000003] no-such-skill"

  Scenario: proxy propagates JSON-RPC errors from the target instance back to the caller
    Given inst-b handles the proxied call and returns a JSON-RPC error with code -32000
    When inst-a receives inst-b's error response
    Then the same error code and message are returned to the original caller

  # ── demo cluster entry point ────────────────────────────────────────────────
  # In the demo cluster, the "alarm-b" instance acts as the MCP gateway.
  # ADHD connects to alarm-b and can reach skills on any peer via the proxy.

  Scenario: ADHD MCP client reaches a skill on alarm-a by connecting to alarm-b
    Given ADHD is configured with alarm-b as its MCP endpoint
    And alarm-a hosts skill "adhd.chaos.register-window"
    And alarm-b's registry includes alarm-a
    When ADHD invokes "[alarm-a-id] adhd.chaos.register-window"
    Then alarm-b proxies the call to alarm-a
    And the tool result is returned to ADHD

  Scenario: adhd.chaos.register-window is reachable via proxy without reconfiguring ADHD
    Given ADHD is connected to alarm-b and alarm-a joins the federation later
    When alarm-a announces itself with skill "adhd.chaos.register-window"
    And ADHD invokes "[alarm-a-id] adhd.chaos.register-window"
    Then the proxy routes the call correctly to alarm-a
    And no ADHD reconfiguration is required

  # ── method namespacing in tools/list ───────────────────────────────────────

  Scenario: tools/list response includes federated skill methods with instance-id prefix
    Given inst-a's local skills are ["open-the-pickle-jar"]
    And inst-b (id "bbbb000000000002") has skills ["start-here"]
    When a JSON-RPC tools/list call arrives at inst-a
    Then the response includes "open-the-pickle-jar" (local)
    And the response includes "[bbbb000000000002] start-here" (federated)

  Scenario: a client can discover the full skill mesh from a single tools/list call
    Given a 3-instance mesh where each instance has 1 skill
    When a tools/list call arrives at the introducer
    Then the response contains 3 methods total (1 local + 2 federated)
