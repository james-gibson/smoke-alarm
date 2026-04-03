package targets

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Protocol identifies the target protocol family.
type Protocol string

const (
	ProtocolMCP      Protocol = "mcp"
	ProtocolACP      Protocol = "acp"
	ProtocolHTTP     Protocol = "http"
	ProtocolTCP      Protocol = "tcp"
	ProtocolOTLPHTTP Protocol = "otlp_http"
	ProtocolOTLPGRPC Protocol = "otlp_grpc"
)

// Transport identifies the connection transport used by a target.
type Transport string

const (
	TransportHTTP      Transport = "http"
	TransportWebSocket Transport = "websocket"
	TransportSSE       Transport = "sse"
	TransportStdio     Transport = "stdio"
	TransportGRPC      Transport = "grpc"
	TransportTCP       Transport = "tcp"
)

// AuthType identifies how a target should be authenticated.
type AuthType string

const (
	AuthNone   AuthType = "none"
	AuthBearer AuthType = "bearer"
	AuthAPIKey AuthType = "apikey"
	AuthOAuth  AuthType = "oauth"
)

// HealthState represents the resulting health classification for a protocol check.
type HealthState string

const (
	StateUnknown    HealthState = "unknown"
	StateHealthy    HealthState = "healthy"
	StateDegraded   HealthState = "degraded"
	StateUnhealthy  HealthState = "unhealthy"
	StateOutage     HealthState = "outage"
	StateRegression HealthState = "regression"
)

// Severity is used for alerting policy and operator visibility.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarn     Severity = "warn"
	SeverityCritical Severity = "critical"
)

// FailureClass normalizes root-cause classes for diagnostics and alert routing.
type FailureClass string

const (
	FailureNone        FailureClass = "none"
	FailureNetwork     FailureClass = "network"
	FailureTimeout     FailureClass = "timeout"
	FailureAuth        FailureClass = "auth"
	FailureProtocol    FailureClass = "protocol"
	FailureConfig      FailureClass = "config"
	FailureTLS         FailureClass = "tls"
	FailureRateLimited FailureClass = "rate_limited"
	FailureUnknown     FailureClass = "unknown"
)

// AuthConfig defines authentication settings for a target.
type AuthConfig struct {
	Type        AuthType  `json:"type" yaml:"type"`
	Header      string    `json:"header,omitempty" yaml:"header,omitempty"`         // e.g. Authorization
	KeyName     string    `json:"key_name,omitempty" yaml:"key_name,omitempty"`     // e.g. x-api-key
	SecretRef   string    `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"` // keystore reference
	ClientID    string    `json:"client_id,omitempty" yaml:"client_id,omitempty"`
	TokenURL    string    `json:"token_url,omitempty" yaml:"token_url,omitempty"`
	RedirectURL string    `json:"redirect_url,omitempty" yaml:"redirect_url,omitempty"`
	CallbackID  string    `json:"callback_id,omitempty" yaml:"callback_id,omitempty"`
	Scopes      []string  `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Audience    string    `json:"audience,omitempty" yaml:"audience,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty"`
}

// ExpectedBehavior captures semantic expectations used by protocol checks.
type ExpectedBehavior struct {
	HealthyStatusCodes []int    `json:"healthy_status_codes,omitempty" yaml:"healthy_status_codes,omitempty"`
	MinCapabilities    []string `json:"min_capabilities,omitempty" yaml:"min_capabilities,omitempty"`
	KnownAgentCountMin int      `json:"known_agent_count_min,omitempty" yaml:"known_agent_count_min,omitempty"`
	ExpectedVersion    string   `json:"expected_version,omitempty" yaml:"expected_version,omitempty"`
}

// CheckPolicy tunes how checks are executed.
type CheckPolicy struct {
	Interval         time.Duration `json:"interval" yaml:"interval"`
	Timeout          time.Duration `json:"timeout" yaml:"timeout"`
	Retries          int           `json:"retries" yaml:"retries"`
	HandshakeProfile string        `json:"handshake_profile,omitempty" yaml:"handshake_profile,omitempty"` // none|base|strict
	RequiredMethods  []string      `json:"required_methods,omitempty" yaml:"required_methods,omitempty"`   // protocol methods expected to succeed
	HURLTests        []HURLTest    `json:"hurl_tests,omitempty" yaml:"hurl_tests,omitempty"`
}

// StdioCommand defines how to launch a stdio-based MCP/ACP server.
type StdioCommand struct {
	Command string            `json:"command" yaml:"command"`
	Args    []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
}

// HURLTest registers a stop-gap HTTP safety test executed before deeper protocol checks.
type HURLTest struct {
	Name     string            `json:"name" yaml:"name"`
	File     string            `json:"file,omitempty" yaml:"file,omitempty"`
	Endpoint string            `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Method   string            `json:"method,omitempty" yaml:"method,omitempty"`
	Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body     string            `json:"body,omitempty" yaml:"body,omitempty"`
}

// TargetType classifies targets as local (stdio command) or remote (URL-based).
// Aligned with OpenCode config.json mcp.type field.
type TargetType string

const (
	TargetTypeLocal  TargetType = "local"
	TargetTypeRemote TargetType = "remote"
)

// Target is a single monitorable endpoint or proxy.
type Target struct {
	ID        string            `json:"id" yaml:"id"`
	Enabled   bool              `json:"enabled" yaml:"enabled"`
	Protocol  Protocol          `json:"protocol" yaml:"protocol"`
	Name      string            `json:"name" yaml:"name"`
	Endpoint  string            `json:"endpoint" yaml:"endpoint"`
	Transport Transport         `json:"transport" yaml:"transport"`
	Type      TargetType        `json:"type" yaml:"type"` // local or remote (OpenCode-aligned)
	Expected  ExpectedBehavior  `json:"expected" yaml:"expected"`
	Auth      AuthConfig        `json:"auth" yaml:"auth"`
	Stdio     StdioCommand      `json:"stdio,omitempty" yaml:"stdio,omitempty"`
	Check     CheckPolicy       `json:"check" yaml:"check"`
	Tags      map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`
	Meta      map[string]string `json:"meta,omitempty" yaml:"meta,omitempty"`
	Headers   map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// Validate performs basic semantic validation before target checks are scheduled.
func (t Target) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("target id is required")
	}
	if t.Protocol == "" {
		return fmt.Errorf("target %q protocol is required", t.ID)
	}
	if t.Transport == "" {
		return fmt.Errorf("target %q transport is required", t.ID)
	}

	switch t.Transport {
	case TransportStdio:
		if strings.TrimSpace(t.Stdio.Command) == "" {
			return fmt.Errorf("target %q stdio command is required when transport=stdio", t.ID)
		}
		if strings.TrimSpace(t.Endpoint) != "" {
			if _, err := url.ParseRequestURI(t.Endpoint); err != nil {
				return fmt.Errorf("target %q endpoint is invalid: %w", t.ID, err)
			}
		}

	case TransportSSE:
		if strings.TrimSpace(t.Endpoint) == "" {
			return fmt.Errorf("target %q endpoint is required", t.ID)
		}
		u, err := url.ParseRequestURI(t.Endpoint)
		if err != nil {
			return fmt.Errorf("target %q endpoint is invalid: %w", t.ID, err)
		}
		scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
		if scheme != "http" && scheme != "https" {
			return fmt.Errorf("target %q sse endpoint must use http/https, got %q", t.ID, u.Scheme)
		}

	default:
		if strings.TrimSpace(t.Endpoint) == "" {
			return fmt.Errorf("target %q endpoint is required", t.ID)
		}
		if _, err := url.ParseRequestURI(t.Endpoint); err != nil {
			return fmt.Errorf("target %q endpoint is invalid: %w", t.ID, err)
		}
	}

	if t.Check.Timeout <= 0 {
		return fmt.Errorf("target %q timeout must be > 0", t.ID)
	}
	if t.Check.Interval <= 0 {
		return fmt.Errorf("target %q interval must be > 0", t.ID)
	}
	if t.Check.Retries < 0 {
		return fmt.Errorf("target %q retries must be >= 0", t.ID)
	}
	if hp := strings.ToLower(strings.TrimSpace(t.Check.HandshakeProfile)); hp != "" {
		switch hp {
		case "none", "base", "strict":
			// valid
		default:
			return fmt.Errorf("target %q handshake_profile %q is not supported", t.ID, t.Check.HandshakeProfile)
		}
	}
	for i, m := range t.Check.RequiredMethods {
		if strings.TrimSpace(m) == "" {
			return fmt.Errorf("target %q required_methods[%d] cannot be empty", t.ID, i)
		}
	}

	for i, ht := range t.Check.HURLTests {
		if strings.TrimSpace(ht.Name) == "" {
			return fmt.Errorf("target %q hurl_tests[%d] name is required", t.ID, i)
		}
		hasFile := strings.TrimSpace(ht.File) != ""
		hasEndpoint := strings.TrimSpace(ht.Endpoint) != ""

		if !hasFile && !hasEndpoint {
			return fmt.Errorf("target %q hurl_tests[%d] requires either file or endpoint", t.ID, i)
		}
		if hasFile && hasEndpoint {
			return fmt.Errorf("target %q hurl_tests[%d] file and endpoint are mutually exclusive", t.ID, i)
		}
		if hasEndpoint {
			if _, err := url.ParseRequestURI(ht.Endpoint); err != nil {
				return fmt.Errorf("target %q hurl_tests[%d] endpoint is invalid: %w", t.ID, i, err)
			}
		}
		if m := strings.ToUpper(strings.TrimSpace(ht.Method)); m != "" {
			switch m {
			case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
			default:
				return fmt.Errorf("target %q hurl_tests[%d] method %q is not supported", t.ID, i, ht.Method)
			}
		}
	}

	switch t.Auth.Type {
	case "", AuthNone:
		// no-op
	case AuthBearer:
		if t.Auth.SecretRef == "" {
			return fmt.Errorf("target %q bearer auth requires secret_ref", t.ID)
		}
	case AuthAPIKey:
		if t.Auth.KeyName == "" || t.Auth.SecretRef == "" {
			return fmt.Errorf("target %q apikey auth requires key_name and secret_ref", t.ID)
		}
	case AuthOAuth:
		if t.Auth.ClientID == "" || t.Auth.TokenURL == "" {
			return fmt.Errorf("target %q oauth auth requires at least client_id and token_url", t.ID)
		}
	default:
		return fmt.Errorf("target %q auth type %q is not supported", t.ID, t.Auth.Type)
	}
	return nil
}

// Transition records a state change for a target.
type Transition struct {
	From   HealthState `json:"from" yaml:"from"`
	To     HealthState `json:"to" yaml:"to"`
	At     time.Time   `json:"at" yaml:"at"`
	Reason string      `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// CheckResult is the normalized result of a target protocol check.
type CheckResult struct {
	TargetID     string         `json:"target_id" yaml:"target_id"`
	Protocol     Protocol       `json:"protocol" yaml:"protocol"`
	State        HealthState    `json:"state" yaml:"state"`
	Severity     Severity       `json:"severity" yaml:"severity"`
	FailureClass FailureClass   `json:"failure_class" yaml:"failure_class"`
	Message      string         `json:"message,omitempty" yaml:"message,omitempty"`
	StatusCode   int            `json:"status_code,omitempty" yaml:"status_code,omitempty"`
	Latency      time.Duration  `json:"latency,omitempty" yaml:"latency,omitempty"`
	CheckedAt    time.Time      `json:"checked_at" yaml:"checked_at"`
	Attempt      int            `json:"attempt" yaml:"attempt"`
	Regression   bool           `json:"regression" yaml:"regression"`
	PreviouslyOK bool           `json:"previously_ok" yaml:"previously_ok"`
	Capabilities []string       `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	AgentCount   int            `json:"agent_count,omitempty" yaml:"agent_count,omitempty"`
	Details      map[string]any `json:"details,omitempty" yaml:"details,omitempty"`
	Transition   *Transition    `json:"transition,omitempty" yaml:"transition,omitempty"`
}

// IsFailure reports whether a check result indicates a failed outcome.
func (r CheckResult) IsFailure() bool {
	switch r.State {
	case StateUnhealthy, StateOutage, StateRegression:
		return true
	default:
		return false
	}
}

// IsEscalated reports whether this result should trigger high-priority alerting.
func (r CheckResult) IsEscalated() bool {
	return r.Severity == SeverityCritical || r.State == StateRegression || r.Regression
}
