@tuner-integration @optional
Feature: Tuner Integration
  As a smoke-alarm instance
  I want to integrate with the Tuner passive observer
  So that operators can monitor my status via Television channels

  Background:
    Given a smoke-alarm configured with tuner integration enabled
    And the hosted server is running

  @config
  Scenario: Tuner config block parsed correctly
    Given a config with tuner.enabled = true
    And tuner.advertise = true
    And tuner.service_type = "_smoke-alarm._tcp"
    Then the config validates successfully
    And tuner integration is active

  @mdns
  Scenario: mDNS advertisement starts on serve
    Given tuner.advertise is true
    When smoke-alarm starts in serve mode
    Then it advertises _smoke-alarm._tcp on the local network
    And TXT records include version and protocol info

  @mdns
  Scenario: mDNS advertisement disabled
    Given tuner.advertise is false
    When smoke-alarm starts in serve mode
    Then no mDNS service is advertised

  @audience
  Scenario: Accept audience metrics via POST
    When a Tuner posts audience data to /tuner/audience
      | channel | count | signal |
      | ntp     | 5     | 0.75   |
    Then the response status is 200
    And the audience metric is stored

  @audience
  Scenario: Get audience metrics via GET
    Given audience metrics have been posted for channels ntp and dns
    When a client requests GET /tuner/audience
    Then the response contains metrics for both channels

  @caller
  Scenario: Caller message received and fan-out
    Given a SSE subscriber on /tuner/caller/ntp/sse
    When a viewer posts a message to /tuner/caller/ntp
    Then the subscriber receives the message via SSE
    And the message includes channel, from, and timestamp

  @caller
  Scenario: Caller message without subscribers
    When a viewer posts a message to /tuner/caller/dns
    And no SSE subscribers exist for dns
    Then the response status is 200
    And the response shows subscribers: 0

  @mcp
  Scenario: MCP tools/list includes tuner tools
    When a client sends tools/list via MCP
    Then the response includes smoke.tuner_list_channels
    And the response includes smoke.tuner_audience
    And the response includes smoke.tuner_caller_messages

  @cli
  Scenario: Tuner status subcommand
    Given a config with tuner integration enabled
    When running ocd-smoke-alarm tuner status --config=...
    Then it displays tuner integration status
    And shows audience and caller hook settings

  @cli
  Scenario: Tuner audience push subcommand
    Given the hosted server is accepting audience metrics
    When running ocd-smoke-alarm tuner audience --channel=test --count=5
    Then the audience metric is pushed to the server

  @events
  Scenario: Tuner events appear in hosted event log
    When audience or caller interactions occur
    Then they appear in GET /hosted/events
    And each event has protocol "tuner"
