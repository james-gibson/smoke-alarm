# features/mdns.feature
# Canon record — last audited: 2026-03-26
# Exercises: internal/mdns — mDNS service advertisement via grandcat/zeroconf
# Source: internal/mdns/advertiser.go
# Step definitions: features/step_definitions/mdns_steps.go

@mdns @core
Feature: mDNS Service Advertisement
  As an operator running ocd-smoke-alarm on a local network
  I want the process to advertise itself via mDNS
  So that Tuner instances can discover it passively without manual configuration

  Background:
    Given the ocd-smoke-alarm binary is installed

  # ── defaults ────────────────────────────────────────────────────────────────

  Scenario: domain defaults to "local" when not specified in options
    Given an Advertiser created with service type "_smoke-alarm._tcp" and no domain
    Then the advertiser domain is "local"

  Scenario: ServiceID returns a formatted service identifier
    Given an Advertiser with service type "_smoke-alarm._tcp", domain "local", and port 9090
    Then ServiceID returns "_smoke-alarm._tcp.local:9090"

  # ── ParsePort ───────────────────────────────────────────────────────────────

  Scenario: ParsePort extracts the port from a valid host:port address
    When ParsePort is called with "127.0.0.1:9090"
    Then the returned port is 9090

  Scenario: ParsePort returns 0 for an address with no port
    When ParsePort is called with "127.0.0.1"
    Then the returned port is 0

  Scenario: ParsePort returns 0 for an empty string
    When ParsePort is called with ""
    Then the returned port is 0

  # ── Start ───────────────────────────────────────────────────────────────────

  Scenario: Start registers the service on the configured port
    Given an Advertiser with service type "_smoke-alarm._tcp" and port 9090
    When Start is called with a live context
    Then the service is registered on port 9090
    And the registration uses service type "_smoke-alarm._tcp"

  Scenario: Start includes TXT records in the registration
    Given an Advertiser with TXT record "version" set to "1.0"
    When Start is called with a live context
    Then the registration includes TXT record "version=1.0"

  Scenario: Start returns an error when zeroconf registration fails
    Given a zeroconf registration error is injected
    When Start is called with a live context
    Then Start returns a non-nil error
    And the error message contains "mdns: register"

  # ── Shutdown ─────────────────────────────────────────────────────────────────

  Scenario: Shutdown deregisters the service
    Given an Advertiser that has been started
    When Shutdown is called
    Then the zeroconf server is shut down
    And subsequent Start calls return a fresh registration

  Scenario: Shutdown is safe to call before Start
    Given an Advertiser that has not been started
    When Shutdown is called
    Then no panic occurs

  # ── context cancellation ────────────────────────────────────────────────────

  Scenario: cancelling the context stops the advertisement
    Given an Advertiser that has been started with a cancellable context
    When the context is canceled
    Then the zeroconf server is shut down

  # ── config integration ──────────────────────────────────────────────────────

  Scenario: advertiser is not started when tuner.advertise is false
    Given a valid config file "configs/sample.yaml" exists
    And the config has tuner.advertise set to false
    When ocd-smoke-alarm starts
    Then the mDNS advertiser is not started

  Scenario: advertiser port matches the actual bound port after BindWithRetry
    Given the configured listen port is occupied by another process
    And BindWithRetry is called with 10 retries
    When the mDNS advertiser is started with the actual bound address
    Then the advertiser registers the service on the actual bound port
    And the advertiser does not register on the configured port
