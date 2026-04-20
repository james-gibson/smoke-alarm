# features/oauth-mock.feature
# Canon record — last audited: 2026-03-25
# Exercises: mock OAuth redirect endpoint in allow and fail modes, auth classification
# Config references: configs/samples/oauth-mock-allow.yaml, configs/samples/oauth-mock-fail.yaml
# Step definitions: features/step_definitions/oauth_mock_steps.go
# see: features/discovery-llmstxt.feature (auto_register_oauth), features/sse-transport.feature (bearer/oauth)
@oauth-mock @optional
Feature: OAuth Mock Redirect Endpoint
  As an operator testing OAuth-authenticated targets without a real auth server
  I want ocd-smoke-alarm to run a local mock redirect endpoint that accepts or rejects OAuth callbacks
  So that I can validate the smoke alarm's failure classification for auth failures before deploying to production

  Background:
    Given the ocd-smoke-alarm binary is installed
  # ── mock endpoint startup ─────────────────────────────────────────────────

  Scenario: mock redirect endpoint starts when mock_redirect.enabled is true
    Given a config file "configs/samples/oauth-mock-allow.yaml" with mock_redirect enabled
    When ocd-smoke-alarm starts with that config
    Then a listener is bound on "localhost:28877"
    And the path "/oauth/callback" is served

  Scenario: mock redirect endpoint does not start when mock_redirect.enabled is false
    Given a config file with mock_redirect.enabled set to false
    When ocd-smoke-alarm starts with that config
    Then no listener is bound on the mock redirect address
  # ── allow mode ────────────────────────────────────────────────────────────

  Scenario: allow mode returns 200 for any callback request
    Given the mock redirect endpoint is running in "allow" mode on "localhost:28877"
    When a GET request is sent to "http://localhost:28877/oauth/callback?state=s1&callback_id=cb-1"
    Then the response status code is 200

  Scenario: oauth-mock-status target is classified as HEALTHY in allow mode
    Given the mock redirect endpoint is running in "allow" mode on "localhost:28877"
    And the target "oauth-mock-status" is enabled in config "configs/samples/oauth-mock-allow.yaml"
    When the probe for target "oauth-mock-status" completes
    Then the target "oauth-mock-status" is classified as "HEALTHY"

  Scenario: ACP target with OAuth auth completes handshake in allow mode
    Given the mock redirect endpoint is running in "allow" mode on "localhost:28877"
    And the hosted ACP server is running on "localhost:28191"
    And the target "hosted-acp-oauth-validated" is enabled in config "configs/samples/oauth-mock-allow.yaml"
    When the probe for target "hosted-acp-oauth-validated" completes
    Then the OAuth redirect was handled by the mock endpoint
    And the target "hosted-acp-oauth-validated" is classified as "HEALTHY"
  # ── fail mode ─────────────────────────────────────────────────────────────

  Scenario: fail mode returns a non-200 status for any callback request
    Given the mock redirect endpoint is running in "fail" mode on "localhost:28877"
    When a GET request is sent to "http://localhost:28877/oauth/callback?state=s2&callback_id=cb-fail-1"
    Then the response status code is not 200

  Scenario: ACP target is classified as DEGRADED when OAuth callback fails
    Given the mock redirect endpoint is running in "fail" mode on "localhost:28877"
    And the target "acp-oauth-fail-sample" is enabled in config "configs/samples/oauth-mock-fail.yaml"
    When the probe for target "acp-oauth-fail-sample" runs
    Then the target "acp-oauth-fail-sample" is classified as "DEGRADED"
    And an alert is emitted with severity "critical"
  # ── HURL preflight with OAuth mock ────────────────────────────────────────

  Scenario: HURL test exercises the mock redirect callback endpoint directly
    Given the mock redirect endpoint is running in "fail" mode on "localhost:28877"
    And the target "acp-oauth-fail-sample" has a hurl_test "oauth-mock-redirect-callback-fail"
    When the probe for target "acp-oauth-fail-sample" runs
    Then the HURL test "oauth-mock-redirect-callback-fail" sends a GET to "http://localhost:28877/oauth/callback?state=s2&callback_id=cb-fail-1"
    And the test result is recorded in the probe output
  # ── token redaction ───────────────────────────────────────────────────────

  Scenario: OAuth client secret is not logged in plaintext
    Given the target "acp-oauth-fail-sample" has secret_ref "env://OAUTH_CLIENT_SECRET"
    When the probe for target "acp-oauth-fail-sample" runs
    Then no log line contains the raw value of "OAUTH_CLIENT_SECRET"
    And any log line referencing the secret contains "****"
  # ── unattended OAuth flows ────────────────────────────────────────────────

  Scenario Outline: unattended OAuth flow is attempted when enabled
    Given "oauth.unattended.enabled" is true
    And "oauth.unattended.<flow>" is true
    When an OAuth token is required for a target
    Then the <flow> flow is attempted without user interaction

    Examples:
      | flow                     |
      | allow_device_code        |
      | allow_refresh_token      |
      | allow_client_credentials |
