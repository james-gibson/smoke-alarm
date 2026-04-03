# features/safety-hurl.feature
# Canon record — last audited: 2026-03-25
# Exercises: internal/safety Scanner — two HURL modes, failure classification, registration
# Code: internal/safety/hurl.go
# Step definitions: features/step_definitions/safety_hurl_steps.go
# see: features/stdio-mcp.feature (hurl_tests on stdio targets)
# see: features/hosted-server.feature (hurl_tests as pre-handshake gate)
# see: features/sse-transport.feature (hurl_tests on SSE targets)
#
# NOTE: The Scanner has TWO distinct execution modes determined by the hurl_test field:
# 1. endpoint-based: in-process HTTP check (ht.Endpoint set, ht.File empty)
# 2. file-based: external `hurl` binary invoked with --test <file> (ht.File set, ht.Endpoint empty)
# These are mutually exclusive per test entry. Mixing file+endpoint on one entry is a config error.

@safety-hurl @core
Feature: Pre-Protocol HURL Safety Scanner
  As an operator
  I want pre-protocol HTTP safety checks to run before MCP/ACP handshakes
  So that targets with failing health endpoints never progress to protocol validation

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a HURL safety scanner is initialized

  # ── registration ──────────────────────────────────────────────────────────

  Scenario: registering a target with valid endpoint-based hurl_tests succeeds
    Given a target "test-target" with hurl_test name "health-check" and endpoint "http://localhost/healthz"
    When the target is registered with the scanner
    Then the scanner reports "test-target" has registered tests

  Scenario: registering a target with a valid file-based hurl_test succeeds
    Given a target "test-target" with hurl_test name "file-check" and file "tests/hurl/health.hurl"
    When the target is registered with the scanner
    Then the scanner reports "test-target" has registered tests

  Scenario: registering a hurl_test with both file and endpoint set returns a config error
    Given a target "bad-target" with hurl_test name "mixed" and both file and endpoint set
    When the target is registered with the scanner
    Then a config error is returned containing "mutually exclusive"

  Scenario: registering a hurl_test with neither file nor endpoint set returns a config error
    Given a target "bad-target" with hurl_test name "empty" with no file or endpoint
    When the target is registered with the scanner
    Then a config error is returned containing "requires either file or endpoint"

  Scenario: registering a hurl_test with an empty name returns a config error
    Given a target "bad-target" with a hurl_test that has an empty name
    When the target is registered with the scanner
    Then a config error is returned containing "name is required"

  Scenario: registering a hurl_test with an unsupported HTTP method returns a config error
    Given a target "bad-target" with hurl_test method "CONNECT"
    When the target is registered with the scanner
    Then a config error is returned containing "unsupported"

  Scenario: unregistering a target removes its tests from the scanner
    Given the target "test-target" is registered with hurl tests
    When the target is unregistered from the scanner
    Then the scanner reports "test-target" has no registered tests

  # ── endpoint-based mode ───────────────────────────────────────────────────

  Scenario: endpoint-based test passes when the endpoint returns 2xx
    Given a hurl_test endpoint "http://localhost/healthz" returns status code 200
    When the scanner runs tests for the target
    Then the test outcome is "pass"
    And the report "passed" count is 1
    And "has_blocking" is false

  Scenario: endpoint-based test fails when the endpoint returns 4xx
    Given a hurl_test endpoint "http://localhost/healthz" returns status code 503
    When the scanner runs tests for the target
    Then the test outcome is "fail"
    And "has_blocking" is true

  Scenario: endpoint-based test fails when the endpoint is unreachable
    Given a hurl_test endpoint "http://localhost:9999/healthz" is unreachable
    When the scanner runs tests for the target
    Then the test outcome is "fail"
    And the failure class is "network"

  Scenario: endpoint-based test uses GET by default when method is not specified
    Given a hurl_test with no method field
    When the scanner runs the test
    Then the outbound request uses method "GET"

  Scenario: endpoint-based test uses the target endpoint when hurl_test endpoint is not set
    Given a target with endpoint "http://localhost:8080/mcp" and a hurl_test with no endpoint field
    When the scanner runs the test
    Then the outbound request is sent to "http://localhost:8080/mcp"

  Scenario: endpoint-based test attaches custom headers from the hurl_test headers map
    Given a hurl_test with header "X-Custom-Header" set to "test-value"
    When the scanner runs the test
    Then the outbound request contains header "X-Custom-Header: test-value"

  # ── file-based mode ───────────────────────────────────────────────────────

  Scenario: file-based test invokes the hurl binary with --test flag
    Given a hurl_test with file "tests/hurl/health.hurl"
    And the "hurl" binary is available on PATH
    When the scanner runs the test
    Then the command executed is "hurl --test tests/hurl/health.hurl"

  Scenario: file-based test injects ENDPOINT variable from the target endpoint
    Given a target with endpoint "https://example.com/mcp"
    And a hurl_test with file "tests/hurl/probe.hurl"
    When the scanner runs the test
    Then the command includes "--variable ENDPOINT=https://example.com/mcp"

  Scenario: file-based test injects TEST_ENDPOINT variable when hurl_test endpoint is set
    Given a hurl_test with file "tests/hurl/probe.hurl" and endpoint "https://example.com/healthz"
    When the scanner runs the test
    Then the command includes "--variable TEST_ENDPOINT=https://example.com/healthz"

  Scenario: file-based test passes when hurl exits with code 0
    Given the "hurl" binary exits with code 0 for "tests/hurl/health.hurl"
    When the scanner runs the test
    Then the test outcome is "pass"

  Scenario: file-based test fails when hurl exits with a non-zero code
    Given the "hurl" binary exits with code 1 for "tests/hurl/health.hurl"
    When the scanner runs the test
    Then the test outcome is "fail"
    And "has_blocking" is true

  Scenario: file-based test returns failure class "config" when hurl binary is not found
    Given the "hurl" binary is not available on PATH
    When the scanner runs a file-based test
    Then the test outcome is "fail"
    And the failure class is "config"

  # ── failure class mapping ─────────────────────────────────────────────────

  Scenario Outline: endpoint error messages are mapped to failure classes
    Given the hurl_test endpoint error message contains "<error-fragment>"
    When the scanner classifies the failure
    Then the failure class is "<failure-class>"

    Examples:
      | error-fragment    | failure-class |
      | timeout           | timeout       |
      | tls               | tls           |
      | x509              | tls           |
      | connection refused| network       |
      | no such host      | network       |

  # ── report aggregation ────────────────────────────────────────────────────

  Scenario: report counts passed, failed, and skipped tests independently
    Given a target with 3 hurl_tests: 2 passing endpoints and 1 failing endpoint
    When the scanner runs all tests for the target
    Then the report "passed" count is 2
    And the report "failed" count is 1
    And the report "skipped" count is 0
    And "has_blocking" is true

  Scenario: report has any_tests_seen false when no tests are registered for the target
    Given a target with no hurl_tests registered
    When the scanner runs tests for the target
    Then "any_tests_seen" is false
    And "has_blocking" is false

  Scenario: context cancellation marks remaining tests as skipped
    Given a target with 3 hurl_tests registered
    And the context is canceled before any tests run
    When the scanner runs tests for the target
    Then each test outcome is "skip"
    And "has_blocking" is false
