#!/usr/bin/env bash
# generate-smoke-targets.yaml
# Usage:
#   ./generate-smoke-targets.sh npm  @modelcontextprotocol/server-filesystem @modelcontextprotocol/server-memory
#   ./generate-smoke-targets.sh uvx  mcp-server-fetch mcp-server-git mcp-server-time

MODE="${1:?Usage: $0 [npm|uvx] package1 package2 ...}"
shift
PACKAGES=("$@")

PORT_BASE=18189
[ "$MODE" = "uvx" ] && PORT_BASE=18190

cat <<HEADER
version: "1"

service:
  name: "smoke-alarm-${MODE}-mcp"
  environment: "${MODE}-mcp-test"
  mode: "foreground"
  log_level: "info"
  poll_interval: "10s"
  timeout: "8s"
  max_workers: 4

health:
  enabled: true
  listen_addr: "localhost:${PORT_BASE}"
  endpoints:
    healthz: "/healthz"
    readyz: "/readyz"
    status: "/status"

runtime:
  lock_file: "/tmp/smoke-alarm.${MODE}-mcp.lock"
  state_dir: "./state/${MODE}-mcp"
  baseline_file: "./state/${MODE}-mcp/known-good.json"
  event_history_size: 300
  graceful_shutdown_timeout: "8s"

discovery:
  enabled: false

alerts:
  aggressive: true
  notify_on_regression_immediately: true
  retry_before_escalation: 1
  dedupe_window: "30s"
  cooldown: "10s"
  severity:
    healthy: "info"
    degraded: "warn"
    regression: "critical"
    outage: "critical"
  sinks:
    log:
      enabled: true
    os_notification:
      enabled: false

auth:
  keystore:
    enabled: false
  redaction:
    enabled: true
    mask: "****"
  oauth:
    enabled: false

targets:
HEADER

for pkg in "${PACKAGES[@]}"; do
  # Derive a clean ID from the package name
  safe_id=$(echo "$pkg" | sed 's|[@/]|-|g; s|^-||; s|-$||')

  if [ "$MODE" = "npm" ]; then
    cat <<TARGET

  - id: "${MODE}-mcp-${safe_id}"
    enabled: true
    protocol: "mcp"
    name: "MCP ${pkg} (npx)"
    endpoint: "stdio://local"
    transport: "stdio"
    stdio:
      command: "npx"
      args:
        - "-y"
        - "${pkg}"
      env:
        MCP_LOG_LEVEL: "warn"
      cwd: "."
    expected:
      min_capabilities:
        - "tools/list"
    auth:
      type: "none"
    check:
      interval: "10s"
      timeout: "8s"
      retries: 1
      handshake_profile: "strict"
      required_methods:
        - "initialize"
        - "tools/list"
      hurl_tests: []
TARGET
  else
    cat <<TARGET

  - id: "${MODE}-mcp-${safe_id}"
    enabled: true
    protocol: "mcp"
    name: "MCP ${pkg} (uvx)"
    endpoint: "stdio://local"
    transport: "stdio"
    stdio:
      command: "uvx"
      args:
        - "${pkg}"
      env: {}
      cwd: "."
    expected:
      min_capabilities:
        - "tools/list"
    auth:
      type: "none"
    check:
      interval: "10s"
      timeout: "8s"
      retries: 1
      handshake_profile: "strict"
      required_methods:
        - "initialize"
        - "tools/list"
      hurl_tests: []
TARGET
  fi
done

cat <<FOOTER

known_state:
  enabled: true
  persist: true
  sustain_success_before_mark_healthy: 1
  classify_new_failures_after_healthy_as: "regression"
  outage_threshold_consecutive_failures: 2

meta_config:
  enabled: true
  output_dir: "./state/${MODE}-mcp/meta-config"
  formats:
    - "yaml"
    - "json"
  include_confidence: true
  include_provenance: true
FOOTER
