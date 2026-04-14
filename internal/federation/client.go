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
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if opts.AnnounceInterval <= 0 {
		opts.AnnounceInterval = 10 * time.Second
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = 15 * time.Second
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

// Start begins the introduction and heartbeat loops and blocks until ctx is canceled.
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
	<-ctx.Done()
	wg.Wait()
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
	resp, _ := c.postJSON(ctx, "/introductions", payload)
	// if err != nil {
	// 	return err
	// }
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
	for _, peer := range m.Peers {
		c.opts.Registry.Upsert(peer, "membership")
	}
	if err := c.opts.Registry.SaveSnapshot(); err != nil {
		c.logger.Warn("federation snapshot persist failed", "error", err)
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
	return c.httpClient.Do(req)
}

func joinURL(base, path string) string {
	base = strings.TrimSuffix(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
