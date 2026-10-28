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
	// Above the held port, not necessarily the next one: another process may hold
	// that, in which case scanning past it is correct (smoke-alarm#74).
	if got := portOf(t, bound); got <= portOf(t, held) {
		t.Fatalf("bound port %d, want a port above the held %d", got, portOf(t, held))
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

// TestBindListenerServesTheListenerItWasGiven covers the seam the integration
// tests use (smoke-alarm#70): a caller that must know the address before the
// server starts binds a listener itself and hands it over, so the port is never
// released in between.
func TestBindListenerServesTheListenerItWasGiven(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	bound := ln.Addr().String()
	srv := NewServer(Options{ListenAddr: bound, EnableHTTP: true})

	got, err := srv.BindListener(ln)
	if err != nil {
		t.Fatalf("BindListener: %v", err)
	}
	if got != bound {
		t.Fatalf("BindListener returned %q, want the listener's address %q", got, bound)
	}
	if srv.Addr() != bound {
		t.Fatalf("Addr() = %q, want %q", srv.Addr(), bound)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	resp, err := http.Get(fmt.Sprintf("http://%s/hosted/status", bound)) //nolint:noctx
	if err != nil {
		t.Fatalf("GET /hosted/status on the held address: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode /hosted/status: %v", err)
	}
	if listen := status["listen"]; listen != bound {
		t.Fatalf("/hosted/status listen = %v, want %q", listen, bound)
	}
}

// TestBindListenerRejectsUnusableListener pins the guards: a nil listener is a
// programming error rather than a silent fallback to ListenAndServe, and a
// listener cannot replace one the server is already serving.
func TestBindListenerRejectsUnusableListener(t *testing.T) {
	srv := NewServer(Options{})
	if _, err := srv.BindListener(nil); err == nil {
		t.Fatal("BindListener(nil) returned no error")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind listener: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	started := NewServer(Options{})
	started.started.Store(true) // white-box: a start that has already happened
	if _, err := started.BindListener(ln); err == nil {
		t.Fatal("BindListener accepted a listener after the server started")
	}
}
