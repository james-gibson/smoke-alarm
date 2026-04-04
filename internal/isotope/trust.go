package isotope

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	lezzDiscoveryPort = 19100
	lezzMDNSService   = "_lezz-demo._tcp"
)

// clusterEntry mirrors the ClusterInfo JSON published by lezz demo's /cluster endpoint.
type clusterEntry struct {
	AdhdMCP string `json:"adhd_mcp"`
}

// VerifyTrust determines the initial 42i trust rung for a registering isotope
// by checking the lezz cluster registry and confirming via mDNS. Both checks
// run concurrently; total wall time is capped at 4 seconds.
//
//   - Rung 6 (Certification)    — in lezz registry AND mDNS confirms
//   - Rung 4 (Higher Authority) — in lezz registry, mDNS unavailable/timeout
//   - Rung 2 (Harness Tools)    — self-registered, not found in any cluster
func VerifyTrust(ctx context.Context, endpoint string) int {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	regCh := make(chan bool, 1)
	mdnsCh := make(chan bool, 1)

	go func() { regCh <- checkRegistry(ctx, endpoint) }()
	go func() { mdnsCh <- confirmViaMDNS(ctx) }()

	registryMatch := <-regCh
	if !registryMatch {
		cancel() // no need to wait for mDNS
		return 2 // Harness Tools — self-registered
	}

	mdnsConfirmed := <-mdnsCh
	if mdnsConfirmed {
		return 6 // Certification — mDNS + registry verified
	}
	return 4 // Higher Authority — registry only
}

// checkRegistry queries the lezz demo cluster registry at the well-known local
// port and returns true if endpoint appears as an adhd_mcp address.
func checkRegistry(ctx context.Context, endpoint string) bool {
	url := fmt.Sprintf("http://127.0.0.1:%d/cluster", lezzDiscoveryPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	var m map[string]clusterEntry
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return false
	}
	for _, c := range m {
		if matchEndpoint(endpoint, c.AdhdMCP) {
			return true
		}
	}
	return false
}

// confirmViaMDNS browses for _lezz-demo._tcp to confirm a live lezz cluster
// is present on the LAN. Returns true on first discovery.
func confirmViaMDNS(ctx context.Context) bool {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return false
	}
	entries := make(chan *zeroconf.ServiceEntry, 2)
	if err := resolver.Browse(ctx, lezzMDNSService, "local.", entries); err != nil {
		return false
	}
	select {
	case <-entries:
		return true
	case <-ctx.Done():
		return false
	}
}

// matchEndpoint returns true when two endpoint strings refer to the same port,
// normalising away scheme, host, and path differences. Port-only matching is
// sufficient for single-host demo clusters; multi-host deployments may upgrade
// this to full host:port comparison when needed.
func matchEndpoint(a, b string) bool {
	return extractPort(a) != "" && extractPort(a) == extractPort(b)
}

// extractPort parses a URL-like string and returns the port component.
func extractPort(s string) string {
	// strip scheme
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	// strip path
	if slash := strings.IndexByte(s, '/'); slash >= 0 {
		s = s[:slash]
	}
	_, port, err := net.SplitHostPort(s)
	if err != nil {
		return ""
	}
	return port
}
