# ocd-smoke-alarm — Continuous Health Monitor

Continuous health monitoring for HTTP endpoints. Polls configured targets on an interval, tracks state transitions (healthy → degraded → unhealthy → outage), detects regressions, and emits structured alerts.

`ocd-smoke-alarm` runs either:

- **Foreground** with a Bubble Tea TUI for live triage
- **Background** as a low-overhead monitoring service (managed by `lezz.go`)

## In the Lab

ocd-smoke-alarm is part of a cluster of tools that share a common lab environment. See [lab-safety](https://github.com/james-gibson/lab-safety) for the full ecosystem map.

Key integrations:
- **lezz.go** — starts and manages ocd-smoke-alarm as a LaunchAgent; `lezz demo` runs two instances that monitor each other
- **adhd** — polls ocd-smoke-alarm `/status` and SSE streams; registers with `/isotope/register` on startup
- **isotope** — shared library used for `VerifyTrust`, wire types (`Registration`, `Record`), and the 42i capability lattice

### Isotope Endpoints

ocd-smoke-alarm is the isotope registry for the cluster:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/isotope/register` | POST | Register an agent; returns a `Record` with a trust rung |
| `/isotope/list` | GET | List all registered isotopes and their trust rungs |

Trust rungs are assigned by `VerifyTrust`: rung 6 if both the lezz cluster registry (`:19100/cluster`) and mDNS (`_lezz-demo._tcp`) confirm the registrant, rung 4 if registry only, rung 2 if self-registered.

### Other Endpoints

| Endpoint | Description |
|----------|-------------|
| `/healthz` | Liveness probe |
| `/readyz` | Readiness probe |
| `/status` | Full component health and target states |
| `/.well-known/smoke-alarm.json` | Machine-readable service descriptor |

Multiple instances monitor each other, creating a Byzantine-tolerant consensus view of cluster health.

---

---

## Status

This repository is under active implementation with a task-driven delivery plan:

- `PLAN.md`
- `TASKS.md`
- `AGENT_HANDOFF.md`

Use those as the canonical execution and handoff references.

---

## Requirements

## OS

- macOS (supported target)
- Linux (supported target)

## Toolchain

- Go `1.22+`
- Git
- (Recommended) `make`
- (Recommended) `golangci-lint`

## Verify prerequisites

```sh
go version
git --version
```

You should see a Go version `>= 1.22`.

---

## Install

## Option A: build from clone (recommended for development)

```sh
git clone <YOUR_REPO_URL> ocd-smoke-alarm
cd ocd-smoke-alarm
go mod tidy
go build ./cmd/ocd-smoke-alarm
```

## Option B: install binary with Go tooling

```sh
go install ./cmd/ocd-smoke-alarm
```

If installing from a remote module path:

```sh
go install <MODULE_PATH>/cmd/ocd-smoke-alarm@latest
```

---

## Quickstart

## 1) Create a config file

Use `configs/` as the default location for environment configs.

Example minimal config (illustrative):

```yaml
version: 1
runtime:
  mode: background
  health_addr: "localhost:8088"

monitor:
  interval: "30s"
  aggressive_regression: true

targets:
  - id: "local-mcp"
    kind: "mcp"
    endpoint: "http://localhost:9000"
    auth:
      type: "none"
```

## 2) Run in foreground (TUI)

```sh
go run ./cmd/ocd-smoke-alarm --mode=foreground --config=configs/dev.yaml --health-addr=localhost:8088
```

## 3) Run in background

```sh
go run ./cmd/ocd-smoke-alarm --mode=background --config=configs/dev.yaml --health-addr=localhost:8088
```

## 4) Health check

```sh
curl -fsS http://localhost:8088/healthz
curl -fsS http://localhost:8088/readyz
```

## 5) Local-first startup profile (recommended for first run)

The default sample includes cloud and auth placeholder targets so you can see full feature coverage, but local-first validation is faster and less noisy.

Recommended first-run flow:

1. Start only local services you actually have (for example a local MCP proxy and optional OTel collector).
2. Disable placeholder cloud targets in your config (or set `enabled: false` for those targets).
3. Run `serve` or `check` and confirm you get clean health/readiness on local targets first.
4. Re-enable remote/cloud targets gradually once credentials and endpoints are real.

Example local-first command:

```sh
go run ./cmd/ocd-smoke-alarm serve --config=configs/dev.local.yaml --mode=foreground
```

Example one-shot validation against the same local profile:

```sh
go run ./cmd/ocd-smoke-alarm check --config=configs/dev.local.yaml
```

To register stop-gap HURL safety tests before protocol smoke checks, add entries under each target:

```yaml
check:
  interval: "10s"
  timeout: "3s"
  retries: 1
  hurl_tests:
    - name: "health-gate"
      endpoint: "http://localhost:3000/healthz"
      method: "GET"
```

You can also register file-based HURL tests:

```yaml
check:
  hurl_tests:
    - name: "mcp-contract-precheck"
      file: "./tests/safety/mcp_health.hurl"
```

To register authoritative remote `llms.txt` URIs and auto-discover MCP/ACP candidate targets from them:

```yaml
discovery:
  enabled: true
  llms_txt:
    enabled: true
    remote_uris:
      - "https://example.com/llms.txt"
      - "https://docs.example.org/llms.txt"
    fetch_timeout: "5s"
    require_https: true
    auto_register_as_targets: true
```

When `auto_register_as_targets` is enabled, the smoke alarm parses remote `llms.txt` link sections and creates protocol candidates automatically when links/descriptions indicate MCP or ACP endpoints.

Auto-discovery currently supports:
- static configured targets
- environment-derived targets
- local proxy scan targets
- authoritative remote `llms.txt` candidates

If `discovery.llms_txt.auto_register_oauth` is enabled, OAuth scaffolding is also generated for candidates that include OAuth/OIDC/auth hints in link names, notes, or sections (for example token endpoint assumptions and starter scopes).

---


## Expected failures with placeholder sample targets

If you run with `configs/sample.yaml` unchanged, some failures are expected:

- Placeholder cloud MCP/ACP endpoints (for example `example.com`) are not real service endpoints.
- Placeholder keystore references (for example `keychain://...`) may not exist yet on your machine.
- Optional local dependencies (like OTel collector at `localhost:13133`) may be offline.

Typical first-run outputs may include:

- auth/config-related warnings for missing secrets
- network/probe failures for unreachable placeholder endpoints
- unhealthy/degraded status for targets you have not provisioned yet

This is normal for an uncustomized sample config and does not mean the monitor core is broken.

## Core Behaviors

- **Auto-discovery:** scans known MCP/ACP config inputs for targets, agents, skills, and rules across local and remote contexts; when the working directory contains Dockerfiles or docker-compose assets the monitor automatically appends the official Docker `llms.txt` feed so container-centric services stay discoverable.
- **Remote `llms.txt` ingestion:** fetches registered authoritative `llms.txt` URIs and extracts MCP/ACP candidate links for initial target registration.
- **Deterministic federation orchestration:** the first local process to claim the configured slot range becomes the introducer, subsequent instances announce and heartbeat through it, and the shared health/TUI view stays stable even across rapid restarts.
- **Client-like validation:** validates targets using config semantics close to real clients (including command-based stdio targets like `npx` and `uvx`).
- **Pre-protocol safety stage (HURL):** runs optional registered HURL stop-gap checks before deeper protocol validation.
- **Protocol handshake smoke checks:** validates MCP/ACP base and strict handshake method paths after safety checks pass.
- **Transport coverage (SSE-first):** supports local/remote `stdio`, `http`, `https`, `sse`, `ws`, and `wss` flows, with SSE preferred for HTTP streaming integrations.
- **Embedded hosted services:** the smoke alarm can host its own MCP/ACP (and optional A2A-style) endpoints for local validation, regression testing, and bootstrap workflows.
- **A2A compatibility path:** A2A-style services can be monitored as HTTP/SSE/WebSocket targets and safety-gated until deeper A2A protocol checks are enabled.
- **Baseline + regression:** tracks known-good target state and marks failures as regressions when a previously healthy target fails.
- **Aggressive escalation:** outages/regressions are elevated quickly by policy.
- **Meta-config generation:** produces partial but valid snippets for user review and copy/paste into `mcp-add`.

## Protocol Validation Stages

For each enabled target, the monitor follows this order:

1. **Config validation** (required fields, transport/auth consistency, command settings for stdio targets)
2. **Optional remote `llms.txt` enrichment** (discover and auto-register MCP/ACP candidates from registered authoritative `llms.txt` URIs)
3. **Safety scan** (optional HURL tests registered under `check.hurl_tests`)
4. **Protocol smoke** (handshake profile execution on configured transport)
5. **Classification** (`healthy`, `degraded`, `unhealthy`, `outage`, `regression`)

If stage 3 (HURL safety) fails, stage 4 is blocked for that target in that cycle, and the failure is surfaced as a pre-protocol safety failure.

### SSE-First Usage Guidance

At this stage, prefer SSE over WebSocket for remote streaming-style MCP/ACP integrations unless a provider explicitly requires WS/WSS. This keeps transport behavior simpler while still allowing progressive handshake and safety-gate validation.

Recommended target transport order for new integrations:
1. `stdio` for local command-hosted development servers
2. `http`/`https` for request-response JSON-RPC endpoints
3. `sse` for streaming over HTTP when available
4. `ws`/`wss` only when required by the upstream service

### Embedded Hosted MCP/ACP Service Mode

The smoke alarm can run an embedded hosted service mode to expose MCP/ACP endpoints directly from the monitor process. This is useful for:
- local end-to-end smoke validation without external dependencies
- agent bootstrap and handshake regression checks
- proving client configs against a known-good in-process service

Hosted service endpoints are configured under `hosted` in config:
- `hosted.enabled`
- `hosted.listen_addr`
- `hosted.transports` (`http`, `sse`)
- `hosted.protocols` (`mcp`, `acp`, optional `a2a`)
- `hosted.endpoints.mcp`, `hosted.endpoints.acp`, `hosted.endpoints.a2a`

When enabled, you can monitor these hosted endpoints as normal targets and use them as baseline controls during outages or config migrations.

### Handshake Matrix Options

Configure per target with:
- `check.handshake_profile`: `none | base | strict`
- `check.required_methods`: explicit ordered method list (overrides profile defaults)

Default profile behavior:
- **MCP + stdio/http/https**
  - `base`: `initialize`
  - `strict`: `initialize`, `tools/list`, `resources/list`
- **ACP + stdio/http/https**
  - `base`: `initialize`
  - `strict`: `initialize`, `session/setup`, `prompt/turn`
- **HTTP/HTTPS non-MCP/ACP**
  - status-based health check (`GET`) unless explicit method requirements are configured
- **WS/WSS**
  - transport reachability check by default, with protocol extension path for deeper checks
- **A2A-style services**
  - currently validated through transport + safety-gate path; use explicit required methods as the interim smoke policy where supported

### Local vs Remote Target Scope

- **Local services:** command-spawned stdio servers (`npx`, `uvx`, binaries), local proxies, localhost HTTP/WS endpoints
- **Remote services:** HTTPS/WSS MCP or ACP endpoints, plus remote authoritative `llms.txt` discovery candidates
- **Mixed fleets:** fully supported; relationship view in the TUI highlights protocol/transport/state mismatches and missing links.

---

## Notifications

The monitor supports a notification abstraction so regressions/outages can be surfaced via:

- structured logs
- local OS notification backends (where configured)

---

## Agent-Only Operations (No Operator Intervention)

This project is intended to be managed by a remote agent for updates.

## Required lifecycle contract

On every update, the agent must perform:

1. **Stop** active runtime
2. **Deploy** newest code/binary/config
3. **Restart** service
4. **Verify** health endpoints and core probes
5. **Rollback** if verification fails

## Recommended update sequence

```sh
# 1) stop
pkill -f ocd-smoke-alarm || true

# 2) update
git fetch --all
git reset --hard origin/main
go mod tidy
go build -o ./bin/ocd-smoke-alarm ./cmd/ocd-smoke-alarm

# 3) restart
./bin/ocd-smoke-alarm --mode=background --config=configs/prod.yaml --health-addr=localhost:8088 &

# 4) verify
curl -fsS http://localhost:8088/healthz
curl -fsS http://localhost:8088/readyz
```

## Update safety rules for agents

- Never hot-swap binaries without stopping the active runtime first.
- Always perform post-restart health verification.
- Treat `healthy -> failing` after update as a regression incident.
- Keep updates atomic and rollback-capable.
- Do not require manual prompts or operator shell intervention.

---

## Quality Gates (Format, Lint, Test)

Run these before/after every change:

```sh
go fmt ./...
go test ./...
```

Or run the consolidated project gate:

```sh
make ci
```

If `golangci-lint` is installed:

```sh
golangci-lint run
```

If integration tests are split by package/tag:

```sh
go test ./tests/integration/... -v
```

---

## Integration Testing Goals

Integration tests are expected to cover at least:

1. healthy target remains healthy
2. healthy target becomes unavailable -> regression event
3. auth failures are classified correctly
4. outage escalation is triggered under aggressive policy
5. stdio MCP/ACP handshake smoke checks pass for valid command targets
6. registered HURL pre-protocol safety checks pass/fail as expected and can block protocol stage
7. agent update stop/deploy/restart/verify path
8. generated meta-config validates/round-trips
9. embedded hosted MCP/ACP HTTP JSON-RPC endpoints respond correctly
10. embedded hosted SSE endpoints emit valid `text/event-stream` responses
11. dynamic config persistence writes uniquely identified JSON/Markdown artifacts
12. OAuth mock redirect allow/fail modes produce deterministic callback outcomes

## Command Verification Expectations

Before handoff, verify these command paths:

1. **Config validation path**
   - `go run ./cmd/ocd-smoke-alarm validate --config=<sample>`
   - Expected: `status: valid` and no runtime side effects.

2. **Discovery + dynamic persistence path**
   - `go run ./cmd/ocd-smoke-alarm discover --config=<sample> --json=false`
   - Expected:
     - discovered records listed
     - dynamic config artifact list printed when `dynamic_config.enabled=true`
     - artifact paths written under configured `dynamic_config.directory`.

3. **One-shot smoke path**
   - `go run ./cmd/ocd-smoke-alarm check --config=<sample>`
   - Expected:
     - healthy samples return success (`exit 0`)
     - intentionally failing samples return failure (`exit != 0`) with actionable reason.

4. **Serve path**
   - `go run ./cmd/ocd-smoke-alarm serve --config=<sample> --mode=background`
   - Expected:
     - startup JSON log line appears
     - health endpoint responds if enabled
     - no unexpected bind/path errors for chosen sample ports.

5. **Log behavior expectation**
   - Presence of logs for startup, discovery persistence, and critical regressions.
   - Absence of noisy repeated logs for non-escalated healthy cycles.

---

## Minimal Footprint Guidance

- Keep polling interval sane (avoid tight loops).
- Reuse HTTP clients and set strict timeouts.
- Use bounded concurrency (worker pools / semaphores).
- Keep history buffers bounded.
- Disable optional UI/verbose output in background mode.

Resource budget verification command (constrained runtime smoke check):

```sh
GOMAXPROCS=1 GOMEMLIMIT=128MiB go test ./tests/integration/... -v
```

Use this command in agent-driven update validation to confirm integration checks still pass under a tighter CPU/memory envelope.

---

## Security Notes

- Prefer OS keystore/keychain-backed secrets where possible.
- Never log raw credentials or tokens.
- Redact auth details in status and error output.
- Validate update source integrity before deployment.

---

## Sample Config Catalog (Direct Review + Quick Testing)

All sample configs live in `configs/samples/` and are intended for editor-side inspection and immediate smoke testing:

- `configs/samples/stdio-mcp-strict.yaml`
  - strict stdio MCP handshake path (`initialize`, `tools/list`, `resources/list`).
- `configs/samples/sse-remote-mixed.yaml`
  - SSE-first remote profile with mixed MCP/ACP targets.
- `configs/samples/hosted-mcp-acp.yaml`
  - embedded hosted MCP/ACP endpoints plus self-monitoring.
- `configs/samples/llmstxt-auto-discovery.yaml`
  - authoritative remote `llms.txt` discovery with target auto-registration.
- `configs/samples/oauth-mock-allow.yaml`
  - OAuth callback mock in `allow` mode.
- `configs/samples/oauth-mock-fail.yaml`
  - OAuth callback mock in `fail` mode.
- `demo` subcommand workflow
  - foreground exploratory mode with hosted MCP/ACP enabled, OAuth mock redirect enabled, discovery on, and dynamic config persistence on.

## Quick Command Matrix (Samples)

Run from repository root:

- Validate every sample:

```sh
for f in configs/samples/*.yaml; do
  go run ./cmd/ocd-smoke-alarm validate --config="$f"
done
```

- Stdio MCP strict smoke:

```sh
go run ./cmd/ocd-smoke-alarm check --config=configs/samples/stdio-mcp-strict.yaml
```

- Hosted MCP/ACP local profile:

```sh
go run ./cmd/ocd-smoke-alarm serve --config=configs/samples/hosted-mcp-acp.yaml --mode=foreground
```

- llms.txt discovery + dynamic config generation:

```sh
go run ./cmd/ocd-smoke-alarm discover --config=configs/samples/llmstxt-auto-discovery.yaml --json=false
```

- SSE-first remote mixed profile (one-shot):

```sh
go run ./cmd/ocd-smoke-alarm check --config=configs/samples/sse-remote-mixed.yaml
```

- OAuth allow/fail scenarios (requires environment variable):

```sh
export OAUTH_CLIENT_SECRET=your-real-client-secret
go run ./cmd/ocd-smoke-alarm check --config=configs/samples/oauth-mock-allow.yaml
go run ./cmd/ocd-smoke-alarm check --config=configs/samples/oauth-mock-fail.yaml
```

- Persist discovery-derived dynamic configs as unique JSON/Markdown artifacts:

```sh
go run ./cmd/ocd-smoke-alarm dynamic-config persist --config=configs/samples/llmstxt-auto-discovery.yaml
go run ./cmd/ocd-smoke-alarm dynamic-config list --config=configs/samples/llmstxt-auto-discovery.yaml
go run ./cmd/ocd-smoke-alarm dynamic-config index --config=configs/samples/llmstxt-auto-discovery.yaml
```

- Launch demo/exploration mode for hosted-agent workflow:

```sh
export OAUTH_CLIENT_SECRET=your-real-client-secret
go run ./cmd/ocd-smoke-alarm demo --config=configs/samples/hosted-mcp-acp.yaml
```

In demo mode, instruct external agents/clients to connect to hosted endpoints (for example `/mcp`, `/acp`, and SSE paths) and observe:
- topology/relationship pane updates in TUI
- demo state-machine visualization transitions
- hosted request/event telemetry from `/hosted/status` and `/hosted/events`
- persisted dynamic configs for later static serving (nginx, etc.)

## OAuth Environment Expectations

OAuth samples are intentionally configured to require real OAuth shape fields:
- `client_id`
- `token_url`
- `redirect_url`
- `callback_id`
- `secret_ref`

For quick local testing, OAuth sample configs use:

- `secret_ref: env://OAUTH_CLIENT_SECRET`
- mock redirect callback endpoint in allow/fail mode

So before running OAuth samples, set:

```sh
export OAUTH_CLIENT_SECRET=your-real-client-secret
```

If this variable is missing, OAuth targets are expected to appear degraded/unhealthy due missing auth material.

## New Subcommands

### `dynamic-config` subcommand

Use this for user-created and discovery-derived config lifecycle operations:

- `dynamic-config persist`
  - runs discovery and writes uniquely identified artifacts to configured dynamic config directory
- `dynamic-config list`
  - lists persisted dynamic config files
- `dynamic-config show --id=<id> --format=json|markdown`
  - prints one persisted config artifact
- `dynamic-config index`
  - writes an index markdown file for static serving workflows

### `demo` subcommand

Use this for exploratory sessions and hosted-agent validation:

- forces foreground/TUI experience
- enables hosted MCP/ACP service behavior for interactive validation
- enables OAuth mock redirect endpoint for deterministic callback behavior
- enables discovery + dynamic config persistence
- designed for “agent connects to me” workflows where you verify that inbound protocol traffic and state transitions match expectations in real time

## Dynamic Config Persistence Workflow

When `dynamic_config.enabled=true`:

1. Discovery records are converted into uniquely identified dynamic configs.
2. Each dynamic config is written as:
   - JSON (`.json`)
   - Markdown (`.md`)
3. IDs are uniqueness-safe when `require_unique_ids=true`.
4. Output is written to `dynamic_config.directory`.
5. Files are generated for later static serving (for example through nginx) using `dynamic_config.serve_base_url`.

This allows users to build new configs from discovery output, save them as portable artifacts, and review/publish them in JSON or Markdown form.

## Documentation Map

- `PLAN.md` — implementation roadmap and phase exits
- `TASKS.md` — prioritized backlog with dependencies
- `AGENT_HANDOFF.md` — token-constrained continuation guide
- `docs/` — operational and troubleshooting docs (as implementation expands)
- `configs/samples/` — ready-to-run profile catalog for local/remote/hosted/OAuth workflows

---

## Contribution Flow (Agent-Optimized)

1. Pick a task ID from `TASKS.md`
2. Implement smallest viable change
3. Add/update integration coverage
4. Run format/lint/test gates
5. Update task status and handoff notes

Keep each change narrow and rollback-friendly.

---

## See Also

- [lab-safety — full ecosystem overview](https://github.com/james-gibson/lab-safety)
- [adhd dashboard](https://github.com/james-gibson/adhd)
- [lezz.go daemon manager](https://github.com/james-gibson/lezz.go)
- [isotope protocol library](https://github.com/james-gibson/isotope)
