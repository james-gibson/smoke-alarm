# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

A minimal-footprint Go monitor for MCP/ACP environments. Runs in foreground (Bubble Tea TUI) or background (daemon) mode. Discovers, validates, and smoke-tests MCP/ACP service configurations, detects regressions from previously-healthy state, and supports unattended agent-managed lifecycle operations.

This is an active prototype validating whether Bubble Tea, MCP, and ACP libraries can discover, inspect, and correct misconfigured MCP/ACP services.

`.rules` is organized as four progressive phases (Orient -> Understand -> Contribute -> Constrain/Extend). Read them in order; you can stop at any phase and still make a safe contribution.

## Build & Test Commands

| Task | Command |
|---|---|
| Full quality gate (fmt+lint+unit+integration+footprint) | `make ci` |
| Format | `make fmt` |
| Lint | `make lint` |
| Unit tests | `make unit` |
| Integration tests | `make integration` |
| Footprint budget checks | `make footprint` |
| Sync dependencies | `make tidy` |
| Run a single test | `go test ./tests/integration/... -run TestSmoke -v` |
| Build binary | `go build -o ./bin/ocd-smoke-alarm ./cmd/ocd-smoke-alarm` |

## Key CLI Subcommands

```sh
go run ./cmd/ocd-smoke-alarm validate --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm check --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm serve --mode=foreground --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm discover --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm demo --config=configs/samples/hosted-mcp-acp.yaml
go run ./cmd/ocd-smoke-alarm dynamic-config persist --config=configs/samples/llmstxt-auto-discovery.yaml
```

## Architecture

### Package Layout

- `cmd/ocd-smoke-alarm/main.go` -- CLI entrypoint, flag parsing, subcommand dispatch
- `internal/engine/` -- probe scheduling, event loop, regression state machine; the core orchestrator
- `internal/knownstate/` -- known-good baseline store for drift/regression detection
- `internal/discovery/` -- MCP/ACP server + agent discovery (static configs + remote llms.txt)
- `internal/dynamicconfig/` -- persisted dynamic config artifact generation (JSON + Markdown)
- `internal/hosted/` -- embedded MCP/ACP HTTP + SSE server for local validation
- `internal/safety/` -- HURL pre-protocol safety checks (runs before protocol handshakes)
- `internal/engine/stdio_prober.go` -- stdio transport probe (npx, uvx command targets)
- `internal/auth/` -- token validity, OAuth flow helpers, mock redirect endpoint
- `internal/health/` -- /healthz, /readyz, /status HTTP endpoints
- `internal/ui/` -- Bubble Tea TUI (Elm Architecture: Init/Update/View)
- `internal/alerts/` -- notification routing and severity policy
- `internal/targets/` -- target registry (config + dynamic)
- `internal/meta/` -- meta-config template generation for mcp-add
- `internal/federation/` -- multi-instance mesh (slot election, registry, introducer/follower protocol, status fan-out)
- `internal/telemetry/` -- OTEL metric export
- `internal/ops/` -- remote stop/update/restart lifecycle orchestration

### Data Flow

1. Load config -> validate schema
2. Discover targets (static + dynamic + llms.txt)
3. Run HURL safety checks (if configured)
4. Protocol smoke handshake (MCP/ACP initialize, tools/list, etc.)
5. Compare against known-good baseline
6. Classify: `HEALTHY` | `DEGRADED` | `REGRESSION` | `OUTAGE`
7. Notify + persist status + expose health API

### Two Distinct UI Concerns

**Agentic UI** (`internal/ui/`): Bubble Tea dashboard rendering to the operator's terminal. Driven by engine's SnapshotProvider. Not visible to MCP/ACP clients. Follows Elm Architecture strictly.

**Dynamic UI via stdio proxy** (`internal/proxy/`, planned): Injects UI into MCP message flow delivered to MCP clients. NOT Bubble Tea. Must produce valid MCP/ACP protocol messages. Must not share state with `internal/ui/`.

When any task says "UI", confirm which of these two is intended before writing code.

## Critical Constraints

- **Reference Conflict Halt Rule**: If implementation would conflict with authoritative docs for Bubble Tea, MCP, or ACP, halt and state the conflict before proceeding. See `.rules` for full details.
- **Bubble Tea**: state mutations only in `Update()`, no side effects in `View()`, async work via `tea.Cmd`
- **MCP/ACP**: do not invent JSON-RPC methods or skip capability handshake steps
- **Routing**: fan-out is valid; cycles are forbidden. Every routed message carries a routing trace. Never put routing logic in `internal/proxy/` -- it belongs in `internal/routing/` (planned).
- **No busy polling** -- all probe loops are timer/event driven
- **Bounded goroutines** -- use worker pools per target group
- **Reuse HTTP clients** with strict timeouts; never create per-request clients

## Code Style

- **Imports**: stdlib first, then external, then internal (`goimports` via `make fmt`)
- **Formatting**: `gofumpt` enforced by lint
- **Naming**: packages lowercase single-word; interfaces PascalCase + `er` suffix; acronyms preserve case (`URL`, `HTTP`)
- **Errors**: wrap with `fmt.Errorf("%w")` for context; use sentinel errors for predictable failure modes
- **Logging**: `log/slog` for structured logging; never log tokens/secrets
- **Structs**: explicit YAML/JSON struct tags; `map[string]any` for dynamic/extensible config fields

## Config

- Schema is versioned -- check `internal/config/config.go` for current version
- `configs/sample.yaml` and `configs/dev.local.yaml` are canonical dev references
- Scenario samples in `configs/samples/` -- update relevant sample when changing a feature it exercises
- `state/` files are generated runtime output, not documentation -- safe to delete and regenerate

## Task Workflow

Follow token-constrained operating mode from `AGENT_HANDOFF.md`:
1. Read `AGENT_HANDOFF.md` -> `TASKS.md` -> files for current task only
2. Surgical edits (1-3 files per run), one subsystem per run
3. Always update `TASKS.md` after changes: `DONE:`, `NEXT:`, `RISKS:`, `OPEN_QUESTIONS:`
4. Run targeted checks before declaring done
5. Library capability gaps are thesis findings -- document in `TASKS.md` under `THESIS-FINDING:`, don't silently work around them
