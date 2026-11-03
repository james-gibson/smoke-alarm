package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

const mcpProtocolVersion = "2025-06-18"

var supportedMCPProtocolVersions = map[string]struct{}{
	"2024-11-05": {},
	"2025-03-26": {},
	"2025-06-18": {},
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// validateMCPInitializeResult checks the fields a client needs before it can
// treat an MCP server as initialized. Pointers distinguish an omitted field
// from a valid, empty capabilities object.
func validateMCPInitializeResult(result any) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal initialize result: %w", err)
	}

	var parsed struct {
		ProtocolVersion *string                    `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
		ServerInfo      *struct {
			Name    *string `json:"name"`
			Version *string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("decode initialize result: %w", err)
	}

	if parsed.ProtocolVersion == nil || strings.TrimSpace(*parsed.ProtocolVersion) == "" {
		return fmt.Errorf("initialize result missing protocolVersion")
	}
	protocolVersion := strings.TrimSpace(*parsed.ProtocolVersion)
	if _, ok := supportedMCPProtocolVersions[protocolVersion]; !ok {
		return fmt.Errorf("initialize result uses unsupported protocolVersion %q", protocolVersion)
	}
	if parsed.Capabilities == nil {
		return fmt.Errorf("initialize result missing capabilities")
	}
	if parsed.ServerInfo == nil {
		return fmt.Errorf("initialize result missing serverInfo")
	}
	if parsed.ServerInfo.Name == nil || strings.TrimSpace(*parsed.ServerInfo.Name) == "" {
		return fmt.Errorf("initialize result missing serverInfo.name")
	}
	if parsed.ServerInfo.Version == nil || strings.TrimSpace(*parsed.ServerInfo.Version) == "" {
		return fmt.Errorf("initialize result missing serverInfo.version")
	}
	return nil
}
