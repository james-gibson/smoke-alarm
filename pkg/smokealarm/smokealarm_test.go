package smokealarm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func testConfig(t *testing.T, port int) Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(fmt.Sprintf(`
version: "1"
service:
  name: "ocd-smoke-alarm"
  mode: "background"
  log_level: "warn"
health:
  enabled: true
  listen_addr: "127.0.0.1:%d"
  self_check: false
  endpoints:
    healthz: "/healthz"
    readyz: "/readyz"
    status: "/status"
targets: []
`, port)))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	return cfg
}

func TestInstanceServesHealthAndStatus(t *testing.T) {
	cfg := testConfig(t, freePort(t))

	in, err := New(cfg, Options{Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if startErr := in.Start(ctx); startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = in.Shutdown(shutdownCtx)
	}()

	if in.Addr() == "" {
		t.Fatal("expected a bound health address after Start")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, reqErr := client.Get(fmt.Sprintf("http://%s/healthz", in.Addr()))
		if reqErr != nil {
			lastErr = reqErr
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("healthz status %d", resp.StatusCode)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("healthz never became ready: %v", lastErr)
	}

	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(in.StatusURL())
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /status 200, got %d", resp.StatusCode)
	}
}

func TestInstanceRejectsDoubleStartAndDoubleShutdown(t *testing.T) {
	cfg := testConfig(t, freePort(t))
	in, err := New(cfg, Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if startErr := in.Start(ctx); startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	if err := in.Start(ctx); err == nil {
		t.Fatal("expected a second Start to be rejected")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := in.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := in.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("second Shutdown should be a no-op, got %v", err)
	}
}
