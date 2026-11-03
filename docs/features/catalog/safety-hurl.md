---
title: Pre-Protocol HURL Safety Scanner
description: "The internal/safety Scanner: two HURL modes (endpoint, file), registration validation, failure classification, and report aggregation."
tags:
  - smoke-alarm
  - feature
  - ops-safety
  - safety
  - hurl
  - gherkin
source: smoke-alarm/features/safety-hurl.feature
subsystem: ops-safety
gherkin_tags:
  - "@safety-hurl"
  - "@core"
scenario_count: 19
---
> **Source:** `features/safety-hurl.feature` · **Subsystem:** [Operations & Safety](../subsystems/ops-safety.md) · **Index:** [Feature catalog](../index.md)

Pre-protocol HTTP safety checks (`internal/safety/hurl.go`) run before MCP/ACP handshakes, so targets with failing health endpoints never progress to protocol validation. The Scanner has two mutually exclusive modes per test entry: **endpoint-based** (in-process HTTP check; `Endpoint` set, `File` empty) and **file-based** (external `hurl` binary with `--test <file>`; `File` set, `Endpoint` empty). Mixing file+endpoint on one entry is a config error.

## Scenario map

**Registration** — valid endpoint- or file-based tests register; both file+endpoint → "mutually exclusive"; neither → "requires either file or endpoint"; empty name → "name is required"; unsupported method (e.g. CONNECT) → "unsupported"; unregister removes the tests.

**Endpoint-based mode** — passes on 2xx (report `passed` 1, `has_blocking` false); fails on 4xx (503 → `has_blocking` true); unreachable → fail class `network`; defaults to GET; uses the target endpoint when the test's endpoint is unset; attaches custom headers.

**File-based mode** — invokes `hurl --test <file>`; injects `--variable ENDPOINT=<target endpoint>`; injects `--variable TEST_ENDPOINT=<test endpoint>` when set; passes on exit 0; fails on non-zero (`has_blocking` true); missing binary → fail class `config`.

**Failure class mapping** — error fragments map to classes: `timeout`→timeout, `tls`/`x509`→tls, `connection refused`/`no such host`→network.

**Report aggregation** — counts passed/failed/skipped independently and sets `has_blocking`; `any_tests_seen` is false when none are registered; context cancellation marks remaining tests "skip" with `has_blocking` false.

## Cross-references

- [Stdio MCP Transport](stdio-mcp.md) — `hurl_tests` on stdio targets
- [Hosted Server](hosted-server.md) — `hurl_tests` as a pre-handshake gate
- [SSE Transport](sse-transport.md) — `hurl_tests` on SSE targets
- [Operations & Safety subsystem](../subsystems/ops-safety.md)