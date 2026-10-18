package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/health"
)

// ClientOptions configures the federation follower client behavior.
type ClientOptions struct {
	IntroducerURL     string
	Registry          *Registry
	HTTPClient        *http.Client
	Logger            *slog.Logger
	AnnounceInterval  time.Duration
	HeartbeatInterval time.Duration
	SystemClock       func() time.Time
	// SharedSecret authenticates requests to the introducer. Must match the
	// introducer's configured secret.
	SharedSecret string
	// StatusFunc supplies this node's local status. When set, the client
	// reports it to the introducer on ReportInterval so an alert here reaches
	// the rest of the cluster.
	StatusFunc func() health.StatusResponse
	// ReportInterval is the status report cadence. Defaults to the heartbeat
	// interval when unset.
	ReportInterval time.Duration
	// StatusSink receives the aggregated peer status returned by the
	// introducer, so this node's view is cluster-wide rather than local-only.
	StatusSink *PeerStatusSink
	// PeerStatusTimeout marks a peer's status stale when unreported for this
	// long. Zero disables staleness marking.
	PeerStatusTimeout time.Duration
}

// Client handles introduction and heartbeat flows against the elected introducer.
type Client struct {
	opts        ClientOptions
	httpClient  *http.Client
	logger      *slog.Logger
	introduced  atomic.Bool
	lastVersion atomic.Uint64
}

// NewClient constructs a follower client that reports to an introducer.
func NewClient(opts ClientOptions) (*Client, error) {
	if strings.TrimSpace(opts.IntroducerURL) == "" {
		return nil, errors.New("federation: introducer URL is required")
	}
	if opts.Registry == nil {
		return nil, errors.New("federation: registry is required")
	}
	// Fail closed: reporting status into a cluster we cannot authenticate to
	// would either be rejected (wasted work) or accepted by a host that is not
	// our introducer.
	if opts.SharedSecret == "" && !IsLoopbackAddr(opts.IntroducerURL) {
		return nil, ErrNoSharedSecret
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if opts.AnnounceInterval <= 0 {
		opts.AnnounceInterval = 10 * time.Second
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = 15 * time.Second
	}
	if opts.ReportInterval <= 0 {
		opts.ReportInterval = opts.HeartbeatInterval
	}
	if opts.SystemClock == nil {
		opts.SystemClock = time.Now
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Client{
		opts:       opts,
		httpClient: opts.HTTPClient,
		logger:     logger,
	}, nil
}

// Start begins the introduction, heartbeat and status-report loops and blocks
// until ctx is canceled.
func (c *Client) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.runAnnouncements(ctx)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.runHeartbeats(ctx)
	}()
	if c.opts.StatusFunc != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.runReports(ctx)
		}()
	}
	<-ctx.Done()
	wg.Wait()
}

// runReports pushes this node's local status to the introducer on the report
// cadence. This is the link that lets one alarm learn of another's state: the
// introducer aggregates reports and hands the whole set back on every
// membership response, so an alert raised here reaches every follower.
func (c *Client) runReports(ctx context.Context) {
	ticker := time.NewTicker(c.opts.ReportInterval)
	defer ticker.Stop()

	for {
		if c.introduced.Load() {
			if err := c.sendStatusReport(ctx); err != nil {
				c.logger.Warn("federation status report failed", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Client) sendStatusReport(ctx context.Context) error {
	report := StatusReport{
		InstanceID:  c.opts.Registry.Self().ID,
		ServiceName: c.opts.Registry.Self().ServiceName,
		ReportedAt:  c.opts.SystemClock().UTC(),
		Status:      c.opts.StatusFunc(),
	}

	resp, err := c.postJSON(ctx, "/reports", report)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("federation status report unexpected status: %s", resp.Status)
	}
	return nil
}

func (c *Client) runAnnouncements(ctx context.Context) {
	ticker := time.NewTicker(c.opts.AnnounceInterval)
	defer ticker.Stop()

	for {
		if err := c.sendIntroduction(ctx); err == nil {
			c.introduced.Store(true)
		} else {
			c.logger.Warn("federation introduction failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Client) runHeartbeats(ctx context.Context) {
	ticker := time.NewTicker(c.opts.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !c.introduced.Load() {
				continue
			}
			if err := c.sendHeartbeat(ctx); err != nil {
				c.logger.Warn("federation heartbeat failed", "error", err)
			}
		}
	}
}

func (c *Client) sendIntroduction(ctx context.Context) error {
	record := c.opts.Registry.Self()
	record.Role = RoleFollower
	record.AnnouncedAt = c.opts.SystemClock().UTC()

	payload := introductionRequest{
		Record: record,
	}
	resp, err := c.postJSON(ctx, "/introductions", payload)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("federation introduction unexpected status: %s", resp.Status)
	}

	var membership membershipResponse
	if err := json.NewDecoder(resp.Body).Decode(&membership); err != nil {
		return fmt.Errorf("federation introduction decode: %w", err)
	}
	c.applyMembership(membership)
	return nil
}

func (c *Client) sendHeartbeat(ctx context.Context) error {
	record := c.opts.Registry.Self()
	record.LastSeenAt = c.opts.SystemClock().UTC()

	resp, err := c.postJSON(ctx, "/heartbeats", record)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("federation heartbeat unexpected status: %s", resp.Status)
	}

	var membership membershipResponse
	if err := json.NewDecoder(resp.Body).Decode(&membership); err != nil {
		return fmt.Errorf("federation heartbeat decode: %w", err)
	}
	c.applyMembership(membership)
	return nil
}

func (c *Client) applyMembership(m membershipResponse) {
	if m.Version > c.lastVersion.Load() {
		c.lastVersion.Store(m.Version)
	}
	// Seed the introducer as a peer so followers see (and can trust) the node
	// that fronted them; self upserts are ignored by the registry.
	c.opts.Registry.Upsert(m.Self, "membership")
	for _, peer := range m.Peers {
		c.opts.Registry.Upsert(peer, "membership")
	}
	if err := c.opts.Registry.SaveSnapshot(); err != nil {
		c.logger.Warn("federation snapshot persist failed", "error", err)
	}

	// Render the cluster-wide status the introducer aggregated. This is what
	// makes an alert on one alarm visible on the others.
	if c.opts.StatusSink != nil {
		c.opts.StatusSink.ApplyAll(m.Statuses, c.opts.SystemClock().UTC(), c.opts.PeerStatusTimeout)
	}
}

func (c *Client) postJSON(ctx context.Context, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := joinURL(c.opts.IntroducerURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if c.opts.SharedSecret != "" {
		req.Header.Set(secretHeader, c.opts.SharedSecret)
	}
	return c.httpClient.Do(req)
}

func joinURL(base, path string) string {
	base = strings.TrimSuffix(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
