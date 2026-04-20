package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/auth"
)

func TestMockRedirectServer_AllowMode(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddrOAuth(t)
	srv := auth.NewMockRedirectServer(auth.MockRedirectOptions{
		ListenAddr:      addr,
		Path:            "/oauth/callback",
		Mode:            auth.MockRedirectAllow,
		ShutdownTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- srv.Start(ctx)
	}()

	waitForStatusOAuth(t, "http://"+addr+"/oauth/mock/status", http.StatusOK, 3*time.Second)

	resp, err := http.Get("http://" + addr + "/oauth/callback?code=abc123&state=s1&callback_id=cb-allow-1")
	if err != nil {
		t.Fatalf("oauth callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 from allow mode callback, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode callback payload: %v", err)
	}

	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, payload=%v", payload)
	}
	if mode, _ := payload["mode"].(string); mode != "allow" {
		t.Fatalf("expected mode=allow, got %q", mode)
	}
	if cb, _ := payload["callback_id"].(string); cb != "cb-allow-1" {
		t.Fatalf("expected callback_id=cb-allow-1, got %q", cb)
	}
	if code, _ := payload["code"].(string); code != "abc123" {
		t.Fatalf("expected code=abc123, got %q", code)
	}

	if got := srv.Hits(); got < 1 {
		t.Fatalf("expected hits >= 1, got %d", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("mock redirect server did not shut down in time")
	}
}

func TestMockRedirectServer_FailMode(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddrOAuth(t)
	srv := auth.NewMockRedirectServer(auth.MockRedirectOptions{
		ListenAddr:      addr,
		Path:            "/oauth/callback",
		Mode:            auth.MockRedirectFail,
		ShutdownTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- srv.Start(ctx)
	}()

	waitForStatusOAuth(t, "http://"+addr+"/oauth/mock/status", http.StatusOK, 3*time.Second)

	resp, err := http.Get("http://" + addr + "/oauth/callback?state=s2&callback_id=cb-fail-1")
	if err != nil {
		t.Fatalf("oauth callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 from fail mode callback, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode callback payload: %v", err)
	}

	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false, payload=%v", payload)
	}
	if mode, _ := payload["mode"].(string); mode != "fail" {
		t.Fatalf("expected mode=fail, got %q", mode)
	}
	if errCode, _ := payload["error"].(string); errCode == "" {
		t.Fatalf("expected non-empty error code in payload: %v", payload)
	}
	if cb, _ := payload["callback_id"].(string); cb != "cb-fail-1" {
		t.Fatalf("expected callback_id=cb-fail-1, got %q", cb)
	}

	if got := srv.Hits(); got < 1 {
		t.Fatalf("expected hits >= 1, got %d", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("mock redirect server did not shut down in time")
	}
}

func waitForStatusOAuth(t *testing.T, url string, want int, timeout time.Duration) {
	t.Helper()

	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(timeout)

	var lastErr error
	var lastCode int

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(25 * time.Millisecond)
			continue
		}
		lastCode = resp.StatusCode
		_ = resp.Body.Close()
		if resp.StatusCode == want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("wait for status=%d failed: %v", want, lastErr)
	}
	t.Fatalf("wait for status=%d failed: last code=%d", want, lastCode)
}

func freeTCPAddrOAuth(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}
