package hosted

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

// TestBindWithRetryYieldsHeldPort proves the hosted surface moves instead of
// disappearing when its configured port is already held (smoke-alarm#53).
func TestBindWithRetryYieldsHeldPort(t *testing.T) {
	held := holdPort(t)
	srv := NewServer(Options{ListenAddr: held})

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
	if srv.Addr() != bound {
		t.Fatalf("Addr() = %q, want the bound address %q", srv.Addr(), bound)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// The surface is usable on the port it claims, and says so itself.
	resp, err := http.Get(fmt.Sprintf("http://%s/hosted/status", bound)) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /hosted/status on bound address: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode /hosted/status: %v", err)
	}
	if got := status["listen"]; got != bound {
		t.Fatalf("/hosted/status listen = %v, want %q", got, bound)
	}
}

func TestAddrFallsBackToConfigured(t *testing.T) {
	srv := NewServer(Options{ListenAddr: "localhost:18091"})
	if srv.Addr() != "localhost:18091" {
		t.Fatalf("Addr() = %q before binding, want the configured address", srv.Addr())
	}
}

func TestBindWithRetryReportsExhaustedRange(t *testing.T) {
	held := holdPort(t)
	srv := NewServer(Options{ListenAddr: held})

	_, err := srv.BindWithRetry(-1)
	if err == nil {
		t.Fatal("expected an error when no retry is allowed and the port is held")
	}
	if msg := err.Error(); !strings.Contains(msg, held) {
		t.Fatalf("error %q does not name the configured address %q", msg, held)
	}
	if srv.Addr() != held {
		t.Fatalf("Addr() = %q after a failed bind, want the configured address", srv.Addr())
	}
}
