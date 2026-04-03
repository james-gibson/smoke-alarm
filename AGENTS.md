# AGENTS.md

Guidelines for agentic coding agents operating in this repository.

> **Note**: This repo also contains agent-specific config files. See:
> - [`.cursorrules`](.cursorrules) - Cursor AI rules
> - [`.cursor/rules/`](.cursor/rules/) - Cursor directory rules
> - [`.github/copilot-instructions.md`](.github/copilot-instructions.md) - GitHub Copilot
> - [`.aider.conf.yml`](.aider.conf.yml) - Aider config
> - [`.claude.md`](.claude.md) - Claude Code settings
> - [`.zed/`](zed/) - Zed editor settings
> - [`.opencode/skills/`](.opencode/skills/) - OpenCode skills
> - [`.opencode.md`](.opencode.md) - OpenCode settings

## What This Is

A minimal-footprint Go monitor for MCP/ACP environments. Runs in foreground (Bubble Tea TUI) or background (daemon) mode. Discovers, validates, and smoke-tests MCP/ACP service configurations, detects regressions from previously-healthy state, and supports unattended agent-managed lifecycle operations.

> **Project Note**: This folder is a scratchpad for managing Gherkin documents (`.feature` files in `features/`). These documents explore interconnection validity and may or may not represent valid implementations. Use `backlog.md` CLI for general project task management.
>
> **Experimental Features**: Random/consolidation Gherkin features go in `/Users/james/src/prototypes/pickled-onions`. This folder is self-organizing - agents capture random features there and try to simplify/consolidate them against features in `features/`.

## Build & Test Commands

| Task | Command |
|---|---|
| Full quality gate (fmt+lint+unit+integration+footprint) | `make ci` |
| Format | `make fmt` |
| Lint | `make lint` |
| Unit tests | `make unit` |
| Integration tests | `make integration` |
| Footprint budget checks | `make footprint` |
| Build binary | `go build -o ./bin/ocd-smoke-alarm ./cmd/ocd-smoke-alarm` |
| Sync dependencies | `make tidy` |

### Running a Single Test

```sh
go test ./tests/integration/... -run TestSmoke -v
go test ./... -run TestName -v
```

### Key CLI Subcommands

```sh
go run ./cmd/ocd-smoke-alarm validate --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm check --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm serve --mode=foreground --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm discover --config=configs/sample.yaml
go run ./cmd/ocd-smoke-alarm demo --config=configs/samples/hosted-mcp-acp.yaml
```

## Code Style Guidelines

### Imports

- Use Go's standard import organization: stdlib first, then external, then internal
- Local package prefix: `ocd-smoke-alarm`
- Run `goimports` (included in `make fmt`) to ensure correct ordering

### Formatting

- Use `gofumpt` for stricter formatting (enforced by lint)
- Run `make fmt` before committing
- No trailing commas unless required for readability

### Types & Structs

- Use explicit struct tags for YAML/JSON serialization (e.g., `yaml:"version"`, `json:"target_id"`)
- Use `map[string]any` for dynamic/extensible config fields
- Prefer value types over pointers unless nilability is required

### Naming Conventions

- **Packages**: lowercase, single word or short hyphenated (e.g., `engine`, `knownstate`)
- **Types**: PascalCase (e.g., `Engine`, `Config`, `TargetRuntimeStatus`)
- **Interfaces**: PascalCase with er suffix (e.g., `Prober`, `Notifier`)
- **Functions/Variables**: PascalCase for exports, camelCase for unexports
- **Constants**: PascalCase or SCREAMING_SNAKE depending on usage
- **Acronyms**: preserve case (e.g., `URL`, `HTTP`, not `Url`, `Http`)

### Error Handling

- Use `errors.Wrap` or `fmt.Errorf` with `%w` for context-rich errors
- Always handle `errcheck` findings - do not ignore errors
- Return descriptive errors from public API functions
- Use sentinel errors (`var ErrNotFound = errors.New(...)`) for predictable failure modes

### Concurrency

- Use `context.Context` for cancellation and timeouts
- Reuse HTTP clients with strict timeouts; never create per-request clients
- Use worker pools for bounded goroutines (see `golang.org/x/sync/semaphore`)
- No busy polling - all probe loops are timer/event driven

### Testing

- Test files: `*_test.go` in same package as implementation
- Use table-driven tests where appropriate
- Integration tests in `tests/integration/` directory
- Exclude test files from strict linting (see `.golangci.yml`)

### Logging

- Use `log/slog` for structured logging
- Avoid logging sensitive data (tokens, keys, credentials)

### Protocol Compliance

- **MCP/ACP**: do not invent JSON-RPC methods or skip capability handshake steps
- **Bubble Tea**: state mutations only in `Update()`, no side effects in `View()`, async work via `tea.Cmd`

### Critical Constraints

- **Reference Conflict Halt Rule**: If implementation would conflict with authoritative docs for Bubble Tea, MCP, or ACP, halt and state the conflict before proceeding
- **Routing**: fan-out is valid; cycles are forbidden. Every routed message carries a routing trace
- **Config schema**: versioned - check `internal/config/config.go` for current version
- **Samples**: update relevant sample in `configs/samples/` when changing a feature it exercises

## Cursor Directory Rules

The `.cursor/rules/` directory contains specialized rules:

| File | Purpose |
|------|---------|
| `engine.md` | Probe scheduling engine rules |
| `ui.md` | Bubble Tea TUI rules |
| `protocol.md` | MCP/ACP protocol compliance |
| `testing.md` | Test writing guidelines |
| `README.md` | Quick reference |

### UI Subsystem (`internal/ui/`)

- Uses Bubble Tea (Elm Architecture)
- State mutations ONLY in `Update()` method
- NO side effects in `View()` method
- Async work via `tea.Cmd`
- Do NOT share state with `internal/proxy/`

### Engine Subsystem (`internal/engine/`)

- Use worker pools for bounded goroutines
- All probe loops must be timer/event driven (no busy polling)
- Reuse HTTP clients with strict timeouts
- Every routed message must carry a routing trace
- Fan-out is valid; cycles are forbidden

### Protocol Compliance

- Do NOT invent JSON-RPC methods
- Do NOT skip capability handshake steps
- Follow all official protocol specifications exactly

## Architecture

### Package Layout

- `cmd/ocd-smoke-alarm/main.go` -- CLI entrypoint, flag parsing, subcommand dispatch
- `internal/engine/` -- probe scheduling, event loop, regression state machine
- `internal/knownstate/` -- known-good baseline store for drift/regression detection
- `internal/discovery/` -- MCP/ACP server + agent discovery (static configs + remote llms.txt)
- `internal/dynamicconfig/` -- persisted dynamic config artifact generation
- `internal/hosted/` -- embedded MCP/ACP HTTP + SSE server for local validation
- `internal/safety/` -- HURL pre-protocol safety checks
- `internal/engine/stdio_prober.go` -- stdio transport probe (npx, uvx command targets)
- `internal/auth/` -- token validity, OAuth flow helpers, mock redirect endpoint
- `internal/health/` -- /healthz, /readyz, /status HTTP endpoints
- `internal/ui/` -- Bubble Tea TUI (Elm Architecture: Init/Update/View)
- `internal/alerts/` -- notification routing and severity policy
- `internal/targets/` -- target registry (config + dynamic)
- `internal/meta/` -- meta-config template generation for mcp-add
- `internal/telemetry/` -- OTEL metric export
- `internal/ops/` -- remote stop/update/restart lifecycle orchestration
- `internal/federation/` -- multi-instance mesh (experimental)

### Two Distinct UI Concerns

- **Agentic UI** (`internal/ui/`): Bubble Tea dashboard. Driven by engine's SnapshotProvider.
- **Dynamic UI via stdio proxy** (`internal/proxy/`, planned): NOT Bubble Tea. Injects UI into MCP message flow. Must produce valid MCP/ACP protocol messages.

When any task says "UI", confirm which of these two is intended before writing code.

## Task Workflow

1. Read `AGENT_HANDOFF.md` -> `TASKS.md` -> files for current task only
2. Surgical edits (1-3 files per run), one subsystem per run
3. Always update `TASKS.md` after changes: `DONE:`, `NEXT:`, `RISKS:`, `OPEN_QUESTIONS:`
4. Run targeted checks before declaring done
5. Library capability gaps are thesis findings -- document in `TASKS.md` under `THESIS-FINDING:`, don't silently work around them