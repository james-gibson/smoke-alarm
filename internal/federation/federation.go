package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/health"
)

type Poller struct {
	mu           sync.RWMutex
	downstream   []string
	upstream     string
	rank         int
	pollInterval time.Duration
	client       *http.Client
	lastStatus   map[string]health.TargetStatus
}

func NewPoller(downstream []string, upstream string, rank int, pollInterval time.Duration) *Poller {
	return &Poller{
		downstream:   downstream,
		upstream:     upstream,
		rank:         rank,
		pollInterval: pollInterval,
		client:       &http.Client{Timeout: 10 * time.Second},
		lastStatus:   make(map[string]health.TargetStatus),
	}
}

func (p *Poller) Start(ctx context.Context, updateFn func([]health.TargetStatus)) {
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	p.pollOnce(ctx, updateFn)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce(ctx, updateFn)
		}
	}
}

func (p *Poller) pollOnce(ctx context.Context, updateFn func([]health.TargetStatus)) {
	var allTargets []health.TargetStatus

	for _, endpoint := range p.downstream {
		targets := p.fetchStatus(ctx, endpoint)
		slog.Debug("federation poll", "endpoint", endpoint, "targets", len(targets))
		for i := range targets {
			targets[i].ID = fmt.Sprintf("[%s] %s", endpoint, targets[i].ID)
			slog.Debug("federation target namespaced", "id", targets[i].ID)
		}
		allTargets = append(allTargets, targets...)
	}

	p.mu.Lock()
	p.lastStatus = make(map[string]health.TargetStatus)
	for _, t := range allTargets {
		p.lastStatus[t.ID] = t
	}
	p.mu.Unlock()

	slog.Debug("federation poll complete", "total", len(allTargets))
	updateFn(allTargets)
}

func (p *Poller) fetchStatus(ctx context.Context, endpoint string) []health.TargetStatus {
	rawURL := fmt.Sprintf("http://%s/status", endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return []health.TargetStatus{{
			ID:       "federation-error",
			State:    "unhealthy",
			Severity: "critical",
			Message:  fmt.Sprintf("failed to build request for %s: %v", endpoint, err),
		}}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return []health.TargetStatus{{
			ID:       "federation-error",
			State:    "unhealthy",
			Severity: "critical",
			Message:  fmt.Sprintf("failed to fetch %s: %v", endpoint, err),
		}}
	}
	defer func() { _ = resp.Body.Close() }()

	var status health.StatusResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&status); decodeErr != nil {
		return []health.TargetStatus{{
			ID:       "federation-error",
			State:    "unhealthy",
			Severity: "critical",
			Message:  fmt.Sprintf("failed to decode %s: %v", endpoint, decodeErr),
		}}
	}

	return status.Targets
}

func (p *Poller) ReportToUpstream(ctx context.Context, status health.StatusResponse) error {
	if p.upstream == "" {
		return nil
	}

	body, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("report to upstream: marshal: %w", err)
	}

	url := fmt.Sprintf("http://%s/federation/report", p.upstream)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("report to upstream: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("report to upstream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return nil
}

func DetectRank(listenAddr string) int {
	_, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(portStr)
	return port
}

type endpointWithPort struct {
	endpoint string
	port     int
}

func SortEndpoints(endpoints []string) []string {
	weighted := make([]endpointWithPort, len(endpoints))
	for i, ep := range endpoints {
		var port int
		if _, portStr, err := net.SplitHostPort(ep); err == nil {
			port, _ = strconv.Atoi(portStr)
		}
		weighted[i] = endpointWithPort{endpoint: ep, port: port}
	}
	sort.Slice(weighted, func(i, j int) bool {
		return weighted[i].port < weighted[j].port
	})
	result := make([]string, len(weighted))
	for i, w := range weighted {
		result[i] = w.endpoint
	}
	return result
}
