package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// holdPort keeps a loopback port bound for the rest of the test.
func holdPort(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind occupier: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi %q: %v", portStr, err)
	}
	return port
}

// TestBindWithRetryYieldsHeldPort is the regression for smoke-alarm#53: holding
// the callback port used to leave the OAuth surface absent while /status still
// reported the process live.
func TestBindWithRetryYieldsHeldPort(t *testing.T) {
	held := holdPort(t)
	srv := NewMockRedirectServer(MockRedirectOptions{ListenAddr: held, Mode: MockRedirectAllow})

	bound, err := srv.BindWithRetry(5)
	if err != nil {
		t.Fatalf("BindWithRetry: %v", err)
	}
	if bound == held {
		t.Fatalf("bound the held address %q", held)
	}
	if got, want := portOf(t, bound), portOf(t, held)+1; got != want {
		t.Fatalf("bound port %d, want %d", got, want)
	}
	if want := fmt.Sprintf("http://%s/oauth/callback", bound); srv.CallbackURL() != want {
		t.Fatalf("CallbackURL() = %q, want %q (the callback must follow the bound port)", srv.CallbackURL(), want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// The callback is served on the bound port, and the endpoint reports it.
	resp, err := http.Get(srv.CallbackURL() + "?state=s1") //nolint:noctx
	if err != nil {
		t.Fatalf("GET callback on bound address: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback returned %d, want 200", resp.StatusCode)
	}
	if srv.Hits() != 1 {
		t.Fatalf("hits = %d, want 1", srv.Hits())
	}

	statusResp, err := http.Get(fmt.Sprintf("http://%s/oauth/mock/status", bound)) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /oauth/mock/status: %v", err)
	}
	defer func() { _ = statusResp.Body.Close() }()
	var status map[string]any
	if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
		t.Fatalf("decode /oauth/mock/status: %v", err)
	}
	if got := status["listen"]; got != bound {
		t.Fatalf("/oauth/mock/status listen = %v, want %q", got, bound)
	}
	if got := status["callback"]; got != fmt.Sprintf("http://%s/oauth/callback", bound) {
		t.Fatalf("/oauth/mock/status callback = %v, want the bound callback", got)
	}
}

func TestAddrFallsBackToConfigured(t *testing.T) {
	srv := NewMockRedirectServer(MockRedirectOptions{ListenAddr: "localhost:28877"})
	if srv.Addr() != "localhost:28877" {
		t.Fatalf("Addr() = %q before binding, want the configured address", srv.Addr())
	}
}

func TestBindWithRetryReportsExhaustedRange(t *testing.T) {
	held := holdPort(t)
	srv := NewMockRedirectServer(MockRedirectOptions{ListenAddr: held})

	_, err := srv.BindWithRetry(-1)
	if err == nil {
		t.Fatal("expected an error when no retry is allowed and the port is held")
	}
	if !strings.Contains(err.Error(), held) {
		t.Fatalf("error %q does not name the configured address %q", err, held)
	}
}
