# GitHub Copilot Instructions

## Project Overview

A minimal-footprint Go monitor for MCP/ACP environments. Runs in foreground (Bubble Tea TUI) or background (daemon) mode.

## Language

- Go 1.22+
- Uses Bubble Tea for TUI
- Protocols: MCP, ACP

## Build Commands

```bash
make ci        # Full quality gate (fmt+lint+unit+integration+footprint)
make fmt       # Format code
make lint      # Run linters
make unit      # Unit tests
make integration  # Integration tests
```

## Key Guidelines

See [AGENTS.md](../AGENTS.md) for full details.

### Critical Constraints

1. **Bubble Tea**: State mutations only in `Update()`, no side effects in `View()`, async work via `tea.Cmd`
2. **MCP/ACP**: Do not invent JSON-RPC methods or skip capability handshake steps
3. **Reference Conflict Halt Rule**: If implementation would conflict with authoritative docs, halt and state the conflict

### Architecture

- `internal/engine/` - Core orchestrator
- `internal/ui/` - Bubble Tea TUI
- `internal/proxy/` - MCP message flow UI (NOT Bubble Tea)
- `tests/integration/` - Integration tests
