package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ModeForeground = "foreground"
	ModeBackground = "background"
	ModeHeadless   = "headless"

	LogDebug = "debug"
	LogInfo  = "info"
	LogWarn  = "warn"
	LogError = "error"

	SeverityInfo     = "info"
	SeverityWarn     = "warning"
	SeverityError    = "error"
	SeverityCritical = "critical"

	TransportHTTP      = "http"
	TransportSSE       = "sse"
	TransportWebSocket = "websocket"
	TransportStdio     = "stdio"

	ProtocolMCP  = "mcp"
	ProtocolACP  = "acp"
	ProtocolHTTP = "http"

	AuthNone   = "none"
	AuthBearer = "bearer"
	AuthAPIKey = "apikey"
	AuthOAuth  = "oauth"

	// TargetTypeLocal classifies a target as local (stdio command).
	// Aligned with OpenCode config.json mcp.type field.
	TargetTypeLocal = "local"
	// TargetTypeRemote classifies a target as remote (URL-based).
	TargetTypeRemote = "remote"
)

// Config is the root typed configuration schema.
type Config struct {
	Version       string              `yaml:"version"`
	Service       ServiceConfig       `yaml:"service"`
	Health        HealthConfig        `yaml:"health"`
	Runtime       RuntimeConfig       `yaml:"runtime"`
	Discovery     DiscoveryConfig     `yaml:"discovery"`
	Alerts        AlertsConfig        `yaml:"alerts"`
	Auth          GlobalAuthConfig    `yaml:"auth"`
	Targets       []TargetConfig      `yaml:"targets"`
	KnownState    KnownStateConfig    `yaml:"known_state"`
	MetaConfig    MetaConfigConfig    `yaml:"meta_config"`
	RemoteAgent   RemoteAgentConfig   `yaml:"remote_agent"`
	Hosted        HostedConfig        `yaml:"hosted"`
	DynamicConfig DynamicConfigConfig `yaml:"dynamic_config"`
	Telemetry     TelemetryConfig     `yaml:"telemetry"`
	Federation    FederationConfig    `yaml:"federation"`
	Tuner         TunerConfig         `yaml:"tuner"`
	Extra         map[string]any      `yaml:",inline"`
}

type FederationConfig struct {
	Enabled           bool     `yaml:"enabled"`
	Upstream          string   `yaml:"upstream"`           // upstream endpoint to report to
	Downstream        []string `yaml:"downstream"`         // downstream endpoints to poll
	Rank              int      `yaml:"rank"`               // rank based on port (lower = upstream)
	PollInterval      string   `yaml:"poll_interval"`      // how often to poll downstream
	BasePort          int      `yaml:"base_port"`          // lowest local port considered for slot claiming
	MaxPort           int      `yaml:"max_port"`           // highest local port considered for slot claiming
	AnnounceInterval  string   `yaml:"announce_interval"`  // how often to send introductions/announcements
	HeartbeatInterval string   `yaml:"heartbeat_interval"` // cadence for heartbeats to introducer/upstream
	HeartbeatTimeout  string   `yaml:"heartbeat_timeout"`  // grace period before peers are considered stale
}

type TelemetryConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Endpoint       string `yaml:"endpoint"`        // OTEL collector URL
	ServiceName    string `yaml:"service_name"`    // metric namespace
	ExportInterval string `yaml:"export_interval"` // flush interval
}

// TunerConfig controls integration with the Tuner passive-observer application.
type TunerConfig struct {
	Enabled     bool                  `yaml:"enabled"`
	Advertise   bool                  `yaml:"advertise"`    // mDNS advertisement of _smoke-alarm._tcp
	ServiceType string                `yaml:"service_type"` // default: _smoke-alarm._tcp
	Audience    TunerAudienceConfig   `yaml:"audience"`
	CallerHook  TunerCallerHookConfig `yaml:"caller_hook"`
}

type TunerAudienceConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"` // path for audience metric POST, default: /tuner/audience
}

type TunerCallerHookConfig struct {
	Enabled     bool `yaml:"enabled"`
	MCPResponse bool `yaml:"mcp_response"` // emit caller messages as MCP tool responses
}

type ServiceConfig struct {
	Name              string `yaml:"name"`
	Environment       string `yaml:"environment"`
	Mode              string `yaml:"mode"`
	LogLevel          string `yaml:"log_level"`
	PollInterval      string `yaml:"poll_interval"`
	Timeout           string `yaml:"timeout"`
	MaxWorkers        int    `yaml:"max_workers"`
	EfficiencyProfile string `yaml:"efficiency_profile"` // auto|low|medium|high
}

const (
	EfficiencyAuto   = "auto"
	EfficiencyLow    = "low"    // minimal compute, longer intervals
	EfficiencyMedium = "medium" // balanced
	EfficiencyHigh   = "high"   // aggressive polling, more resources

	// TODO: Add CPU percentage, file descriptors, network I/O, disk I/O metrics
	// when cloud spot pricing is lower - these require periodic sampling
)

// TODO(future): Additional OS-level telemetry to add when needed:
// - process.cpu.percent (requires periodic sampling, higher overhead)
// - process.open_fds (file descriptor count)
// - process.network.io_bytes (network I/O counters)
// - process.disk.io_bytes (disk I/O counters)
// - process.thread_count (native thread count)
// - OTEL resource attributes for container/pod metadata

func applyEfficiencyProfile(svc *ServiceConfig) {
	profile := strings.ToLower(svc.EfficiencyProfile)
	if profile == "" || profile == EfficiencyAuto {
		profile = EfficiencyMedium
	}

	switch profile {
	case EfficiencyLow:
		if svc.MaxWorkers <= 0 || svc.MaxWorkers > 2 {
			svc.MaxWorkers = 1
		}
		if svc.PollInterval == "" {
			svc.PollInterval = "60s"
		}
		if svc.Timeout == "" {
			svc.Timeout = "10s"
		}

	case EfficiencyMedium:
		if svc.MaxWorkers <= 0 || svc.MaxWorkers > 4 {
			svc.MaxWorkers = 4
		}
		if svc.PollInterval == "" {
			svc.PollInterval = "15s"
		}
		if svc.Timeout == "" {
			svc.Timeout = "5s"
		}

	case EfficiencyHigh:
		if svc.MaxWorkers <= 0 {
			svc.MaxWorkers = 8
		}
		if svc.PollInterval == "" {
			svc.PollInterval = "5s"
		}
		if svc.Timeout == "" {
			svc.Timeout = "3s"
		}
	}
}

type HealthConfig struct {
	Enabled    bool         `yaml:"enabled"`
	ListenAddr string       `yaml:"listen_addr"`
	Endpoints  HealthRoutes `yaml:"endpoints"`
	SelfCheck  bool         `yaml:"self_check"`
}

type HealthRoutes struct {
	Healthz string `yaml:"healthz"`
	Readyz  string `yaml:"readyz"`
	Status  string `yaml:"status"`
}

type RuntimeConfig struct {
	LockFile                string `yaml:"lock_file"`
	StateDir                string `yaml:"state_dir"`
	BaselineFile            string `yaml:"baseline_file"`
	EventHistorySize        int    `yaml:"event_history_size"`
	GracefulShutdownTimeout string `yaml:"graceful_shutdown_timeout"`
}

type DiscoveryConfig struct {
	Enabled          bool                   `yaml:"enabled"`
	Interval         string                 `yaml:"interval"`
	IncludeLocalPath []string               `yaml:"include_local_paths"`
	IncludeEnvVars   []string               `yaml:"include_env_vars"`
	LocalProxyScan   LocalProxyScanConfig   `yaml:"local_proxy_scan"`
	CloudCatalog     CloudCatalogConfig     `yaml:"cloud_catalog"`
	LLMSTxt          LLMSTxtDiscoveryConfig `yaml:"llms_txt"`
}

type LocalProxyScanConfig struct {
	Enabled bool     `yaml:"enabled"`
	Hosts   []string `yaml:"hosts"`
	Ports   []int    `yaml:"ports"`
}

type CloudCatalogConfig struct {
	Enabled bool     `yaml:"enabled"`
	URLs    []string `yaml:"urls"`
}

type LLMSTxtDiscoveryConfig struct {
	Enabled               bool     `yaml:"enabled"`
	RemoteURIs            []string `yaml:"remote_uris"`
	FetchTimeout          string   `yaml:"fetch_timeout"`
	RequireHTTPS          bool     `yaml:"require_https"`
	AutoRegisterAsTargets bool     `yaml:"auto_register_as_targets"`
	AutoRegisterOAuth     bool     `yaml:"auto_register_oauth"`
}

type AlertsConfig struct {
	Aggressive                    bool            `yaml:"aggressive"`
	NotifyOnRegressionImmediately bool            `yaml:"notify_on_regression_immediately"`
	RetryBeforeEscalation         int             `yaml:"retry_before_escalation"`
	DedupeWindow                  string          `yaml:"dedupe_window"`
	Cooldown                      string          `yaml:"cooldown"`
	Severity                      SeverityConfig  `yaml:"severity"`
	Sinks                         AlertSinkConfig `yaml:"sinks"`
}

type SeverityConfig struct {
	Healthy    string `yaml:"healthy"`
	Degraded   string `yaml:"degraded"`
	Regression string `yaml:"regression"`
	Outage     string `yaml:"outage"`
}

type AlertSinkConfig struct {
	Log            SinkToggleConfig         `yaml:"log"`
	OSNotification OSNotificationSinkConfig `yaml:"os_notification"`
}

type SinkToggleConfig struct {
	Enabled bool `yaml:"enabled"`
}

type OSNotificationSinkConfig struct {
	Enabled     bool   `yaml:"enabled"`
	TitlePrefix string `yaml:"title_prefix"`
}

type GlobalAuthConfig struct {
	Keystore  GlobalKeystoreConfig `yaml:"keystore"`
	Redaction RedactionConfig      `yaml:"redaction"`
	OAuth     GlobalOAuthConfig    `yaml:"oauth"`
}

type GlobalKeystoreConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Provider string `yaml:"provider"` // auto|keychain|secretservice|file
}

type RedactionConfig struct {
	Enabled bool   `yaml:"enabled"`
	Mask    string `yaml:"mask"`
}

type GlobalOAuthConfig struct {
	Enabled          bool                    `yaml:"enabled"`
	TokenGracePeriod string                  `yaml:"token_grace_period"`
	Unattended       GlobalOAuthUnattended   `yaml:"unattended"`
	MockRedirect     OAuthMockRedirectConfig `yaml:"mock_redirect"`
}

type GlobalOAuthUnattended struct {
	Enabled                bool `yaml:"enabled"`
	AllowDeviceCode        bool `yaml:"allow_device_code"`
	AllowClientCredentials bool `yaml:"allow_client_credentials"`
	AllowRefreshToken      bool `yaml:"allow_refresh_token"`
}

type OAuthMockRedirectConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenAddr string `yaml:"listen_addr"`
	Path       string `yaml:"path"`
	Mode       string `yaml:"mode"` // allow|fail
}

type TargetConfig struct {
	ID        string             `yaml:"id"`
	Enabled   bool               `yaml:"enabled"`
	Protocol  string             `yaml:"protocol"`
	Name      string             `yaml:"name"`
	Endpoint  string             `yaml:"endpoint"`
	Transport string             `yaml:"transport"`
	Expected  ExpectedConfig     `yaml:"expected"`
	Auth      TargetAuthConfig   `yaml:"auth"`
	Stdio     StdioCommandConfig `yaml:"stdio"`
	Check     TargetCheckConfig  `yaml:"check"`

	// OpenCode-aligned fields (additive — existing fields keep working).
	// Type is "local" (stdio command) or "remote" (URL-based). Inferred from
	// Transport if empty. Aligns with OpenCode config.json mcp.type.
	Type string `yaml:"type,omitempty"`
	// URL is an alias for Endpoint on remote targets. If both are set, Endpoint wins.
	URL string `yaml:"url,omitempty"`
	// Command is an OpenCode-style command array alternative to Stdio.Command + Stdio.Args.
	// e.g. ["npx", "-y", "@modelcontextprotocol/server-everything"]
	// If both Command and Stdio.Command are set, Stdio wins.
	Command []string `yaml:"command,omitempty"`
	// Environment is an OpenCode-style env map alternative to Stdio.Env.
	// If both are set, Stdio.Env wins.
	Environment map[string]string `yaml:"environment,omitempty"`
	// Headers for remote targets, passed through to HTTP requests.
	Headers map[string]string `yaml:"headers,omitempty"`
	// OAuth configuration block for remote targets (OpenCode-aligned).
	// If Auth.Type is already "oauth" with fields set, those take precedence.
	OAuth *TargetOAuthConfig `yaml:"oauth,omitempty"`
}

// TargetOAuthConfig is an OpenCode-aligned OAuth block for remote MCP targets.
type TargetOAuthConfig struct {
	ClientID        string `yaml:"client_id,omitempty"`
	ClientSecretRef string `yaml:"client_secret_ref,omitempty"` // env:// or keychain:// reference
	Scope           string `yaml:"scope,omitempty"`
	TokenURL        string `yaml:"token_url,omitempty"`
}

type ExpectedConfig struct {
	HealthyStatusCodes []int    `yaml:"healthy_status_codes"`
	MinCapabilities    []string `yaml:"min_capabilities"`
	KnownAgentCountMin int      `yaml:"known_agent_count_min"`
	ExpectedVersion    string   `yaml:"expected_version"`
}

type TargetAuthConfig struct {
	Type        string   `yaml:"type"` // none|bearer|apikey|oauth
	SecretRef   string   `yaml:"secret_ref"`
	Header      string   `yaml:"header"`
	KeyName     string   `yaml:"key_name"`
	ClientID    string   `yaml:"client_id"`
	TokenURL    string   `yaml:"token_url"`
	RedirectURL string   `yaml:"redirect_url"`
	CallbackID  string   `yaml:"callback_id"`
	Scopes      []string `yaml:"scopes"`
}

type StdioCommandConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
	Cwd     string            `yaml:"cwd"`
}

type HURLTestConfig struct {
	Name     string            `yaml:"name"`
	File     string            `yaml:"file"`
	Endpoint string            `yaml:"endpoint"`
	Method   string            `yaml:"method"`
	Headers  map[string]string `yaml:"headers"`
	Body     string            `yaml:"body"`
}

type TargetCheckConfig struct {
	Interval         string           `yaml:"interval"`
	Timeout          string           `yaml:"timeout"`
	Retries          int              `yaml:"retries"`
	HandshakeProfile string           `yaml:"handshake_profile"`
	RequiredMethods  []string         `yaml:"required_methods"`
	HURLTests        []HURLTestConfig `yaml:"hurl_tests"`
}

type KnownStateConfig struct {
	Enabled                            bool   `yaml:"enabled"`
	Persist                            bool   `yaml:"persist"`
	SustainSuccessBeforeMarkHealthy    int    `yaml:"sustain_success_before_mark_healthy"`
	ClassifyNewFailuresAfterHealthyAs  string `yaml:"classify_new_failures_after_healthy_as"`
	OutageThresholdConsecutiveFailures int    `yaml:"outage_threshold_consecutive_failures"`
}

type MetaConfigConfig struct {
	Enabled           bool             `yaml:"enabled"`
	OutputDir         string           `yaml:"output_dir"`
	Formats           []string         `yaml:"formats"`
	IncludeConfidence bool             `yaml:"include_confidence"`
	IncludeProvenance bool             `yaml:"include_provenance"`
	Placeholders      MetaPlaceholders `yaml:"placeholders"`
}

type MetaPlaceholders struct {
	Token        string `yaml:"token"`
	ClientSecret string `yaml:"client_secret"`
	Endpoint     string `yaml:"endpoint"`
}

type RemoteAgentConfig struct {
	ManagedUpdates  bool               `yaml:"managed_updates"`
	ControlEndpoint string             `yaml:"control_endpoint"`
	Update          RemoteUpdateConfig `yaml:"update"`
	Safety          RemoteSafetyConfig `yaml:"safety"`
}

type RemoteUpdateConfig struct {
	Strategy          string `yaml:"strategy"`
	StopCommand       string `yaml:"stop_command"`
	StartCommand      string `yaml:"start_command"`
	VerifyCommand     string `yaml:"verify_command"`
	RollbackOnFailure bool   `yaml:"rollback_on_failure"`
	MaxWaitForHealthy string `yaml:"max_wait_for_healthy"`
}

type RemoteSafetyConfig struct {
	RequireLock bool   `yaml:"require_lock"`
	LockTTL     string `yaml:"lock_ttl"`
}

type HostedConfig struct {
	Enabled    bool                 `yaml:"enabled"`
	ListenAddr string               `yaml:"listen_addr"`
	Transports []string             `yaml:"transports"` // http|sse
	Protocols  []string             `yaml:"protocols"`  // mcp|acp|a2a
	Endpoints  HostedEndpointConfig `yaml:"endpoints"`
}

type HostedEndpointConfig struct {
	MCP string `yaml:"mcp"`
	ACP string `yaml:"acp"`
	A2A string `yaml:"a2a"`
}

type DynamicConfigConfig struct {
	Enabled          bool     `yaml:"enabled"`
	Directory        string   `yaml:"directory"`
	Formats          []string `yaml:"formats"` // json|markdown
	ServeBaseURL     string   `yaml:"serve_base_url"`
	AllowOverwrite   bool     `yaml:"allow_overwrite"`
	RequireUniqueIDs bool     `yaml:"require_unique_ids"`
}

type ValidationError struct {
	Problems []string
}

func (v ValidationError) Error() string {
	if len(v.Problems) == 0 {
		return "configuration invalid"
	}
	return "configuration invalid:\n - " + strings.Join(v.Problems, "\n - ")
}

// Load reads configuration from file path, applies defaults, and validates it.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg, err := LoadBytes(raw)
	if err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	return cfg, nil
}

// LoadBytes parses config data, applies defaults, and validates.
func LoadBytes(data []byte) (Config, error) {
	var cfg Config

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, err
	}

	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ApplyDefaults mutates config with conservative defaults.
func (c *Config) ApplyDefaults() {
	// Version is not defaulted — it must be explicitly set; Validate() will
	// reject an empty or unsupported version with a clear error.

	// Service defaults.
	if c.Service.Name == "" {
		c.Service.Name = "ocd-smoke-alarm"
	}
	if c.Service.Environment == "" {
		c.Service.Environment = "dev"
	}
	if c.Service.Mode == "" {
		c.Service.Mode = ModeBackground
	}
	if c.Service.LogLevel == "" {
		c.Service.LogLevel = LogInfo
	}
	if c.Service.EfficiencyProfile == "" {
		if c.Service.Mode == ModeBackground || c.Service.Mode == ModeHeadless {
			c.Service.EfficiencyProfile = EfficiencyLow
		} else {
			c.Service.EfficiencyProfile = EfficiencyMedium
		}
	}

	// Apply efficiency profile settings.
	applyEfficiencyProfile(&c.Service)

	// Final fallback for any values not set by profile.
	if c.Service.PollInterval == "" {
		c.Service.PollInterval = "15s"
	}
	if c.Service.Timeout == "" {
		c.Service.Timeout = "5s"
	}
	if c.Service.MaxWorkers <= 0 {
		c.Service.MaxWorkers = 4
	}

	// Health defaults.
	if c.Health.ListenAddr == "" {
		c.Health.ListenAddr = "localhost:8088"
	}
	if c.Health.Endpoints.Healthz == "" {
		c.Health.Endpoints.Healthz = "/healthz"
	}
	if c.Health.Endpoints.Readyz == "" {
		c.Health.Endpoints.Readyz = "/readyz"
	}
	if c.Health.Endpoints.Status == "" {
		c.Health.Endpoints.Status = "/status"
	}
	// Default to enabled unless explicitly disabled.
	if !c.Health.Enabled && c.Health.ListenAddr == "localhost:8088" {
		c.Health.Enabled = true
	}
	if c.Health.SelfCheck && !c.Health.Enabled {
		c.Health.Enabled = true
	}

	// Runtime defaults.
	if c.Runtime.LockFile == "" {
		c.Runtime.LockFile = fmt.Sprintf("/tmp/%s.lock", strings.ReplaceAll(c.Service.Name, " ", "-"))
	}
	if c.Runtime.StateDir == "" {
		c.Runtime.StateDir = "./state"
	}
	if c.Runtime.BaselineFile == "" {
		c.Runtime.BaselineFile = "./state/known-good.json"
	}
	if c.Runtime.EventHistorySize <= 0 {
		c.Runtime.EventHistorySize = 500
	}
	if c.Runtime.GracefulShutdownTimeout == "" {
		c.Runtime.GracefulShutdownTimeout = "10s"
	}

	// Discovery defaults.
	if c.Discovery.Interval == "" {
		c.Discovery.Interval = "60s"
	}
	if len(c.Discovery.LocalProxyScan.Hosts) == 0 {
		c.Discovery.LocalProxyScan.Hosts = []string{"127.0.0.1", "localhost"}
	}
	if len(c.Discovery.LocalProxyScan.Ports) == 0 {
		c.Discovery.LocalProxyScan.Ports = []int{3000, 4317, 4318, 8080, 9000}
	}
	if c.Discovery.LLMSTxt.FetchTimeout == "" {
		c.Discovery.LLMSTxt.FetchTimeout = "5s"
	}
	if c.Discovery.LLMSTxt.AutoRegisterOAuth && !c.Discovery.LLMSTxt.AutoRegisterAsTargets {
		c.Discovery.LLMSTxt.AutoRegisterAsTargets = true
	}

	// Alert defaults.
	if c.Alerts.DedupeWindow == "" {
		c.Alerts.DedupeWindow = "2m"
	}
	if c.Alerts.Cooldown == "" {
		c.Alerts.Cooldown = "1m"
	}
	if c.Alerts.RetryBeforeEscalation < 0 {
		c.Alerts.RetryBeforeEscalation = 0
	}
	if c.Alerts.Severity.Healthy == "" {
		c.Alerts.Severity.Healthy = SeverityInfo
	}
	if c.Alerts.Severity.Degraded == "" {
		c.Alerts.Severity.Degraded = SeverityWarn
	}
	if c.Alerts.Severity.Regression == "" {
		c.Alerts.Severity.Regression = SeverityCritical
	}
	if c.Alerts.Severity.Outage == "" {
		c.Alerts.Severity.Outage = SeverityCritical
	}
	if c.Alerts.Sinks.OSNotification.TitlePrefix == "" {
		c.Alerts.Sinks.OSNotification.TitlePrefix = "[Smoke Alarm]"
	}

	// Auth defaults.
	if c.Auth.Keystore.Provider == "" {
		c.Auth.Keystore.Provider = "auto"
	}
	if c.Auth.Redaction.Mask == "" {
		c.Auth.Redaction.Mask = "****"
	}
	if c.Auth.OAuth.TokenGracePeriod == "" {
		c.Auth.OAuth.TokenGracePeriod = "30s"
	}
	if c.Auth.OAuth.MockRedirect.ListenAddr == "" {
		c.Auth.OAuth.MockRedirect.ListenAddr = "localhost:8877"
	}
	if c.Auth.OAuth.MockRedirect.Path == "" {
		c.Auth.OAuth.MockRedirect.Path = "/oauth/callback"
	}
	if c.Auth.OAuth.MockRedirect.Mode == "" {
		c.Auth.OAuth.MockRedirect.Mode = "allow"
	}

	// Known state defaults.
	if c.KnownState.SustainSuccessBeforeMarkHealthy <= 0 {
		c.KnownState.SustainSuccessBeforeMarkHealthy = 2
	}
	if c.KnownState.ClassifyNewFailuresAfterHealthyAs == "" {
		c.KnownState.ClassifyNewFailuresAfterHealthyAs = "regression"
	}
	if c.KnownState.OutageThresholdConsecutiveFailures <= 0 {
		c.KnownState.OutageThresholdConsecutiveFailures = 2
	}

	// Meta config defaults.
	if c.MetaConfig.OutputDir == "" {
		c.MetaConfig.OutputDir = "./state/meta-config"
	}
	if len(c.MetaConfig.Formats) == 0 {
		c.MetaConfig.Formats = []string{"yaml", "json"}
	}
	if c.MetaConfig.Placeholders.Token == "" {
		c.MetaConfig.Placeholders.Token = "${TOKEN}"
	}
	if c.MetaConfig.Placeholders.ClientSecret == "" {
		c.MetaConfig.Placeholders.ClientSecret = "${CLIENT_SECRET}"
	}
	if c.MetaConfig.Placeholders.Endpoint == "" {
		c.MetaConfig.Placeholders.Endpoint = "${ENDPOINT}"
	}

	// Remote agent defaults.
	if c.RemoteAgent.Update.MaxWaitForHealthy == "" {
		c.RemoteAgent.Update.MaxWaitForHealthy = "60s"
	}
	if c.RemoteAgent.Safety.LockTTL == "" {
		c.RemoteAgent.Safety.LockTTL = "5m"
	}

	// Hosted service defaults.
	if c.Hosted.ListenAddr == "" {
		c.Hosted.ListenAddr = ":0"
	}
	if len(c.Hosted.Transports) == 0 {
		c.Hosted.Transports = []string{TransportHTTP, TransportSSE}
	}
	if len(c.Hosted.Protocols) == 0 {
		c.Hosted.Protocols = []string{ProtocolMCP, ProtocolACP}
	}
	if c.Hosted.Endpoints.MCP == "" {
		c.Hosted.Endpoints.MCP = "/mcp"
	}
	if c.Hosted.Endpoints.ACP == "" {
		c.Hosted.Endpoints.ACP = "/acp"
	}
	if c.Hosted.Endpoints.A2A == "" {
		c.Hosted.Endpoints.A2A = "/a2a"
	}

	// Dynamic config defaults.
	if c.DynamicConfig.Directory == "" {
		c.DynamicConfig.Directory = "./state/dynamic-config"
	}
	if len(c.DynamicConfig.Formats) == 0 {
		c.DynamicConfig.Formats = []string{"json", "markdown"}
	}
	if c.DynamicConfig.ServeBaseURL == "" {
		c.DynamicConfig.ServeBaseURL = "/dynamic-config"
	}
	if !c.DynamicConfig.RequireUniqueIDs {
		c.DynamicConfig.RequireUniqueIDs = true
	}

	if c.Federation.BasePort <= 0 || c.Federation.BasePort > 65535 {
		c.Federation.BasePort = 19100
	}
	if c.Federation.MaxPort <= 0 || c.Federation.MaxPort < c.Federation.BasePort || c.Federation.MaxPort > 65535 {
		c.Federation.MaxPort = c.Federation.BasePort + 3
	}
	if c.Federation.PollInterval == "" {
		c.Federation.PollInterval = "30s"
	}
	if c.Federation.AnnounceInterval == "" {
		c.Federation.AnnounceInterval = "10s"
	}
	if c.Federation.HeartbeatInterval == "" {
		c.Federation.HeartbeatInterval = "15s"
	}
	if c.Federation.HeartbeatTimeout == "" {
		c.Federation.HeartbeatTimeout = "45s"
	}

	for i := range c.Targets {
		t := &c.Targets[i]

		// Reconcile OpenCode-aligned fields into canonical fields.
		reconcileTargetOpenCodeFields(t)

		if t.Name == "" {
			t.Name = t.ID
		}
		if t.Auth.Type == "" {
			t.Auth.Type = AuthNone
		}
		if t.Check.Interval == "" {
			t.Check.Interval = c.Service.PollInterval
		}
		if t.Check.Timeout == "" {
			t.Check.Timeout = c.Service.Timeout
		}
		if t.Check.Retries < 0 {
			t.Check.Retries = 0
		}
		if strings.TrimSpace(t.Check.HandshakeProfile) == "" {
			switch strings.ToLower(strings.TrimSpace(t.Protocol)) {
			case ProtocolMCP, ProtocolACP:
				t.Check.HandshakeProfile = "base"
			default:
				t.Check.HandshakeProfile = "none"
			}
		}
		if len(t.Check.RequiredMethods) == 0 {
			switch strings.ToLower(strings.TrimSpace(t.Protocol)) {
			case ProtocolMCP, ProtocolACP:
				t.Check.RequiredMethods = []string{"initialize"}
			}
		}
		if len(t.Expected.HealthyStatusCodes) == 0 {
			switch strings.ToLower(t.Transport) {
			case TransportWebSocket:
				t.Expected.HealthyStatusCodes = []int{101}
			default:
				t.Expected.HealthyStatusCodes = []int{200}
			}
		}
		if t.Transport == "" {
			t.Transport = inferTransport(t.Endpoint)
		}
		if strings.EqualFold(t.Transport, TransportStdio) && strings.TrimSpace(t.Endpoint) == "" {
			t.Endpoint = "stdio://local"
		}

		// Infer Type from Transport if not explicitly set.
		if t.Type == "" {
			if strings.EqualFold(t.Transport, TransportStdio) {
				t.Type = TargetTypeLocal
			} else {
				t.Type = TargetTypeRemote
			}
		}
	}
}

// Validate performs semantic validation and returns detailed error context.
func (c Config) Validate() error {
	var errs ValidationError
	add := func(format string, args ...any) {
		errs.Problems = append(errs.Problems, fmt.Sprintf(format, args...))
	}

	if c.Version == "" {
		add("version is required")
	} else if c.Version != "1" {
		add("unsupported version %q: only version \"1\" is supported", c.Version)
	}

	if !inSet(strings.ToLower(c.Service.Mode), ModeForeground, ModeBackground, ModeHeadless) {
		add("service.mode must be one of: foreground, background, headless")
	}
	if !inSet(strings.ToLower(c.Service.EfficiencyProfile), EfficiencyAuto, EfficiencyLow, EfficiencyMedium, EfficiencyHigh) {
		add("service.efficiency_profile must be one of: auto, low, medium, high")
	}
	if !inSet(strings.ToLower(c.Service.LogLevel), LogDebug, LogInfo, LogWarn, LogError) {
		add("service.log_level must be one of: debug, info, warn, error")
	}
	if _, err := parsePositiveDuration(c.Service.PollInterval); err != nil {
		add("service.poll_interval invalid: %v", err)
	}
	if _, err := parsePositiveDuration(c.Service.Timeout); err != nil {
		add("service.timeout invalid: %v", err)
	}
	if c.Service.MaxWorkers <= 0 || c.Service.MaxWorkers > 2048 {
		add("service.max_workers must be in range [1, 2048]")
	}

	if c.Health.Enabled {
		if c.Health.ListenAddr == "" {
			add("health.listen_addr required when health is enabled")
		} else if _, _, err := net.SplitHostPort(c.Health.ListenAddr); err != nil {
			add("health.listen_addr invalid host:port: %v", err)
		}
		for name, ep := range map[string]string{
			"healthz": c.Health.Endpoints.Healthz,
			"readyz":  c.Health.Endpoints.Readyz,
			"status":  c.Health.Endpoints.Status,
		} {
			if !strings.HasPrefix(ep, "/") {
				add("health.endpoints.%s must start with /", name)
			}
		}
	}

	if _, err := parsePositiveDuration(c.Runtime.GracefulShutdownTimeout); err != nil {
		add("runtime.graceful_shutdown_timeout invalid: %v", err)
	}
	if c.Runtime.EventHistorySize <= 0 {
		add("runtime.event_history_size must be > 0")
	}

	if c.Discovery.Enabled {
		if _, err := parsePositiveDuration(c.Discovery.Interval); err != nil {
			add("discovery.interval invalid: %v", err)
		}
		for _, p := range c.Discovery.LocalProxyScan.Ports {
			if p < 1 || p > 65535 {
				add("discovery.local_proxy_scan.ports must be in range [1,65535], got %d", p)
			}
		}
		for _, u := range c.Discovery.CloudCatalog.URLs {
			if err := validateURL(u); err != nil {
				add("discovery.cloud_catalog.urls invalid %q: %v", u, err)
			}
		}

		if c.Discovery.LLMSTxt.Enabled {
			if _, err := parsePositiveDuration(c.Discovery.LLMSTxt.FetchTimeout); err != nil {
				add("discovery.llms_txt.fetch_timeout invalid: %v", err)
			}
			for i, uri := range c.Discovery.LLMSTxt.RemoteURIs {
				if strings.TrimSpace(uri) == "" {
					add("discovery.llms_txt.remote_uris[%d] cannot be empty", i)
					continue
				}
				if err := validateRemoteLLMSTxtURI(uri, c.Discovery.LLMSTxt.RequireHTTPS); err != nil {
					add("discovery.llms_txt.remote_uris[%d] invalid %q: %v", i, uri, err)
				}
			}
			if c.Discovery.LLMSTxt.AutoRegisterOAuth && !c.Discovery.LLMSTxt.AutoRegisterAsTargets {
				add("discovery.llms_txt.auto_register_oauth requires discovery.llms_txt.auto_register_as_targets=true")
			}
		}
	}

	if _, err := parsePositiveDuration(c.Alerts.DedupeWindow); err != nil {
		add("alerts.dedupe_window invalid: %v", err)
	}
	if _, err := parsePositiveDuration(c.Alerts.Cooldown); err != nil {
		add("alerts.cooldown invalid: %v", err)
	}
	if c.Alerts.RetryBeforeEscalation < 0 {
		add("alerts.retry_before_escalation must be >= 0")
	}
	validSeverities := map[string]bool{
		"info":     true,
		"warning":  true,
		"error":    true,
		"critical": true,
	}
	for field, sev := range map[string]string{
		"healthy":    c.Alerts.Severity.Healthy,
		"degraded":   c.Alerts.Severity.Degraded,
		"regression": c.Alerts.Severity.Regression,
		"outage":     c.Alerts.Severity.Outage,
	} {
		norm := strings.ToLower(sev)
		if norm == "warn" {
			norm = "warning"
		}
		if !validSeverities[norm] {
			add("alerts.severity.%s invalid %q", field, sev)
		}
	}

	if !inSet(strings.ToLower(c.Auth.Keystore.Provider), "auto", "keychain", "secretservice", "file") {
		add("auth.keystore.provider must be one of: auto, keychain, secretservice, file")
	}
	if c.Auth.Redaction.Enabled && c.Auth.Redaction.Mask == "" {
		add("auth.redaction.mask cannot be empty when redaction is enabled")
	}
	if c.Auth.OAuth.Enabled {
		if _, err := parsePositiveDuration(c.Auth.OAuth.TokenGracePeriod); err != nil {
			add("auth.oauth.token_grace_period invalid: %v", err)
		}
	}

	if c.Auth.OAuth.MockRedirect.Enabled {
		if _, _, err := net.SplitHostPort(c.Auth.OAuth.MockRedirect.ListenAddr); err != nil {
			add("auth.oauth.mock_redirect.listen_addr invalid host:port: %v", err)
		}
		if !strings.HasPrefix(c.Auth.OAuth.MockRedirect.Path, "/") {
			add("auth.oauth.mock_redirect.path must start with /")
		}
		if !inSet(strings.ToLower(strings.TrimSpace(c.Auth.OAuth.MockRedirect.Mode)), "allow", "fail") {
			add("auth.oauth.mock_redirect.mode must be one of: allow, fail")
		}
	}

	if c.KnownState.Enabled {
		if c.KnownState.SustainSuccessBeforeMarkHealthy < 1 {
			add("known_state.sustain_success_before_mark_healthy must be >= 1")
		}
		if c.KnownState.OutageThresholdConsecutiveFailures < 1 {
			add("known_state.outage_threshold_consecutive_failures must be >= 1")
		}
		if !inSet(strings.ToLower(c.KnownState.ClassifyNewFailuresAfterHealthyAs), "regression", "outage") {
			add("known_state.classify_new_failures_after_healthy_as must be regression or outage")
		}
	}

	if c.MetaConfig.Enabled {
		if c.MetaConfig.OutputDir == "" {
			add("meta_config.output_dir is required when meta_config.enabled=true")
		}
		seen := map[string]struct{}{}
		for _, f := range c.MetaConfig.Formats {
			ff := strings.ToLower(strings.TrimSpace(f))
			if !inSet(ff, "yaml", "json") {
				add("meta_config.formats contains unsupported value %q", f)
			}
			if _, ok := seen[ff]; ok {
				add("meta_config.formats contains duplicate value %q", f)
			}
			seen[ff] = struct{}{}
		}
	}

	if c.RemoteAgent.ManagedUpdates {
		if c.RemoteAgent.ControlEndpoint != "" {
			if err := validateURL(c.RemoteAgent.ControlEndpoint); err != nil {
				add("remote_agent.control_endpoint invalid: %v", err)
			}
		}
		if _, err := parsePositiveDuration(c.RemoteAgent.Update.MaxWaitForHealthy); err != nil {
			add("remote_agent.update.max_wait_for_healthy invalid: %v", err)
		}
		if c.RemoteAgent.Safety.RequireLock {
			if _, err := parsePositiveDuration(c.RemoteAgent.Safety.LockTTL); err != nil {
				add("remote_agent.safety.lock_ttl invalid: %v", err)
			}
		}
	}

	if c.Hosted.Enabled {
		if _, _, err := net.SplitHostPort(c.Hosted.ListenAddr); err != nil {
			add("hosted.listen_addr invalid host:port: %v", err)
		}
		for i, tr := range c.Hosted.Transports {
			if !inSet(strings.ToLower(strings.TrimSpace(tr)), TransportHTTP, TransportSSE) {
				add("hosted.transports[%d] must be one of: http, sse", i)
			}
		}
		for i, proto := range c.Hosted.Protocols {
			if !inSet(strings.ToLower(strings.TrimSpace(proto)), ProtocolMCP, ProtocolACP, "a2a") {
				add("hosted.protocols[%d] must be one of: mcp, acp, a2a", i)
			}
		}
		for name, ep := range map[string]string{
			"mcp": c.Hosted.Endpoints.MCP,
			"acp": c.Hosted.Endpoints.ACP,
			"a2a": c.Hosted.Endpoints.A2A,
		} {
			if strings.TrimSpace(ep) == "" {
				add("hosted.endpoints.%s is required when hosted.enabled=true", name)
				continue
			}
			if !strings.HasPrefix(ep, "/") {
				add("hosted.endpoints.%s must start with /", name)
			}
		}
	}

	if c.DynamicConfig.Enabled {
		if strings.TrimSpace(c.DynamicConfig.Directory) == "" {
			add("dynamic_config.directory is required when dynamic_config.enabled=true")
		}
		if strings.TrimSpace(c.DynamicConfig.ServeBaseURL) == "" {
			add("dynamic_config.serve_base_url is required when dynamic_config.enabled=true")
		} else if !strings.HasPrefix(c.DynamicConfig.ServeBaseURL, "/") {
			add("dynamic_config.serve_base_url must start with /")
		}

		seenFormats := map[string]struct{}{}
		for i, f := range c.DynamicConfig.Formats {
			ff := strings.ToLower(strings.TrimSpace(f))
			if !inSet(ff, "json", "markdown") {
				add("dynamic_config.formats[%d] must be one of: json, markdown", i)
				continue
			}
			if _, ok := seenFormats[ff]; ok {
				add("dynamic_config.formats[%d] duplicate format %q", i, f)
				continue
			}
			seenFormats[ff] = struct{}{}
		}
	}

	if c.Federation.Enabled {
		if c.Federation.BasePort < 1 || c.Federation.BasePort > 65535 {
			add("federation.base_port must be in range [1,65535]")
		}
		if c.Federation.MaxPort < c.Federation.BasePort || c.Federation.MaxPort > 65535 {
			add("federation.max_port must be >= base_port and <= 65535")
		}
		if _, err := parsePositiveDuration(c.Federation.PollInterval); err != nil {
			add("federation.poll_interval invalid: %v", err)
		}
		if _, err := parsePositiveDuration(c.Federation.AnnounceInterval); err != nil {
			add("federation.announce_interval invalid: %v", err)
		}
		if _, err := parsePositiveDuration(c.Federation.HeartbeatInterval); err != nil {
			add("federation.heartbeat_interval invalid: %v", err)
		}
		if _, err := parsePositiveDuration(c.Federation.HeartbeatTimeout); err != nil {
			add("federation.heartbeat_timeout invalid: %v", err)
		}
	}

	seenIDs := make(map[string]struct{}, len(c.Targets))
	for i, t := range c.Targets {
		prefix := fmt.Sprintf("targets[%d]", i)
		if t.ID == "" {
			add("%s.id is required", prefix)
		} else {
			if _, ok := seenIDs[t.ID]; ok {
				add("%s.id duplicate value %q", prefix, t.ID)
			}
			seenIDs[t.ID] = struct{}{}
		}
		if !t.Enabled {
			continue
		}

		if !inSet(strings.ToLower(t.Protocol), ProtocolMCP, ProtocolACP, ProtocolHTTP) {
			add("%s invalid protocol %q: must be one of: mcp, acp, http", prefix, t.Protocol)
		}
		if !inSet(strings.ToLower(t.Transport), TransportHTTP, TransportSSE, TransportWebSocket, TransportStdio) {
			add("%s invalid transport %q: must be one of: http, sse, websocket, stdio", prefix, t.Transport)
		}
		if t.Type != "" && !inSet(strings.ToLower(t.Type), TargetTypeLocal, TargetTypeRemote) {
			add("%s.type must be one of: local, remote", prefix)
		}
		if strings.EqualFold(t.Transport, TransportStdio) {
			if strings.TrimSpace(t.Stdio.Command) == "" {
				add("%s.stdio.command is required when transport=stdio", prefix)
			}
			if strings.TrimSpace(t.Endpoint) != "" {
				if err := validateURL(t.Endpoint); err != nil {
					add("%s.endpoint invalid: %v", prefix, err)
				}
			}
		} else {
			if t.Endpoint == "" {
				add("%s.endpoint is required", prefix)
			} else if err := validateURL(t.Endpoint); err != nil {
				add("%s.endpoint invalid: %v", prefix, err)
			}
		}
		if _, err := parsePositiveDuration(t.Check.Interval); err != nil {
			add("%s.check.interval invalid: %v", prefix, err)
		}
		if _, err := parsePositiveDuration(t.Check.Timeout); err != nil {
			add("%s.check.timeout invalid: %v", prefix, err)
		}
		if t.Check.Retries < 0 || t.Check.Retries > 100 {
			add("%s.check.retries must be in range [0,100]", prefix)
		}
		if !inSet(strings.ToLower(strings.TrimSpace(t.Check.HandshakeProfile)), "none", "base", "strict") {
			add("%s.check.handshake_profile must be one of: none, base, strict", prefix)
		}
		for j, m := range t.Check.RequiredMethods {
			if strings.TrimSpace(m) == "" {
				add("%s.check.required_methods[%d] cannot be empty", prefix, j)
			}
		}
		for j, ht := range t.Check.HURLTests {
			hprefix := fmt.Sprintf("%s.check.hurl_tests[%d]", prefix, j)
			if strings.TrimSpace(ht.Name) == "" {
				add("%s.name is required", hprefix)
			}
			if strings.TrimSpace(ht.File) == "" && strings.TrimSpace(ht.Endpoint) == "" {
				add("%s.file or %s.endpoint is required", hprefix, hprefix)
			}
			if strings.TrimSpace(ht.File) != "" && strings.TrimSpace(ht.Endpoint) != "" {
				add("%s.file and %s.endpoint are mutually exclusive", hprefix, hprefix)
			}
			if strings.TrimSpace(ht.Endpoint) != "" {
				if err := validateURL(ht.Endpoint); err != nil {
					add("%s.endpoint invalid: %v", hprefix, err)
				}
			}
			if strings.TrimSpace(ht.Method) != "" &&
				!inSet(strings.ToUpper(strings.TrimSpace(ht.Method)), "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS") {
				add("%s.method invalid %q", hprefix, ht.Method)
			}
		}
		for _, code := range t.Expected.HealthyStatusCodes {
			if code < 100 || code > 599 {
				add("%s.expected.healthy_status_codes contains invalid code %d", prefix, code)
			}
		}
		if t.Expected.KnownAgentCountMin < 0 {
			add("%s.expected.known_agent_count_min must be >= 0", prefix)
		}

		authType := strings.ToLower(t.Auth.Type)
		if !inSet(authType, AuthNone, AuthBearer, AuthAPIKey, AuthOAuth) {
			add("%s.auth.type must be one of: none, bearer, apikey, oauth", prefix)
		}
		switch authType {
		case AuthBearer:
			if t.Auth.SecretRef == "" {
				add("%s.auth.secret_ref required for bearer auth", prefix)
			}
			if t.Auth.Header == "" {
				add("%s.auth.header required for bearer auth", prefix)
			}
		case AuthAPIKey:
			if t.Auth.SecretRef == "" {
				add("%s.auth.secret_ref required for apikey auth", prefix)
			}
			if t.Auth.KeyName == "" {
				add("%s.auth.key_name required for apikey auth", prefix)
			}
		case AuthOAuth:
			if t.Auth.ClientID == "" {
				add("%s.auth.client_id required for oauth auth", prefix)
			}
			if t.Auth.TokenURL == "" {
				add("%s.auth.token_url required for oauth auth", prefix)
			} else if err := validateURL(t.Auth.TokenURL); err != nil {
				add("%s.auth.token_url invalid: %v", prefix, err)
			}
			// redirect_url and callback_id are optional — required only for
			// the mock redirect flow, not for OpenCode-style OAuth configs.
			if t.Auth.RedirectURL != "" {
				if err := validateURL(t.Auth.RedirectURL); err != nil {
					add("%s.auth.redirect_url invalid: %v", prefix, err)
				}
			}
		}
	}

	if len(errs.Problems) > 0 {
		return errs
	}
	return nil
}

// EnabledTargets returns enabled targets only.
func (c Config) EnabledTargets() []TargetConfig {
	out := make([]TargetConfig, 0, len(c.Targets))
	for _, t := range c.Targets {
		if t.Enabled {
			out = append(out, t)
		}
	}
	return out
}

// TargetByID finds target by identifier.
func (c Config) TargetByID(id string) (TargetConfig, bool) {
	for _, t := range c.Targets {
		if t.ID == id {
			return t, true
		}
	}
	return TargetConfig{}, false
}

// reconcileTargetOpenCodeFields merges OpenCode-aligned fields into the
// canonical target fields. Existing fields always win when both are set.
// This allows configs written in either style to work identically.
func reconcileTargetOpenCodeFields(t *TargetConfig) {
	// Type -> Transport inference: if Type is set but Transport is not, derive it.
	if t.Type != "" && t.Transport == "" {
		switch strings.ToLower(t.Type) {
		case TargetTypeLocal:
			t.Transport = TransportStdio
		case TargetTypeRemote:
			// Transport will be inferred from endpoint/URL later in ApplyDefaults.
		}
	}

	// URL -> Endpoint: OpenCode uses "url" for remote targets.
	if t.Endpoint == "" && t.URL != "" {
		t.Endpoint = t.URL
	}

	// Command -> Stdio: OpenCode uses command array ["npx", "arg1", "arg2"].
	if t.Stdio.Command == "" && len(t.Command) > 0 {
		t.Stdio.Command = t.Command[0]
		if len(t.Command) > 1 {
			t.Stdio.Args = t.Command[1:]
		}
		if t.Transport == "" {
			t.Transport = TransportStdio
		}
	}

	// Environment -> Stdio.Env: OpenCode uses "environment" at target level.
	if len(t.Stdio.Env) == 0 && len(t.Environment) > 0 {
		t.Stdio.Env = t.Environment
	}

	// OAuth block -> Auth fields: OpenCode uses an "oauth" object on the target.
	if t.OAuth != nil && t.Auth.Type == "" {
		t.Auth.Type = AuthOAuth
		if t.Auth.ClientID == "" {
			t.Auth.ClientID = t.OAuth.ClientID
		}
		if t.Auth.SecretRef == "" {
			t.Auth.SecretRef = t.OAuth.ClientSecretRef
		}
		if t.Auth.TokenURL == "" {
			t.Auth.TokenURL = t.OAuth.TokenURL
		}
		if len(t.Auth.Scopes) == 0 && t.OAuth.Scope != "" {
			t.Auth.Scopes = strings.Split(t.OAuth.Scope, " ")
		}
	}
}

func inferTransport(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return TransportHTTP
	}

	switch strings.ToLower(u.Scheme) {
	case "ws", "wss":
		return TransportWebSocket
	case "stdio":
		return TransportStdio
	case "http", "https":
		path := strings.ToLower(strings.TrimSpace(u.Path))
		query := strings.ToLower(strings.TrimSpace(u.RawQuery))
		full := path + "?" + query
		if strings.Contains(full, "transport=sse") ||
			strings.Contains(full, "accept=text/event-stream") ||
			strings.Contains(full, "text/event-stream") ||
			strings.Contains(full, "/sse") ||
			strings.Contains(full, "/stream") ||
			strings.Contains(full, "/events") {
			return TransportSSE
		}
		return TransportHTTP
	default:
		return TransportHTTP
	}
}

func parsePositiveDuration(raw string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, errors.New("empty duration")
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be > 0, got %s", d)
	}
	return d, nil
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "" {
		return errors.New("missing scheme")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss", "stdio":
		return nil
	default:
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
}

func validateRemoteLLMSTxtURI(raw string, requireHTTPS bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "" {
		return errors.New("missing scheme")
	}

	scheme := strings.ToLower(u.Scheme)
	if requireHTTPS && scheme != "https" {
		return fmt.Errorf("https is required, got %q", u.Scheme)
	}
	if !requireHTTPS && scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return errors.New("missing host")
	}
	if !strings.HasSuffix(strings.ToLower(u.Path), "/llms.txt") && !strings.EqualFold(strings.TrimSpace(u.Path), "llms.txt") {
		return errors.New("path must reference llms.txt")
	}
	return nil
}

func inSet(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
