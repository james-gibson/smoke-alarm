package health

import (
	"strings"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
)

// SelfDescription is the machine-readable service description document served
// at /.well-known/smoke-alarm.json. It is generated from config + runtime state
// and follows the schema at configs/schema/self-description.schema.json.
type SelfDescription struct {
	Schema       string         `json:"$schema,omitempty"`
	Version      string         `json:"version"`
	Service      SDService      `json:"service"`
	Health       SDHealth       `json:"health"`
	Capabilities SDCapabilities `json:"capabilities"`
	Permissions  SDPermissions  `json:"permissions"`
	MCP          map[string]any `json:"mcp"`
	Targets      []SDTarget     `json:"targets"`
}

// SDService identifies the running instance.
type SDService struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Mode        string `json:"mode"`
	Environment string `json:"environment,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
}

// SDHealth describes health endpoint paths.
type SDHealth struct {
	ListenAddr string      `json:"listen_addr,omitempty"`
	Endpoints  SDEndpoints `json:"endpoints"`
}

// SDEndpoints lists the individual health paths.
type SDEndpoints struct {
	Healthz         string `json:"healthz,omitempty"`
	Readyz          string `json:"readyz,omitempty"`
	Status          string `json:"status,omitempty"`
	SelfDescription string `json:"self_description,omitempty"`
}

// SDCapabilities describes what the instance can do.
type SDCapabilities struct {
	Monitoring    SDMonitoring    `json:"monitoring"`
	Discovery     SDDiscovery     `json:"discovery"`
	Hosted        SDHosted        `json:"hosted"`
	DynamicConfig SDDynamicConfig `json:"dynamic_config"`
	MetaConfig    SDMetaConfig    `json:"meta_config"`
	Federation    SDFederation    `json:"federation"`
}

// SDMonitoring describes target monitoring capabilities.
type SDMonitoring struct {
	Protocols         []string `json:"protocols"`
	Transports        []string `json:"transports"`
	HandshakeProfiles []string `json:"handshake_profiles"`
	RegressionDetect  bool     `json:"regression_detection"`
	SafetyChecks      bool     `json:"safety_checks"`
}

// SDDiscovery describes target discovery capabilities.
type SDDiscovery struct {
	Enabled     bool     `json:"enabled"`
	Sources     []string `json:"sources"`
	LLMSTxtURIs []string `json:"llms_txt_uris,omitempty"`
}

// SDHosted describes embedded hosted endpoints.
type SDHosted struct {
	Enabled    bool              `json:"enabled"`
	ListenAddr string            `json:"listen_addr,omitempty"`
	Endpoints  map[string]string `json:"endpoints,omitempty"`
	Transports []string          `json:"transports,omitempty"`
	Protocols  []string          `json:"protocols,omitempty"`
}

// SDDynamicConfig describes dynamic config artifact generation.
type SDDynamicConfig struct {
	Enabled      bool     `json:"enabled"`
	Formats      []string `json:"formats,omitempty"`
	ServeBaseURL string   `json:"serve_base_url,omitempty"`
}

// SDMetaConfig describes meta-config template generation.
type SDMetaConfig struct {
	Enabled bool     `json:"enabled"`
	Formats []string `json:"formats,omitempty"`
}

// SDFederation describes multi-instance federation state.
type SDFederation struct {
	Enabled bool   `json:"enabled"`
	Role    string `json:"role"`
}

// SDPermissions describes what the service reads, probes, and writes.
type SDPermissions struct {
	Filesystem SDFilesystem `json:"filesystem"`
	Network    SDNetwork    `json:"network"`
	Secrets    SDSecrets    `json:"secrets"`
}

// SDFilesystem describes filesystem access patterns.
type SDFilesystem struct {
	Read  []string `json:"read"`
	Write []string `json:"write"`
}

// SDNetwork describes network access patterns.
type SDNetwork struct {
	Probe           string   `json:"probe"`
	Listen          []string `json:"listen"`
	OutboundDomains []string `json:"outbound_domains,omitempty"`
}

// SDSecrets describes credential handling.
type SDSecrets struct {
	Keystore  bool     `json:"keystore"`
	EnvRefs   []string `json:"env_refs,omitempty"`
	Redaction bool     `json:"redaction"`
}

// SDTarget is a summary of one monitored target.
type SDTarget struct {
	ID               string `json:"id"`
	Name             string `json:"name,omitempty"`
	Protocol         string `json:"protocol"`
	Transport        string `json:"transport"`
	Type             string `json:"type,omitempty"`
	Endpoint         string `json:"endpoint,omitempty"`
	State            string `json:"state,omitempty"`
	AuthType         string `json:"auth_type,omitempty"`
	HandshakeProfile string `json:"handshake_profile,omitempty"`
	Enabled          bool   `json:"enabled"`
}

// NewSelfDescriptionFactory returns a SelfDescriptionFunc that builds the live
// self-description from the given config and the health server's runtime state.
func NewSelfDescriptionFactory(cfg config.Config, appVersion string, startedAt time.Time, srv *Server) SelfDescriptionFunc {
	return func() any {
		return buildSelfDescription(cfg, appVersion, startedAt, srv)
	}
}

func buildSelfDescription(cfg config.Config, appVersion string, startedAt time.Time, srv *Server) SelfDescription {
	sd := SelfDescription{
		Version: "1",
		Service: SDService{
			Name:        cfg.Service.Name,
			Version:     appVersion,
			Mode:        cfg.Service.Mode,
			Environment: cfg.Service.Environment,
			StartedAt:   startedAt.Format(time.RFC3339),
		},
		Health: SDHealth{
			ListenAddr: cfg.Health.ListenAddr,
			Endpoints: SDEndpoints{
				Healthz:         cfg.Health.Endpoints.Healthz,
				Readyz:          cfg.Health.Endpoints.Readyz,
				Status:          cfg.Health.Endpoints.Status,
				SelfDescription: "/.well-known/smoke-alarm.json",
			},
		},
		Capabilities: buildCapabilities(cfg),
		Permissions:  buildPermissions(cfg),
		MCP:          make(map[string]any),
		Targets:      buildTargets(cfg, srv),
	}
	return sd
}

func buildCapabilities(cfg config.Config) SDCapabilities {
	// Discovery sources
	var sources []string
	if cfg.Discovery.Enabled {
		sources = append(sources, "static")
		if len(cfg.Discovery.IncludeEnvVars) > 0 {
			sources = append(sources, "environment")
		}
		if cfg.Discovery.LocalProxyScan.Enabled {
			sources = append(sources, "local_proxy")
		}
		if cfg.Discovery.CloudCatalog.Enabled {
			sources = append(sources, "cloud_catalog")
		}
		if cfg.Discovery.LLMSTxt.Enabled {
			sources = append(sources, "llms_txt")
		}
	}

	// Hosted endpoints
	hostedEndpoints := make(map[string]string)
	if cfg.Hosted.Enabled {
		if cfg.Hosted.Endpoints.MCP != "" {
			hostedEndpoints["mcp"] = cfg.Hosted.Endpoints.MCP
		}
		if cfg.Hosted.Endpoints.ACP != "" {
			hostedEndpoints["acp"] = cfg.Hosted.Endpoints.ACP
		}
		if cfg.Hosted.Endpoints.A2A != "" {
			hostedEndpoints["a2a"] = cfg.Hosted.Endpoints.A2A
		}
	}

	// Safety checks: true if any target has hurl_tests configured
	hasSafety := false
	for _, t := range cfg.Targets {
		if len(t.Check.HURLTests) > 0 {
			hasSafety = true
			break
		}
	}

	return SDCapabilities{
		Monitoring: SDMonitoring{
			Protocols:         []string{"mcp", "acp", "http"},
			Transports:        []string{"http", "https", "sse", "websocket", "stdio"},
			HandshakeProfiles: []string{"none", "base", "strict"},
			RegressionDetect:  cfg.KnownState.Enabled,
			SafetyChecks:      hasSafety,
		},
		Discovery: SDDiscovery{
			Enabled:     cfg.Discovery.Enabled,
			Sources:     sources,
			LLMSTxtURIs: cfg.Discovery.LLMSTxt.RemoteURIs,
		},
		Hosted: SDHosted{
			Enabled:    cfg.Hosted.Enabled,
			ListenAddr: cfg.Hosted.ListenAddr,
			Endpoints:  hostedEndpoints,
			Transports: cfg.Hosted.Transports,
			Protocols:  cfg.Hosted.Protocols,
		},
		DynamicConfig: SDDynamicConfig{
			Enabled:      cfg.DynamicConfig.Enabled,
			Formats:      cfg.DynamicConfig.Formats,
			ServeBaseURL: cfg.DynamicConfig.ServeBaseURL,
		},
		MetaConfig: SDMetaConfig{
			Enabled: cfg.MetaConfig.Enabled,
			Formats: cfg.MetaConfig.Formats,
		},
		Federation: SDFederation{
			Enabled: cfg.Federation.Enabled,
			Role:    "standalone",
		},
	}
}

func buildPermissions(cfg config.Config) SDPermissions {
	readPaths := []string{"./configs/"}
	writePaths := []string{cfg.Runtime.StateDir}

	if cfg.Runtime.BaselineFile != "" {
		readPaths = append(readPaths, cfg.Runtime.BaselineFile)
	}
	for _, p := range cfg.Discovery.IncludeLocalPath {
		readPaths = append(readPaths, p)
	}
	if cfg.DynamicConfig.Enabled && cfg.DynamicConfig.Directory != "" {
		writePaths = append(writePaths, cfg.DynamicConfig.Directory)
	}
	if cfg.Runtime.LockFile != "" {
		writePaths = append(writePaths, cfg.Runtime.LockFile)
	}

	listen := []string{cfg.Health.ListenAddr}
	if cfg.Hosted.Enabled && cfg.Hosted.ListenAddr != "" {
		listen = append(listen, cfg.Hosted.ListenAddr)
	}

	// Collect outbound domains from llms.txt URIs
	var outbound []string
	for _, uri := range cfg.Discovery.LLMSTxt.RemoteURIs {
		if d := domainFromURI(uri); d != "" {
			outbound = append(outbound, d)
		}
	}

	// Env refs from OAuth configs
	var envRefs []string
	for _, t := range cfg.Targets {
		if t.Auth.SecretRef != "" && strings.HasPrefix(t.Auth.SecretRef, "env://") {
			envRefs = append(envRefs, t.Auth.SecretRef)
		}
		if t.OAuth != nil && t.OAuth.ClientSecretRef != "" && strings.HasPrefix(t.OAuth.ClientSecretRef, "env://") {
			envRefs = append(envRefs, t.OAuth.ClientSecretRef)
		}
	}
	envRefs = dedup(envRefs)

	return SDPermissions{
		Filesystem: SDFilesystem{
			Read:  readPaths,
			Write: writePaths,
		},
		Network: SDNetwork{
			Probe:           "configured_targets_only",
			Listen:          listen,
			OutboundDomains: outbound,
		},
		Secrets: SDSecrets{
			Keystore:  cfg.Auth.Keystore.Enabled,
			EnvRefs:   envRefs,
			Redaction: cfg.Auth.Redaction.Enabled,
		},
	}
}

func buildTargets(cfg config.Config, srv *Server) []SDTarget {
	out := make([]SDTarget, 0, len(cfg.Targets))

	// Get runtime state from health server
	var runtimeStates map[string]TargetStatus
	if srv != nil {
		srv.mu.RLock()
		runtimeStates = make(map[string]TargetStatus, len(srv.targets))
		for k, v := range srv.targets {
			runtimeStates[k] = v
		}
		srv.mu.RUnlock()
	}

	for _, t := range cfg.Targets {
		if !t.Enabled {
			continue
		}
		sdt := SDTarget{
			ID:               t.ID,
			Name:             t.Name,
			Protocol:         t.Protocol,
			Transport:        t.Transport,
			Type:             t.Type,
			Endpoint:         t.Endpoint,
			AuthType:         t.Auth.Type,
			HandshakeProfile: t.Check.HandshakeProfile,
			Enabled:          t.Enabled,
		}
		if rs, ok := runtimeStates[t.ID]; ok {
			sdt.State = rs.State
		}
		out = append(out, sdt)
	}
	return out
}

func domainFromURI(uri string) string {
	// Simple extraction: strip scheme, take host.
	u := uri
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(u, prefix) {
			u = u[len(prefix):]
			break
		}
	}
	if idx := strings.IndexByte(u, '/'); idx > 0 {
		u = u[:idx]
	}
	if idx := strings.IndexByte(u, ':'); idx > 0 {
		u = u[:idx]
	}
	return u
}

func dedup(ss []string) []string {
	if len(ss) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
