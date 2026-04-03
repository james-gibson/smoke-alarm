package discovery

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// Record is one discovered target with provenance metadata.
type Record struct {
	Target     targets.Target    `json:"target" yaml:"target"`
	Source     string            `json:"source" yaml:"source"`         // static_config | env | local_proxy_scan
	Confidence float64           `json:"confidence" yaml:"confidence"` // [0.0, 1.0]
	Evidence   map[string]string `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

// Result contains discovery outputs and non-fatal errors.
type Result struct {
	StartedAt  time.Time `json:"started_at" yaml:"started_at"`
	FinishedAt time.Time `json:"finished_at" yaml:"finished_at"`
	Records    []Record  `json:"records" yaml:"records"`
	Errors     []string  `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// Discoverer discovers monitorable targets from config, env, and local proxy scans.
type Discoverer struct {
	DialTimeout  time.Duration
	ProbeTimeout time.Duration
	httpClient   *http.Client
	now          func() time.Time
}

// Option customizes discoverer behavior.
type Option func(*Discoverer)

// WithDialTimeout sets TCP dial timeout for local proxy scans.
func WithDialTimeout(d time.Duration) Option {
	return func(ds *Discoverer) {
		if d > 0 {
			ds.DialTimeout = d
		}
	}
}

// WithProbeTimeout sets HTTP probe timeout.
func WithProbeTimeout(d time.Duration) Option {
	return func(ds *Discoverer) {
		if d > 0 {
			ds.ProbeTimeout = d
		}
	}
}

// WithHTTPClient injects a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(ds *Discoverer) {
		if c != nil {
			ds.httpClient = c
		}
	}
}

// New creates a discoverer with conservative defaults.
func New(opts ...Option) *Discoverer {
	ds := &Discoverer{
		DialTimeout:  250 * time.Millisecond,
		ProbeTimeout: 900 * time.Millisecond,
		now:          time.Now,
	}
	for _, opt := range opts {
		opt(ds)
	}
	if ds.httpClient == nil {
		ds.httpClient = &http.Client{
			Timeout: ds.ProbeTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        8,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     30 * time.Second,
				DisableKeepAlives:   false,
			},
		}
	}
	return ds
}

// Discover runs discovery across static config targets and enabled dynamic sources.
func (d *Discoverer) Discover(ctx context.Context, cfg config.Config) Result {
	started := d.now()
	out := Result{
		StartedAt: started,
		Records:   make([]Record, 0, len(cfg.Targets)+8),
		Errors:    []string{},
	}

	seen := map[string]struct{}{}

	// 1) Static config targets (deterministic baseline).
	for _, t := range cfg.Targets {
		if !t.Enabled {
			continue
		}
		rec := Record{
			Target:     toTargetFromConfig(t),
			Source:     "static_config",
			Confidence: 1.0,
			Evidence: map[string]string{
				"target_id": t.ID,
			},
		}
		key := dedupeKey(rec.Target)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out.Records = append(out.Records, rec)
	}

	// 2) Dynamic discovery.
	if cfg.Discovery.Enabled {
		envRecords, envErrs := d.discoverFromEnv(ctx, cfg)
		for _, e := range envErrs {
			out.Errors = append(out.Errors, e.Error())
		}
		for _, rec := range envRecords {
			key := dedupeKey(rec.Target)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out.Records = append(out.Records, rec)
		}

		lpRecords, lpErrs := d.discoverLocalProxies(ctx, cfg)
		for _, e := range lpErrs {
			out.Errors = append(out.Errors, e.Error())
		}
		for _, rec := range lpRecords {
			key := dedupeKey(rec.Target)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out.Records = append(out.Records, rec)
		}

		llmsRecords, llmsErrs := d.discoverFromLLMSTxt(ctx, cfg)
		for _, e := range llmsErrs {
			out.Errors = append(out.Errors, e.Error())
		}
		for _, rec := range llmsRecords {
			key := dedupeKey(rec.Target)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out.Records = append(out.Records, rec)
		}
	}

	sort.Slice(out.Records, func(i, j int) bool {
		return out.Records[i].Target.ID < out.Records[j].Target.ID
	})

	out.FinishedAt = d.now()
	return out
}

func (d *Discoverer) discoverFromEnv(ctx context.Context, cfg config.Config) ([]Record, []error) {
	records := []Record{}
	errs := []error{}

	for _, envName := range cfg.Discovery.IncludeEnvVars {
		if envName == "" {
			continue
		}
		val, ok := os.LookupEnv(envName)
		if !ok || strings.TrimSpace(val) == "" {
			continue
		}

		candidates := splitEnvTargets(val)
		for idx, c := range candidates {
			select {
			case <-ctx.Done():
				errs = append(errs, ctx.Err())
				return records, errs
			default:
			}

			tt, err := envCandidateToTarget(envName, c, idx)
			if err != nil {
				errs = append(errs, fmt.Errorf("env %s candidate %q: %w", envName, c, err))
				continue
			}
			records = append(records, Record{
				Target:     tt,
				Source:     "env",
				Confidence: 0.70,
				Evidence: map[string]string{
					"env_var": envName,
					"value":   c,
				},
			})
		}
	}

	return records, errs
}

func (d *Discoverer) discoverLocalProxies(ctx context.Context, cfg config.Config) ([]Record, []error) {
	records := []Record{}
	errs := []error{}

	scan := cfg.Discovery.LocalProxyScan
	if !scan.Enabled {
		return records, errs
	}

	hosts := scan.Hosts
	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1", "localhost"}
	}
	ports := scan.Ports
	if len(ports) == 0 {
		ports = []int{3000, 4317, 4318, 8080, 9000}
	}

	for _, host := range hosts {
		for _, port := range ports {
			select {
			case <-ctx.Done():
				errs = append(errs, ctx.Err())
				return records, errs
			default:
			}

			if port < 1 || port > 65535 {
				errs = append(errs, fmt.Errorf("invalid scan port: %d", port))
				continue
			}

			open, probeErr := d.isTCPPortOpen(ctx, host, port)
			if probeErr != nil {
				errs = append(errs, fmt.Errorf("tcp probe %s:%d: %w", host, port, probeErr))
				continue
			}
			if !open {
				continue
			}

			// Probe candidate endpoints with lightweight HTTP checks.
			candidates := []string{
				fmt.Sprintf("http://%s:%d/mcp", host, port),
				fmt.Sprintf("http://%s:%d/acp", host, port),
				fmt.Sprintf("http://%s:%d/healthz", host, port),
				fmt.Sprintf("http://%s:%d/", host, port),
			}

			for _, endpoint := range candidates {
				ok, status, err := d.httpReachable(ctx, endpoint)
				if err != nil {
					// non-fatal; continue scanning.
					continue
				}
				if !ok {
					continue
				}

				proto, confidence := inferProtoFromEndpoint(endpoint)
				id := safeID(fmt.Sprintf("discovered-%s-%d-%s", host, port, proto))

				rec := Record{
					Target: targets.Target{
						ID:        id,
						Enabled:   true,
						Protocol:  proto,
						Name:      fmt.Sprintf("Discovered %s %s:%d", strings.ToUpper(string(proto)), host, port),
						Endpoint:  endpoint,
						Transport: targets.TransportHTTP,
						Expected: targets.ExpectedBehavior{
							HealthyStatusCodes: []int{200, 204},
						},
						Auth: targets.AuthConfig{
							Type: targets.AuthNone,
						},
						Check: targets.CheckPolicy{
							Interval: mustDuration(cfg.Service.PollInterval, 15*time.Second),
							Timeout:  mustDuration(cfg.Service.Timeout, 5*time.Second),
							Retries:  1,
						},
						Tags: map[string]string{
							"discovered": "true",
							"host":       host,
							"port":       strconv.Itoa(port),
						},
					},
					Source:     "local_proxy_scan",
					Confidence: confidence,
					Evidence: map[string]string{
						"status_code": strconv.Itoa(status),
						"probe":       endpoint,
					},
				}
				records = append(records, rec)

				// If one endpoint proves an open service on this port, avoid producing many duplicates.
				break
			}
		}
	}

	return records, errs
}

var llmsLinkPattern = regexp.MustCompile(`^\s*-\s*\[([^\]]+)\]\(([^)]+)\)(?::\s*(.*))?\s*$`)

type llmsCandidate struct {
	Name     string
	URL      string
	Section  string
	Notes    string
	Optional bool
}

func (d *Discoverer) discoverFromLLMSTxt(ctx context.Context, cfg config.Config) ([]Record, []error) {
	records := []Record{}
	errs := []error{}

	llmsCfg := cfg.Discovery.LLMSTxt
	if !llmsCfg.Enabled || len(llmsCfg.RemoteURIs) == 0 {
		return records, errs
	}

	timeout := mustDuration(llmsCfg.FetchTimeout, 5*time.Second)

	for _, rawURI := range llmsCfg.RemoteURIs {
		uri := strings.TrimSpace(rawURI)
		if uri == "" {
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		content, fetchErr := d.fetchLLMSTxt(reqCtx, uri)
		cancel()
		if fetchErr != nil {
			errs = append(errs, fmt.Errorf("llms.txt fetch %q failed: %w", uri, fetchErr))
			continue
		}

		candidates, parseErr := parseLLMSTxtCandidates(uri, content)
		if parseErr != nil {
			errs = append(errs, fmt.Errorf("llms.txt parse %q failed: %w", uri, parseErr))
			continue
		}
		if len(candidates) == 0 {
			continue
		}
		if !llmsCfg.AutoRegisterAsTargets {
			continue
		}

		host := hostKey(uri)
		for i, c := range candidates {
			proto, confidence := inferProtocolFromLLMSCandidate(c)
			if proto != targets.ProtocolMCP && proto != targets.ProtocolACP {
				continue
			}

			transport := transportFromURL(c.URL)
			endpoint := c.URL
			if strings.TrimSpace(endpoint) == "" {
				continue
			}

			targetID := safeID(fmt.Sprintf("llms-%s-%d-%s", host, i, proto))
			targetName := strings.TrimSpace(c.Name)
			if targetName == "" {
				targetName = fmt.Sprintf("LLMS discovered %s target", strings.ToUpper(string(proto)))
			}

			authCfg := authFromLLMSCandidate(uri, c, proto, llmsCfg.AutoRegisterOAuth)

			rec := Record{
				Target: targets.Target{
					ID:        targetID,
					Enabled:   true,
					Protocol:  proto,
					Name:      targetName,
					Endpoint:  endpoint,
					Transport: transport,
					Expected: targets.ExpectedBehavior{
						HealthyStatusCodes: defaultStatusCodesForTransport(transport),
					},
					Auth: authCfg,
					Check: targets.CheckPolicy{
						Interval: mustDuration(cfg.Service.PollInterval, 15*time.Second),
						Timeout:  mustDuration(cfg.Service.Timeout, 5*time.Second),
						Retries:  1,
					},
					Tags: map[string]string{
						"discovered":      "true",
						"source":          "llms_txt",
						"llms_section":    c.Section,
						"llms_optional":   strconv.FormatBool(c.Optional),
						"llms_remote_uri": uri,
					},
					Meta: map[string]string{
						"llms_notes": c.Notes,
					},
				},
				Source:     "llms_txt",
				Confidence: confidence,
				Evidence: map[string]string{
					"remote_uri": uri,
					"section":    c.Section,
					"link_name":  c.Name,
					"link_url":   c.URL,
				},
			}
			records = append(records, rec)
		}
	}

	return records, errs
}

func (d *Discoverer) fetchLLMSTxt(ctx context.Context, uri string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return "", err
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func parseLLMSTxtCandidates(baseURI, content string) ([]llmsCandidate, error) {
	candidates := []llmsCandidate{}
	sc := bufio.NewScanner(strings.NewReader(content))

	section := ""
	optionalSection := false

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			optionalSection = strings.EqualFold(section, "Optional")
			continue
		}

		matches := llmsLinkPattern.FindStringSubmatch(line)
		if len(matches) != 4 {
			continue
		}

		name := strings.TrimSpace(matches[1])
		rawLink := strings.TrimSpace(matches[2])
		notes := strings.TrimSpace(matches[3])

		resolved, err := resolveLLMSLink(baseURI, rawLink)
		if err != nil {
			continue
		}

		if !candidateLooksLikeMCPOrACP(name, resolved, notes, section) {
			continue
		}

		candidates = append(candidates, llmsCandidate{
			Name:     name,
			URL:      resolved,
			Section:  section,
			Notes:    notes,
			Optional: optionalSection,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	return candidates, nil
}

func resolveLLMSLink(baseURI, rawLink string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(baseURI))
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(strings.TrimSpace(rawLink))
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme == "" || resolved.Host == "" {
		return "", fmt.Errorf("invalid resolved link %q", resolved.String())
	}
	if resolved.Scheme != "http" && resolved.Scheme != "https" && resolved.Scheme != "ws" && resolved.Scheme != "wss" {
		return "", fmt.Errorf("unsupported scheme %q", resolved.Scheme)
	}
	return resolved.String(), nil
}

func candidateLooksLikeMCPOrACP(name, link, notes, section string) bool {
	s := strings.ToLower(strings.Join([]string{name, link, notes, section}, " "))
	if strings.Contains(s, "mcp") || strings.Contains(s, "model context protocol") {
		return true
	}
	if strings.Contains(s, "acp") || strings.Contains(s, "agent client protocol") {
		return true
	}
	return false
}

func inferProtocolFromLLMSCandidate(c llmsCandidate) (targets.Protocol, float64) {
	text := strings.ToLower(strings.Join([]string{c.Name, c.URL, c.Notes, c.Section}, " "))
	switch {
	case strings.Contains(text, "acp"), strings.Contains(text, "agent client protocol"):
		return targets.ProtocolACP, 0.84
	case strings.Contains(text, "mcp"), strings.Contains(text, "model context protocol"):
		return targets.ProtocolMCP, 0.88
	default:
		return inferProtoFromEndpoint(c.URL)
	}
}

func authFromLLMSCandidate(remoteURI string, c llmsCandidate, proto targets.Protocol, autoOAuth bool) targets.AuthConfig {
	if !autoOAuth || !candidateHintsOAuth(c) {
		return targets.AuthConfig{Type: targets.AuthNone}
	}

	tokenURL := inferOAuthTokenURL(c.URL, remoteURI)
	scopes := defaultOAuthScopesForProtocol(proto)

	return targets.AuthConfig{
		Type:        targets.AuthOAuth,
		ClientID:    "ocd-smoke-alarm",
		TokenURL:    tokenURL,
		RedirectURL: "http://127.0.0.1:8877/oauth/callback",
		CallbackID:  fmt.Sprintf("%s-callback", safeID(c.Name)),
		Scopes:      scopes,
		SecretRef:   fmt.Sprintf("keychain://ocd-smoke-alarm/%s/client-secret", safeID(c.Name)),
	}
}

func candidateHintsOAuth(c llmsCandidate) bool {
	text := strings.ToLower(strings.Join([]string{c.Name, c.URL, c.Notes, c.Section}, " "))
	switch {
	case strings.Contains(text, "oauth"),
		strings.Contains(text, "oidc"),
		strings.Contains(text, "openid"),
		strings.Contains(text, "bearer"),
		strings.Contains(text, "token"),
		strings.Contains(text, "auth"):
		return true
	default:
		return false
	}
}

func inferOAuthTokenURL(endpoint, fallbackBase string) string {
	ep, err := url.Parse(strings.TrimSpace(endpoint))
	if err == nil && ep.Host != "" {
		scheme := strings.ToLower(ep.Scheme)
		switch scheme {
		case "http":
			return "http://" + ep.Host + "/oauth/token"
		case "https":
			return "https://" + ep.Host + "/oauth/token"
		case "ws":
			return "http://" + ep.Host + "/oauth/token"
		case "wss":
			return "https://" + ep.Host + "/oauth/token"
		}
	}

	base, err := url.Parse(strings.TrimSpace(fallbackBase))
	if err == nil && base.Host != "" {
		if strings.EqualFold(base.Scheme, "http") {
			return "http://" + base.Host + "/oauth/token"
		}
		return "https://" + base.Host + "/oauth/token"
	}

	return "https://example.com/oauth/token"
}

func defaultOAuthScopesForProtocol(proto targets.Protocol) []string {
	switch proto {
	case targets.ProtocolACP:
		return []string{"acp.read", "acp.execute"}
	case targets.ProtocolMCP:
		return []string{"mcp.read", "mcp.execute"}
	default:
		return []string{"api.read"}
	}
}

func transportFromURL(raw string) targets.Transport {
	u, err := url.Parse(raw)
	if err != nil {
		return targets.TransportHTTP
	}

	switch strings.ToLower(u.Scheme) {
	case "ws", "wss":
		return targets.TransportWebSocket
	case "http", "https":
		if looksLikeSSEEndpoint(u) {
			return targets.TransportSSE
		}
		return targets.TransportHTTP
	default:
		return targets.TransportHTTP
	}
}

func looksLikeSSEEndpoint(u *url.URL) bool {
	if u == nil {
		return false
	}

	path := strings.ToLower(strings.TrimSpace(u.Path))
	query := strings.ToLower(strings.TrimSpace(u.RawQuery))
	full := path + "?" + query

	// Common SSE naming conventions.
	if strings.Contains(full, "sse") ||
		strings.Contains(full, "event-stream") ||
		strings.Contains(full, "/events") ||
		strings.Contains(full, "/stream") {
		return true
	}

	return false
}

func hostKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "llms"
	}
	h := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if h == "" {
		return "llms"
	}
	return safeID(h)
}

func toTargetFromConfig(t config.TargetConfig) targets.Target {
	return targets.Target{
		ID:        t.ID,
		Enabled:   t.Enabled,
		Protocol:  targets.Protocol(strings.ToLower(t.Protocol)),
		Name:      t.Name,
		Endpoint:  t.Endpoint,
		Transport: targets.Transport(strings.ToLower(t.Transport)),
		Expected: targets.ExpectedBehavior{
			HealthyStatusCodes: t.Expected.HealthyStatusCodes,
			MinCapabilities:    t.Expected.MinCapabilities,
			KnownAgentCountMin: t.Expected.KnownAgentCountMin,
			ExpectedVersion:    t.Expected.ExpectedVersion,
		},
		Auth: targets.AuthConfig{
			Type:        targets.AuthType(strings.ToLower(t.Auth.Type)),
			Header:      t.Auth.Header,
			KeyName:     t.Auth.KeyName,
			SecretRef:   t.Auth.SecretRef,
			ClientID:    t.Auth.ClientID,
			TokenURL:    t.Auth.TokenURL,
			RedirectURL: t.Auth.RedirectURL,
			CallbackID:  t.Auth.CallbackID,
			Scopes:      t.Auth.Scopes,
		},
		Stdio: targets.StdioCommand{
			Command: t.Stdio.Command,
			Args:    t.Stdio.Args,
			Env:     t.Stdio.Env,
			Cwd:     t.Stdio.Cwd,
		},
		Check: targets.CheckPolicy{
			Interval:         mustDuration(t.Check.Interval, 15*time.Second),
			Timeout:          mustDuration(t.Check.Timeout, 5*time.Second),
			Retries:          t.Check.Retries,
			HandshakeProfile: t.Check.HandshakeProfile,
			RequiredMethods:  append([]string(nil), t.Check.RequiredMethods...),
			HURLTests:        mapHURLTestsFromConfig(t.Check.HURLTests),
		},
	}
}

func mapHURLTestsFromConfig(in []config.HURLTestConfig) []targets.HURLTest {
	if len(in) == 0 {
		return nil
	}

	out := make([]targets.HURLTest, 0, len(in))
	for _, ht := range in {
		out = append(out, targets.HURLTest{
			Name:     ht.Name,
			File:     ht.File,
			Endpoint: ht.Endpoint,
			Method:   ht.Method,
			Headers:  ht.Headers,
			Body:     ht.Body,
		})
	}
	return out
}

func splitEnvTargets(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func envCandidateToTarget(envName, raw string, idx int) (targets.Target, error) {
	if !strings.Contains(raw, "://") {
		// Allow host:port shorthand by assuming http.
		raw = "http://" + raw
	}
	if !strings.HasPrefix(raw, "http://") &&
		!strings.HasPrefix(raw, "https://") &&
		!strings.HasPrefix(raw, "ws://") &&
		!strings.HasPrefix(raw, "wss://") {
		return targets.Target{}, fmt.Errorf("unsupported endpoint scheme in %q", raw)
	}

	proto := inferProtoFromEnvOrURL(envName, raw)
	transport := targets.TransportHTTP
	if strings.HasPrefix(raw, "ws://") || strings.HasPrefix(raw, "wss://") {
		transport = targets.TransportWebSocket
	}

	id := safeID(fmt.Sprintf("env-%s-%d", strings.ToLower(envName), idx))
	return targets.Target{
		ID:        id,
		Enabled:   true,
		Protocol:  proto,
		Name:      fmt.Sprintf("Env discovered %s", envName),
		Endpoint:  raw,
		Transport: transport,
		Expected: targets.ExpectedBehavior{
			HealthyStatusCodes: defaultStatusCodesForTransport(transport),
		},
		Auth: targets.AuthConfig{
			Type: targets.AuthNone,
		},
		Check: targets.CheckPolicy{
			Interval: 20 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  1,
		},
		Tags: map[string]string{
			"discovered": "true",
			"env_var":    envName,
		},
	}, nil
}

func inferProtoFromEnvOrURL(envName, raw string) targets.Protocol {
	en := strings.ToLower(envName)
	ru := strings.ToLower(raw)

	switch {
	case strings.Contains(en, "acp") || strings.Contains(ru, "/acp"):
		return targets.ProtocolACP
	case strings.Contains(en, "mcp") || strings.Contains(ru, "/mcp"):
		return targets.ProtocolMCP
	default:
		return targets.ProtocolHTTP
	}
}

func inferProtoFromEndpoint(endpoint string) (targets.Protocol, float64) {
	e := strings.ToLower(endpoint)
	switch {
	case strings.Contains(e, "/acp"):
		return targets.ProtocolACP, 0.82
	case strings.Contains(e, "/mcp"):
		return targets.ProtocolMCP, 0.85
	default:
		return targets.ProtocolHTTP, 0.55
	}
}

func defaultStatusCodesForTransport(t targets.Transport) []int {
	switch t {
	case targets.TransportWebSocket:
		return []int{101}
	case targets.TransportSSE:
		// SSE endpoints typically return HTTP 200 with text/event-stream.
		return []int{200}
	default:
		return []int{200}
	}
}

func (d *Discoverer) isTCPPortOpen(ctx context.Context, host string, port int) (bool, error) { //nolint:unparam
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: d.DialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		var nerr net.Error
		if errors.As(err, &nerr) && nerr.Timeout() {
			return false, nil
		}
		// Connection refused / unreachable are normal outcomes for scanning.
		return false, nil
	}
	_ = conn.Close()
	return true, nil
}

func (d *Discoverer) httpReachable(ctx context.Context, endpoint string) (bool, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return false, 0, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Any HTTP response indicates a reachable service; keep classification later.
	return true, resp.StatusCode, nil
}

func dedupeKey(t targets.Target) string {
	return strings.ToLower(fmt.Sprintf("%s|%s|%s", t.Protocol, t.Transport, t.Endpoint))
}

func mustDuration(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func safeID(v string) string {
	v = strings.ToLower(v)
	v = strings.ReplaceAll(v, " ", "-")
	v = strings.ReplaceAll(v, "_", "-")
	v = strings.ReplaceAll(v, ".", "-")
	v = strings.ReplaceAll(v, "/", "-")
	v = strings.ReplaceAll(v, ":", "-")

	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "discovered-target"
	}
	return out
}
