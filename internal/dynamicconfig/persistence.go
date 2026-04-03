package dynamicconfig

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// Format constants for persisted dynamic config outputs.
const (
	FormatJSON     = "json"
	FormatMarkdown = "markdown"
)

// StoreOptions controls persistence behavior.
type StoreOptions struct {
	Directory        string
	Formats          []string // json|markdown
	ServeBaseURL     string
	AllowOverwrite   bool
	RequireUniqueIDs bool
	Now              func() time.Time
}

// Store persists discovery-generated dynamic configs.
type Store struct {
	mu   sync.Mutex
	opts StoreOptions
}

// SavedArtifact describes one persisted file and where it could be served from.
type SavedArtifact struct {
	ID         string    `json:"id"`
	Format     string    `json:"format"`
	Path       string    `json:"path"`
	ServeURL   string    `json:"serve_url,omitempty"`
	SavedAt    time.Time `json:"saved_at"`
	TargetID   string    `json:"target_id"`
	TargetName string    `json:"target_name,omitempty"`
}

// PersistedConfig is the canonical persisted payload.
type PersistedConfig struct {
	ID         string            `json:"id"`
	Version    string            `json:"version"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	Source     string            `json:"source"`
	Confidence float64           `json:"confidence"`
	Evidence   map[string]string `json:"evidence,omitempty"`
	Target     PersistedTarget   `json:"target"`
}

// PersistedTarget is a stable, export-friendly target schema.
type PersistedTarget struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Enabled   bool                   `json:"enabled"`
	Protocol  string                 `json:"protocol"`
	Transport string                 `json:"transport"`
	Endpoint  string                 `json:"endpoint"`
	Auth      PersistedTargetAuth    `json:"auth"`
	Check     PersistedTargetCheck   `json:"check"`
	Expected  PersistedTargetExpect  `json:"expected"`
	Tags      map[string]string      `json:"tags,omitempty"`
	Meta      map[string]string      `json:"meta,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
}

type PersistedTargetAuth struct {
	Type        string   `json:"type"`
	Header      string   `json:"header,omitempty"`
	KeyName     string   `json:"key_name,omitempty"`
	SecretRef   string   `json:"secret_ref,omitempty"`
	ClientID    string   `json:"client_id,omitempty"`
	TokenURL    string   `json:"token_url,omitempty"`
	RedirectURL string   `json:"redirect_url,omitempty"`
	CallbackID  string   `json:"callback_id,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	Audience    string   `json:"audience,omitempty"`
}

type PersistedTargetCheck struct {
	Interval         string                    `json:"interval"`
	Timeout          string                    `json:"timeout"`
	Retries          int                       `json:"retries"`
	HandshakeProfile string                    `json:"handshake_profile,omitempty"`
	RequiredMethods  []string                  `json:"required_methods,omitempty"`
	HURLTests        []PersistedTargetHURLTest `json:"hurl_tests,omitempty"`
}

type PersistedTargetHURLTest struct {
	Name     string            `json:"name"`
	File     string            `json:"file,omitempty"`
	Endpoint string            `json:"endpoint,omitempty"`
	Method   string            `json:"method,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     string            `json:"body,omitempty"`
}

type PersistedTargetExpect struct {
	HealthyStatusCodes []int    `json:"healthy_status_codes,omitempty"`
	MinCapabilities    []string `json:"min_capabilities,omitempty"`
	KnownAgentCountMin int      `json:"known_agent_count_min,omitempty"`
	ExpectedVersion    string   `json:"expected_version,omitempty"`
}

// NewStoreFromConfig builds a Store using dynamic_config settings from root config.
func NewStoreFromConfig(dc config.DynamicConfigConfig) *Store {
	return NewStore(StoreOptions{
		Directory:        dc.Directory,
		Formats:          dc.Formats,
		ServeBaseURL:     dc.ServeBaseURL,
		AllowOverwrite:   dc.AllowOverwrite,
		RequireUniqueIDs: dc.RequireUniqueIDs,
	})
}

// NewStore builds a dynamic config store.
func NewStore(opts StoreOptions) *Store {
	if strings.TrimSpace(opts.Directory) == "" {
		opts.Directory = "./state/dynamic-config"
	}
	opts.Formats = normalizeFormats(opts.Formats)
	if strings.TrimSpace(opts.ServeBaseURL) == "" {
		opts.ServeBaseURL = "/dynamic-config"
	}
	if !strings.HasPrefix(opts.ServeBaseURL, "/") {
		opts.ServeBaseURL = "/" + opts.ServeBaseURL
	}
	if opts.Now == nil {
		opts.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Store{opts: opts}
}

// SaveDiscoveryRecords persists each discovery record as one dynamic config payload.
// It returns every file artifact created (json/markdown per record depending on formats).
func (s *Store) SaveDiscoveryRecords(ctx context.Context, records []discovery.Record) ([]SavedArtifact, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(s.opts.Directory, 0o755); err != nil {
		return nil, fmt.Errorf("create dynamic config directory: %w", err)
	}

	out := make([]SavedArtifact, 0, len(records)*len(s.opts.Formats))
	for _, rec := range records {
		if err := ctx.Err(); err != nil {
			return out, err
		}

		cfgID := s.makeID(rec)
		p := toPersistedConfig(cfgID, rec, s.opts.Now())

		artifacts, err := s.savePersistedConfigLocked(ctx, p)
		if err != nil {
			return out, err
		}
		out = append(out, artifacts...)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ID == out[j].ID {
			return out[i].Format < out[j].Format
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// SaveDiscoveryRecord persists one discovery record.
func (s *Store) SaveDiscoveryRecord(ctx context.Context, rec discovery.Record) ([]SavedArtifact, error) {
	return s.SaveDiscoveryRecords(ctx, []discovery.Record{rec})
}

func (s *Store) savePersistedConfigLocked(ctx context.Context, p PersistedConfig) ([]SavedArtifact, error) {
	now := s.opts.Now()
	artifacts := make([]SavedArtifact, 0, len(s.opts.Formats))

	for _, f := range s.opts.Formats {
		if err := ctx.Err(); err != nil {
			return artifacts, err
		}

		var (
			path string
			body []byte
			err  error
		)

		switch f {
		case FormatJSON:
			path = filepath.Join(s.opts.Directory, p.ID+".json")
			body, err = json.MarshalIndent(p, "", "  ")
		case FormatMarkdown:
			path = filepath.Join(s.opts.Directory, p.ID+".md")
			body = []byte(RenderMarkdown(p))
		default:
			continue
		}
		if err != nil {
			return artifacts, fmt.Errorf("serialize dynamic config (%s): %w", f, err)
		}
		if err := s.writeFile(path, body); err != nil {
			return artifacts, err
		}

		artifacts = append(artifacts, SavedArtifact{
			ID:         p.ID,
			Format:     f,
			Path:       path,
			ServeURL:   s.serveURLFor(path),
			SavedAt:    now,
			TargetID:   p.Target.ID,
			TargetName: p.Target.Name,
		})
	}

	return artifacts, nil
}

func (s *Store) writeFile(path string, body []byte) error {
	if _, err := os.Stat(path); err == nil && !s.opts.AllowOverwrite {
		return fmt.Errorf("dynamic config exists and overwrite disabled: %s", path)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return fmt.Errorf("write temp dynamic config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit dynamic config file: %w", err)
	}
	return nil
}

func (s *Store) makeID(rec discovery.Record) string {
	base := strings.TrimSpace(rec.Target.ID)
	if base == "" {
		base = makeStableTargetID(rec.Target)
	}
	base = sanitizeID(base)

	if !s.opts.RequireUniqueIDs {
		return base
	}

	// Require uniqueness by appending short hash from source + endpoint + protocol.
	fingerprint := rec.Source + "|" + rec.Target.Endpoint + "|" + string(rec.Target.Protocol)
	sum := sha1.Sum([]byte(fingerprint)) // deterministic, low-overhead, non-security use
	short := hex.EncodeToString(sum[:])[:10]
	return sanitizeID(base + "-" + short)
}

func makeStableTargetID(t targets.Target) string {
	raw := fmt.Sprintf("%s-%s-%s", t.Protocol, t.Transport, t.Endpoint)
	sum := sha1.Sum([]byte(raw))
	return "dynamic-" + hex.EncodeToString(sum[:])[:12]
}

func sanitizeID(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "dynamic-config"
	}
	var b strings.Builder
	prevDash := false
	for _, r := range v {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "dynamic-config"
	}
	return out
}

func (s *Store) serveURLFor(path string) string {
	base := strings.TrimRight(s.opts.ServeBaseURL, "/")
	name := filepath.Base(path)
	return base + "/" + name
}

func normalizeFormats(in []string) []string {
	set := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, f := range in {
		ff := strings.ToLower(strings.TrimSpace(f))
		switch ff {
		case "json":
			ff = FormatJSON
		case "markdown", "md":
			ff = FormatMarkdown
		default:
			continue
		}
		if _, ok := set[ff]; ok {
			continue
		}
		set[ff] = struct{}{}
		out = append(out, ff)
	}
	if len(out) == 0 {
		return []string{FormatJSON, FormatMarkdown}
	}
	sort.Strings(out)
	return out
}

func toPersistedConfig(id string, rec discovery.Record, now time.Time) PersistedConfig {
	t := rec.Target
	return PersistedConfig{
		ID:         id,
		Version:    "1",
		CreatedAt:  now,
		UpdatedAt:  now,
		Source:     rec.Source,
		Confidence: rec.Confidence,
		Evidence:   cloneMap(rec.Evidence),
		Target: PersistedTarget{
			ID:        t.ID,
			Name:      t.Name,
			Enabled:   t.Enabled,
			Protocol:  string(t.Protocol),
			Transport: string(t.Transport),
			Endpoint:  t.Endpoint,
			Auth: PersistedTargetAuth{
				Type:        string(t.Auth.Type),
				Header:      t.Auth.Header,
				KeyName:     t.Auth.KeyName,
				SecretRef:   t.Auth.SecretRef,
				ClientID:    t.Auth.ClientID,
				TokenURL:    t.Auth.TokenURL,
				RedirectURL: t.Auth.RedirectURL,
				CallbackID:  t.Auth.CallbackID,
				Scopes:      append([]string(nil), t.Auth.Scopes...),
				Audience:    t.Auth.Audience,
			},
			Check: PersistedTargetCheck{
				Interval:         t.Check.Interval.String(),
				Timeout:          t.Check.Timeout.String(),
				Retries:          t.Check.Retries,
				HandshakeProfile: t.Check.HandshakeProfile,
				RequiredMethods:  append([]string(nil), t.Check.RequiredMethods...),
				HURLTests:        toPersistedHURLTests(t.Check.HURLTests),
			},
			Expected: PersistedTargetExpect{
				HealthyStatusCodes: append([]int(nil), t.Expected.HealthyStatusCodes...),
				MinCapabilities:    append([]string(nil), t.Expected.MinCapabilities...),
				KnownAgentCountMin: t.Expected.KnownAgentCountMin,
				ExpectedVersion:    t.Expected.ExpectedVersion,
			},
			Tags: cloneMap(t.Tags),
			Meta: cloneMap(t.Meta),
		},
	}
}

func toPersistedHURLTests(in []targets.HURLTest) []PersistedTargetHURLTest {
	if len(in) == 0 {
		return nil
	}
	out := make([]PersistedTargetHURLTest, 0, len(in))
	for _, ht := range in {
		out = append(out, PersistedTargetHURLTest{
			Name:     ht.Name,
			File:     ht.File,
			Endpoint: ht.Endpoint,
			Method:   ht.Method,
			Headers:  cloneMap(ht.Headers),
			Body:     ht.Body,
		})
	}
	return out
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// RenderMarkdown returns a markdown representation suitable for static hosting.
func RenderMarkdown(p PersistedConfig) string {
	var b strings.Builder
	b.WriteString("# Dynamic Config: " + p.ID + "\n\n")
	b.WriteString(fmt.Sprintf("- **Version:** `%s`\n", p.Version))
	b.WriteString(fmt.Sprintf("- **Created:** `%s`\n", p.CreatedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **Source:** `%s`\n", p.Source))
	b.WriteString(fmt.Sprintf("- **Confidence:** `%.2f`\n", p.Confidence))
	b.WriteString("\n## Target\n\n")
	b.WriteString(fmt.Sprintf("- **ID:** `%s`\n", p.Target.ID))
	b.WriteString(fmt.Sprintf("- **Name:** `%s`\n", p.Target.Name))
	b.WriteString(fmt.Sprintf("- **Protocol:** `%s`\n", p.Target.Protocol))
	b.WriteString(fmt.Sprintf("- **Transport:** `%s`\n", p.Target.Transport))
	b.WriteString(fmt.Sprintf("- **Endpoint:** `%s`\n", p.Target.Endpoint))
	b.WriteString(fmt.Sprintf("- **Auth Type:** `%s`\n", p.Target.Auth.Type))
	b.WriteString(fmt.Sprintf("- **Handshake Profile:** `%s`\n", p.Target.Check.HandshakeProfile))
	if len(p.Target.Check.RequiredMethods) > 0 {
		b.WriteString("- **Required Methods:** `" + strings.Join(p.Target.Check.RequiredMethods, "`, `") + "`\n")
	}
	if len(p.Evidence) > 0 {
		b.WriteString("\n## Evidence\n\n")
		keys := make([]string, 0, len(p.Evidence))
		for k := range p.Evidence {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("- **%s:** `%s`\n", k, p.Evidence[k]))
		}
	}
	return b.String()
}

// Validate checks store options for obvious misconfiguration.
func (s *Store) Validate() error {
	if strings.TrimSpace(s.opts.Directory) == "" {
		return errors.New("directory is required")
	}
	if len(s.opts.Formats) == 0 {
		return errors.New("at least one format is required")
	}
	if strings.TrimSpace(s.opts.ServeBaseURL) == "" {
		return errors.New("serve base URL is required")
	}
	return nil
}
