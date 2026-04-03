# features/targets.feature
# Canon record — last audited: 2026-03-26
# Step definitions: features/step_definitions/targets_steps.go

@targets @core
Feature: Target Definition and Validation
  As an operator configuring ocd-smoke-alarm
  I want target definitions to be validated before any probes are scheduled
  So that misconfigured targets are caught at startup rather than discovered at runtime

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/sample.yaml" exists

  # ── required fields ────────────────────────────────────────────────────────

  Scenario: Target with all required fields passes validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation succeeds

  Scenario: Target with empty id fails validation
    Given a target with id "", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "target id is required"

  Scenario: Target with empty protocol fails validation
    Given a target with id "t1", protocol "", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "protocol is required"

  Scenario: Target with empty transport fails validation
    Given a target with id "t1", protocol "mcp", transport "", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "transport is required"

  # ── endpoint rules ─────────────────────────────────────────────────────────

  Scenario: HTTP transport target with empty endpoint fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint ""
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "endpoint is required"

  Scenario: HTTP transport target with malformed endpoint fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "not-a-url"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "endpoint is invalid"

  Scenario: SSE transport target with non-http endpoint fails validation
    Given a target with id "t1", protocol "mcp", transport "sse", endpoint "ws://example.com/events"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "sse endpoint must use http/https"

  Scenario: SSE transport target with https endpoint passes validation
    Given a target with id "t1", protocol "mcp", transport "sse", endpoint "https://example.com/sse"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation succeeds

  Scenario: Stdio transport target with empty command fails validation
    Given a target with id "t1", protocol "mcp", transport "stdio", and no command set
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "stdio command is required"

  Scenario: Stdio transport target with command passes validation
    Given a target with id "t1", protocol "mcp", transport "stdio", and command "npx"
    And the target has interval 1s, timeout 1s, and retries 0
    When I validate the target
    Then validation succeeds

  # ── check policy rules ─────────────────────────────────────────────────────

  Scenario: Target with zero timeout fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 0s, and retries 0
    When I validate the target
    Then validation fails with message "timeout must be > 0"

  Scenario: Target with zero interval fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 0s, timeout 1s, and retries 0
    When I validate the target
    Then validation fails with message "interval must be > 0"

  Scenario: Target with negative retries fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries -1
    When I validate the target
    Then validation fails with message "retries must be >= 0"

  Scenario Outline: Unsupported handshake_profile fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And the target handshake_profile is "<profile>"
    When I validate the target
    Then validation fails with message "handshake_profile"

    Examples:
      | profile  |
      | extreme  |
      | advanced |
      | ""       |

  Scenario Outline: Supported handshake_profile passes validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And the target handshake_profile is "<profile>"
    When I validate the target
    Then validation succeeds

    Examples:
      | profile |
      | none    |
      | base    |
      | strict  |

  Scenario: required_methods with an empty entry fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And required_methods includes an empty string
    When I validate the target
    Then validation fails with message "required_methods"

  # ── HURL test rules ────────────────────────────────────────────────────────

  Scenario: HURL test with no file or endpoint fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And a HURL test named "bad-hurl" with no file and no endpoint
    When I validate the target
    Then validation fails with message "hurl_tests[0] requires either file or endpoint"

  Scenario: HURL test with both file and endpoint fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And a HURL test named "conflict" with file "test.hurl" and endpoint "https://example.com/hurl"
    When I validate the target
    Then validation fails with message "hurl_tests[0] file and endpoint are mutually exclusive"

  Scenario: HURL test with unsupported HTTP method fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And a HURL test named "bad-method" with endpoint "https://example.com/hurl" and method "BREW"
    When I validate the target
    Then validation fails with message "method"

  Scenario: HURL test with file only passes validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And a HURL test named "file-test" with file "checks/health.hurl" and no endpoint
    When I validate the target
    Then validation succeeds

  # ── auth config rules ──────────────────────────────────────────────────────

  Scenario: Bearer auth without secret_ref fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And auth type is "bearer" with no secret_ref
    When I validate the target
    Then validation fails with message "bearer auth requires secret_ref"

  Scenario: Bearer auth with secret_ref passes validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And auth type is "bearer" with secret_ref "my-token"
    When I validate the target
    Then validation succeeds

  Scenario: API key auth without key_name or secret_ref fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And auth type is "apikey" with no key_name and no secret_ref
    When I validate the target
    Then validation fails with message "apikey auth requires key_name and secret_ref"

  Scenario: OAuth auth without client_id and token_url fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And auth type is "oauth" with no client_id and no token_url
    When I validate the target
    Then validation fails with message "oauth auth requires at least client_id and token_url"

  Scenario: Unknown auth type fails validation
    Given a target with id "t1", protocol "mcp", transport "http", endpoint "https://example.com/mcp"
    And the target has interval 1s, timeout 1s, and retries 0
    And auth type is "magic"
    When I validate the target
    Then validation fails with message "auth type"

  # ── CheckResult semantics ──────────────────────────────────────────────────

  Scenario Outline: IsFailure returns true for failure states
    Given a check result with state "<state>"
    When I call IsFailure on the result
    Then IsFailure returns <expected>

    Examples:
      | state      | expected |
      | unhealthy  | true     |
      | outage     | true     |
      | regression | true     |
      | healthy    | false    |
      | degraded   | false    |
      | unknown    | false    |

  Scenario Outline: IsEscalated returns true when alert-worthy conditions are met
    Given a check result with state "<state>", severity "<severity>", regression flag <regression>
    When I call IsEscalated on the result
    Then IsEscalated returns <expected>

    Examples:
      | state      | severity | regression | expected |
      | healthy    | critical | false      | true     |
      | regression | info     | false      | true     |
      | healthy    | info     | true       | true     |
      | healthy    | info     | false      | false    |
      | degraded   | warn     | false      | false    |

  # ── type vocabulary (canonical values) ────────────────────────────────────

  Scenario Outline: Protocol values are recognized
    Given a target with protocol "<protocol>"
    Then the target protocol is accepted as a known protocol

    Examples:
      | protocol  |
      | mcp       |
      | acp       |
      | http      |
      | tcp       |

  Scenario Outline: Transport values are recognized
    Given a target with transport "<transport>"
    Then the target transport is accepted as a known transport

    Examples:
      | transport |
      | http      |
      | websocket |
      | sse       |
      | stdio     |
      | grpc      |
      | tcp       |

  Scenario Outline: FailureClass values are recognized
    Given a check result with failure class "<class>"
    Then the failure class is accepted as a known failure class

    Examples:
      | class        |
      | none         |
      | network      |
      | timeout      |
      | auth         |
      | protocol     |
      | config       |
      | tls          |
      | rate_limited |
      | unknown      |
