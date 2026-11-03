---
title: OAuth Mock Redirect Endpoint
description: "The mock OAuth redirect endpoint in allow and fail modes: startup, callback handling, auth-failure classification, HURL preflight, token redaction, and unattended flows."
tags:
  - smoke-alarm
  - feature
  - ops-safety
  - oauth
  - auth
  - gherkin
source: smoke-alarm/features/oauth-mock.feature
subsystem: ops-safety
gherkin_tags:
  - "@oauth-mock"
  - "@optional"
scenario_count: 11
---
> **Source:** `features/oauth-mock.feature` · **Subsystem:** [Operations & Safety](../subsystems/ops-safety.md) · **Index:** [Feature catalog](../index.md)

A local mock OAuth redirect endpoint lets an operator test OAuth-authenticated targets without a real auth server, validating the smoke alarm's failure classification for auth failures before deploying to production.

## Scenario map

**Mock endpoint startup** — binds `localhost:28877` and serves `/oauth/callback` when `mock_redirect.enabled` is true; binds nothing when false.

**Allow mode** — returns 200 for any callback; the `oauth-mock-status` target is classified HEALTHY; an ACP target with OAuth auth completes its handshake (redirect handled by the mock) and is HEALTHY.

**Fail mode** — returns non-200 for any callback; an ACP target is classified DEGRADED and an alert is emitted with severity `critical`.

**HURL preflight with OAuth mock** — a `hurl_test` exercises the mock callback endpoint directly (GET to the callback URL), with the result recorded in the probe output.

**Token redaction** — an OAuth client secret referenced via `secret_ref` (`env://OAUTH_CLIENT_SECRET`) is never logged in plaintext; any log referencing it contains `****`.

**Unattended OAuth flows** — when `oauth.unattended.enabled` and a flow flag are true, the flow is attempted without user interaction (allow_device_code, allow_refresh_token, allow_client_credentials).

## Cross-references

- [Discovery via llms.txt](discovery-llmstxt.md) — `auto_register_oauth`
- [SSE Transport](sse-transport.md) — bearer/oauth handling
- [Alert Routing and Severity Policy](alerts.md) — auth-failure alerting and redaction
- [Operations & Safety subsystem](../subsystems/ops-safety.md)