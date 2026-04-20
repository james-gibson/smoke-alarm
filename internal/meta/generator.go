package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// Generator builds intra-valid partial meta configs suitable for copy/paste
// workflows (for example into mcp-add), while keeping placeholders explicit.
type Generator struct {
	cfg config.MetaConfigConfig
	now func() time.Time
}

// NewGenerator creates a meta config generator with sane defaults.
func NewGenerator(cfg config.MetaConfigConfig) *Generator {
	if cfg.OutputDir == "" {
		cfg.OutputDir = "./state/meta-config"
	}
	if len(cfg.Formats) == 0 {
		cfg.Formats = []string{"yaml", "json"}
	}
	if cfg.Placeholders.Token == "" {
		cfg.Placeholders.Token = "${TOKEN}"
	}
	if cfg.Placeholders.ClientSecret == "" {
		cfg.Placeholders.ClientSecret = "${CLIENT_SECRET}"
	}
	if cfg.Placeholders.Endpoint == "" {
		cfg.Placeholders.Endpoint = "${ENDPOINT}"
	}

	return &Generator{
		cfg: cfg,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Document is the root generated meta config object.
type Document struct {
	Version     string      `json:"version" yaml:"version"`
	GeneratedAt time.Time   `json:"generated_at" yaml:"generated_at"`
	Source      string      `json:"source" yaml:"source"`
	Entries     []MetaEntry `json:"entries" yaml:"entries"`
	Notes       []string    `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// MetaEntry is a partial-but-valid configuration candidate.
type MetaEntry struct {
	ID         string            `json:"id" yaml:"id"`
	Name       string            `json:"name" yaml:"name"`
	Protocol   string            `json:"protocol" yaml:"protocol"`
	Transport  string            `json:"transport" yaml:"transport"`
	Endpoint   string            `json:"endpoint" yaml:"endpoint"`
	Auth       MetaAuth          `json:"auth" yaml:"auth"`
	Expected   MetaExpected      `json:"expected,omitempty" yaml:"expected,omitempty"`
	Monitoring MetaMonitoring    `json:"monitoring" yaml:"monitoring"`
	Labels     map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`

	// Optional annotations.
	Confidence *float64 `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	Provenance string   `json:"provenance,omitempty" yaml:"provenance,omitempty"`
}

// MetaAuth contains intentionally partial authentication material.
type MetaAuth struct {
	Type        string   `json:"type" yaml:"type"`
	Header      string   `json:"header,omitempty" yaml:"header,omitempty"`
	KeyName     string   `json:"key_name,omitempty" yaml:"key_name,omitempty"`
	Token       string   `json:"token,omitempty" yaml:"token,omitempty"`
	ClientID    string   `json:"client_id,omitempty" yaml:"client_id,omitempty"`
	TokenURL    string   `json:"token_url,omitempty" yaml:"token_url,omitempty"`
	RedirectURL string   `json:"redirect_url,omitempty" yaml:"redirect_url,omitempty"`
	CallbackID  string   `json:"callback_id,omitempty" yaml:"callback_id,omitempty"`
	Scopes      []string `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Audience    string   `json:"audience,omitempty" yaml:"audience,omitempty"`
	SecretRef   string   `json:"secret_ref,omitempty" yaml:"secret_ref,omitempty"`
}

// MetaExpected carries lightweight expectations for initial validation.
type MetaExpected struct {
	HealthyStatusCodes []int    `json:"healthy_status_codes,omitempty" yaml:"healthy_status_codes,omitempty"`
	MinCapabilities    []string `json:"min_capabilities,omitempty" yaml:"min_capabilities,omitempty"`
	KnownAgentCountMin int      `json:"known_agent_count_min,omitempty" yaml:"known_agent_count_min,omitempty"`
	ExpectedVersion    string   `json:"expected_version,omitempty" yaml:"expected_version,omitempty"`
}

// MetaMonitoring contains monitor settings that make the snippet executable.
type MetaMonitoring struct {
	Interval string `json:"interval" yaml:"interval"`
	Timeout  string `json:"timeout" yaml:"timeout"`
	Retries  int    `json:"retries" yaml:"retries"`
	Enabled  bool   `json:"enabled" yaml:"enabled"`
}

// GenerateFromDiscovery builds a document from discovery records.
func (g *Generator) GenerateFromDiscovery(records []discovery.Record) Document {
	entries := make([]MetaEntry, 0, len(records))

	for _, rec := range records {
		t := rec.Target
		entry := g.entryFromTarget(t)

		if g.cfg.IncludeConfidence {
			c := rec.Confidence
			entry.Confidence = &c
		}
		if g.cfg.IncludeProvenance {
			entry.Provenance = rec.Source
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})

	return Document{
		Version:     "1",
		GeneratedAt: g.now(),
		Source:      "discovery",
		Entries:     entries,
		Notes: []string{
			"Generated entries are partial but structurally valid.",
			"Replace placeholders with real values before production use.",
			"Validate each entry against target service expectations.",
		},
	}
}

// GenerateFromTargets builds a document from explicit target list.
func (g *Generator) GenerateFromTargets(ts []targets.Target) Document {
	entries := make([]MetaEntry, 0, len(ts))
	for _, t := range ts {
		entries = append(entries, g.entryFromTarget(t))
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})

	return Document{
		Version:     "1",
		GeneratedAt: g.now(),
		Source:      "targets",
		Entries:     entries,
		Notes: []string{
			"Generated entries are partial but structurally valid.",
			"Replace placeholders with real values before production use.",
		},
	}
}

// ValidateDocument performs lightweight structural checks for generated output.
func ValidateDocument(doc Document) error {
	if doc.Version == "" {
		return errors.New("document version is required")
	}
	if len(doc.Entries) == 0 {
		return errors.New("document must contain at least one entry")
	}

	seen := make(map[string]struct{}, len(doc.Entries))
	for i, e := range doc.Entries {
		prefix := fmt.Sprintf("entries[%d]", i)

		if strings.TrimSpace(e.ID) == "" {
			return fmt.Errorf("%s.id is required", prefix)
		}
		if _, ok := seen[e.ID]; ok {
			return fmt.Errorf("%s.id duplicate: %q", prefix, e.ID)
		}
		seen[e.ID] = struct{}{}

		if strings.TrimSpace(e.Protocol) == "" {
			return fmt.Errorf("%s.protocol is required", prefix)
		}
		if strings.TrimSpace(e.Transport) == "" {
			return fmt.Errorf("%s.transport is required", prefix)
		}
		if strings.TrimSpace(e.Endpoint) == "" {
			return fmt.Errorf("%s.endpoint is required", prefix)
		}
		if strings.TrimSpace(e.Monitoring.Interval) == "" {
			return fmt.Errorf("%s.monitoring.interval is required", prefix)
		}
		if strings.TrimSpace(e.Monitoring.Timeout) == "" {
			return fmt.Errorf("%s.monitoring.timeout is required", prefix)
		}
		if e.Monitoring.Retries < 0 {
			return fmt.Errorf("%s.monitoring.retries must be >= 0", prefix)
		}

		authType := strings.ToLower(strings.TrimSpace(e.Auth.Type))
		switch authType {
		case "", "none":
		case "bearer":
			if e.Auth.Header == "" {
				return fmt.Errorf("%s.auth.header required for bearer", prefix)
			}
			if e.Auth.Token == "" && e.Auth.SecretRef == "" {
				return fmt.Errorf("%s.auth.token or auth.secret_ref required for bearer", prefix)
			}
		case "apikey":
			if e.Auth.KeyName == "" {
				return fmt.Errorf("%s.auth.key_name required for apikey", prefix)
			}
			if e.Auth.Token == "" && e.Auth.SecretRef == "" {
				return fmt.Errorf("%s.auth.token or auth.secret_ref required for apikey", prefix)
			}
		case "oauth":
			if e.Auth.ClientID == "" {
				return fmt.Errorf("%s.auth.client_id required for oauth", prefix)
			}
			if e.Auth.TokenURL == "" {
				return fmt.Errorf("%s.auth.token_url required for oauth", prefix)
			}
		default:
			return fmt.Errorf("%s.auth.type unsupported value %q", prefix, e.Auth.Type)
		}
	}

	return nil
}

// Render serializes the document into yaml or json.
func Render(format string, doc Document) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "yaml", "yml":
		b, err := yaml.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("marshal yaml: %w", err)
		}
		return b, nil
	case "json":
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal json: %w", err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

// Write emits configured output formats to disk and returns created paths.
func (g *Generator) Write(ctx context.Context, doc Document) ([]string, error) {
	if err := ValidateDocument(doc); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(g.cfg.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	base := fmt.Sprintf("mcp-add-meta-%s", g.now().Format("20060102-150405"))
	formats := normalizedFormats(g.cfg.Formats)

	out := make([]string, 0, len(formats))
	for _, f := range formats {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		b, err := Render(f, doc)
		if err != nil {
			return out, err
		}

		ext := fileExt(f)
		path := filepath.Join(g.cfg.OutputDir, base+ext)
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return out, fmt.Errorf("write %s: %w", path, err)
		}
		out = append(out, path)
	}

	return out, nil
}

func (g *Generator) entryFromTarget(t targets.Target) MetaEntry {
	entry := MetaEntry{
		ID:        fallback(t.ID, "generated-target"),
		Name:      fallback(t.Name, t.ID),
		Protocol:  strings.ToLower(string(t.Protocol)),
		Transport: strings.ToLower(string(t.Transport)),
		Endpoint:  fallback(t.Endpoint, g.cfg.Placeholders.Endpoint),
		Auth:      g.authFromTarget(t.Auth),
		Expected: MetaExpected{
			HealthyStatusCodes: cloneInts(t.Expected.HealthyStatusCodes),
			MinCapabilities:    cloneStrings(t.Expected.MinCapabilities),
			KnownAgentCountMin: t.Expected.KnownAgentCountMin,
			ExpectedVersion:    t.Expected.ExpectedVersion,
		},
		Monitoring: MetaMonitoring{
			Interval: durationOrDefault(t.Check.Interval, "30s"),
			Timeout:  durationOrDefault(t.Check.Timeout, "5s"),
			Retries:  clampRetries(t.Check.Retries),
			Enabled:  t.Enabled,
		},
		Labels: map[string]string{
			"generated_by": "ocd-smoke-alarm",
			"intent":       "mcp-add-partial",
		},
	}

	// Keep empty expected fields compact.
	if len(entry.Expected.HealthyStatusCodes) == 0 &&
		len(entry.Expected.MinCapabilities) == 0 &&
		entry.Expected.KnownAgentCountMin == 0 &&
		entry.Expected.ExpectedVersion == "" {
		entry.Expected = MetaExpected{}
	}

	return entry
}

func (g *Generator) authFromTarget(a targets.AuthConfig) MetaAuth {
	authType := strings.ToLower(string(a.Type))
	if authType == "" {
		authType = "none"
	}

	out := MetaAuth{
		Type:        authType,
		Header:      a.Header,
		KeyName:     a.KeyName,
		ClientID:    a.ClientID,
		TokenURL:    a.TokenURL,
		RedirectURL: a.RedirectURL,
		CallbackID:  a.CallbackID,
		Scopes:      cloneStrings(a.Scopes),
		Audience:    a.Audience,
		SecretRef:   a.SecretRef,
	}

	switch authType {
	case "none":
		return out

	case "bearer":
		if out.Header == "" {
			out.Header = "Authorization"
		}
		// Use placeholder by default; do not emit actual token.
		if out.SecretRef == "" {
			out.Token = g.cfg.Placeholders.Token
		}
		return out

	case "apikey":
		if out.KeyName == "" {
			out.KeyName = "x-api-key"
		}
		if out.SecretRef == "" {
			out.Token = g.cfg.Placeholders.Token
		}
		return out

	case "oauth":
		if out.ClientID == "" {
			out.ClientID = "ocd-smoke-alarm"
		}
		if out.TokenURL == "" {
			out.TokenURL = "https://auth.example.com/oauth/token"
		}
		if out.RedirectURL == "" {
			out.RedirectURL = "http://localhost:8877/oauth/callback"
		}
		if out.CallbackID == "" {
			out.CallbackID = "generated-oauth-callback"
		}
		if out.SecretRef == "" {
			out.Token = g.cfg.Placeholders.ClientSecret
		}
		return out

	default:
		// Preserve unknown type with minimal placeholder semantics.
		if out.SecretRef == "" {
			out.Token = g.cfg.Placeholders.Token
		}
		return out
	}
}

func normalizedFormats(in []string) []string {
	set := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))

	for _, f := range in {
		ff := strings.ToLower(strings.TrimSpace(f))
		if ff == "yml" {
			ff = "yaml"
		}
		switch ff {
		case "yaml", "json":
			if _, ok := set[ff]; ok {
				continue
			}
			set[ff] = struct{}{}
			out = append(out, ff)
		}
	}

	if len(out) == 0 {
		return []string{"yaml", "json"}
	}
	sort.Strings(out)
	return out
}

func fileExt(format string) string {
	switch strings.ToLower(format) {
	case "yaml", "yml":
		return ".yaml"
	case "json":
		return ".json"
	default:
		return ".txt"
	}
}

func fallback(v, alt string) string {
	if strings.TrimSpace(v) == "" {
		return alt
	}
	return v
}

func durationOrDefault(d time.Duration, fallback string) string {
	if d <= 0 {
		return fallback
	}
	return d.String()
}

func clampRetries(v int) int {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return v
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneInts(in []int) []int {
	if len(in) == 0 {
		return nil
	}
	out := make([]int, len(in))
	copy(out, in)
	return out
}
