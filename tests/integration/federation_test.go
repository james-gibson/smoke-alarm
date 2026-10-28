package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type federationInstance struct {
	name       string
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	stdout     *bytes.Buffer
	stderr     *bytes.Buffer
	healthAddr string
	stopped    bool

	// done is closed once cmd.Wait has returned; waitErr is set before the
	// close. Wait may only be called once, and both Stop and the readiness
	// waiters need its result, so the instance owns that single call.
	done    chan struct{}
	mu      sync.Mutex
	waitErr error
}

type federationStartOptions struct {
	HealthAddr   string
	ReadyzPath   string
	StartTimeout time.Duration
	Env          map[string]string
}

var (
	buildBinaryOnce sync.Once
	buildBinaryErr  error
	builtBinaryPath string
)

func ensureMonitorBinary(t *testing.T) string {
	t.Helper()

	buildBinaryOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "ocd-smoke-alarm-bin-*")
		if err != nil {
			buildBinaryErr = fmt.Errorf("create temp dir: %w", err)
			return
		}

		outputPath := filepath.Join(tmpDir, "ocd-smoke-alarm")
		moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			buildBinaryErr = fmt.Errorf("determine module root: %w", err)
			return
		}

		cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/ocd-smoke-alarm")
		cmd.Dir = moduleRoot
		var stderr bytes.Buffer
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			buildBinaryErr = fmt.Errorf("go build failed: %w\n%s", err, stderr.String())
			return
		}

		builtBinaryPath = outputPath
	})

	if buildBinaryErr != nil {
		t.Fatalf("build integration binary: %v", buildBinaryErr)
	}

	return builtBinaryPath
}

func startFederationInstance(t *testing.T, name, configPath string, opts federationStartOptions, extraArgs ...string) *federationInstance {
	t.Helper()

	if opts.StartTimeout <= 0 {
		opts.StartTimeout = 10 * time.Second
	}
	if opts.ReadyzPath == "" {
		opts.ReadyzPath = "/readyz"
	}

	cmdArgs := append([]string{"serve", "--config", configPath}, extraArgs...)
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, ensureMonitorBinary(t), cmdArgs...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("OCD_SMOKE_ALARM_TEST_INSTANCE=%s", name))
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start instance %q: %v", name, err)
	}

	instance := &federationInstance{
		name:       name,
		cmd:        cmd,
		cancel:     cancel,
		stdout:     stdoutBuf,
		stderr:     stderrBuf,
		healthAddr: opts.HealthAddr,
		done:       make(chan struct{}),
	}

	// Reap the child exactly once, from here on: the readiness waiters need to
	// tell "still starting" from "died", and a second cmd.Wait would error.
	go func() {
		err := cmd.Wait()
		instance.mu.Lock()
		instance.waitErr = err
		instance.mu.Unlock()
		close(instance.done)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), opts.StartTimeout)
	defer waitCancel()

	if opts.HealthAddr != "" {
		readyURL := fmt.Sprintf("http://%s%s", opts.HealthAddr, opts.ReadyzPath)
		if err := waitForHealth(waitCtx, readyURL); err != nil {
			instance.Stop(nil)
			t.Fatalf("instance %q failed readiness check: %v\nstdout: %s\nstderr: %s", name, err, stdoutBuf.String(), stderrBuf.String())
		}
	}

	t.Cleanup(func() {
		instance.Stop(t)
	})

	return instance
}

// exitErr reports the child's exit status once it has exited. The buffers are
// only read after done is closed, because cmd.Wait returns only when the
// output-copying goroutines have finished.
func (fi *federationInstance) exitErr() (error, bool) {
	select {
	case <-fi.done:
		fi.mu.Lock()
		defer fi.mu.Unlock()
		return fi.waitErr, true
	default:
		return nil, false
	}
}

func (fi *federationInstance) Stop(t *testing.T) {
	if fi == nil || fi.stopped {
		return
	}
	fi.stopped = true
	if fi.cancel != nil {
		fi.cancel()
	}

	select {
	case <-fi.done:
		if err, _ := fi.exitErr(); err != nil && t != nil {
			t.Logf("instance %q exited with error: %v\nstdout: %s\nstderr: %s", fi.name, err, fi.stdout.String(), fi.stderr.String())
		}
	case <-time.After(5 * time.Second):
		if t != nil {
			t.Logf("instance %q did not exit in time; killing", fi.name)
		}
		if err := fi.cmd.Process.Kill(); err != nil && t != nil {
			t.Logf("kill instance %q: %v", fi.name, err)
		}
		<-fi.done
	}
}

func (fi *federationInstance) ReadyURL(path string) string {
	if fi.healthAddr == "" {
		return ""
	}
	if path == "" {
		path = "/status"
	}
	return fmt.Sprintf("http://%s%s", fi.healthAddr, path)
}

type membershipRecord struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"`
	Introducer string    `json:"introducer"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type membershipView struct {
	Self         membershipRecord   `json:"self"`
	IntroducerID string             `json:"introducer_id"`
	Peers        []membershipRecord `json:"peers"`
	Version      uint64             `json:"version"`
}

type federationIdentity struct {
	ID   string `json:"id"`
	Port int    `json:"port"`
	Role string `json:"role"`
}

func TestFederationIntroducerAndFollowerLifecycle(t *testing.T) {
	t.Parallel()

	basePort, maxPort := allocateFederationPortRange(t, 3)
	baseDir := t.TempDir()

	cfg1, stateDir1 := writeFederationConfig(t, baseDir, "inst1", basePort, maxPort)
	inst1 := startFederationInstance(t, "inst1", cfg1, federationStartOptions{
		StartTimeout: 15 * time.Second,
	})
	identity1 := waitForIdentity(t, inst1, stateDir1, 10*time.Second)
	// The introducer is the instance that took base_port, which is where the
	// test starts looking — but read the port it actually bound rather than
	// assuming the probe's answer held until the child got there.
	membershipURL := fmt.Sprintf("http://localhost:%d/membership", identity1.Port)

	view1 := waitForMembership(t, membershipURL, 10*time.Second, func(m membershipView) bool {
		return m.Self.ID != "" && m.Self.Role == "introducer" && len(m.Peers) == 0
	})
	if view1.IntroducerID != view1.Self.ID {
		t.Fatalf("expected introducer ID to match self, got %s vs %s", view1.IntroducerID, view1.Self.ID)
	}

	if identity1.Role != "introducer" {
		t.Fatalf("expected introducer role, got %s", identity1.Role)
	}

	cfg2, stateDir2 := writeFederationConfig(t, baseDir, "inst2", basePort, maxPort)
	inst2 := startFederationInstance(t, "inst2", cfg2, federationStartOptions{
		StartTimeout: 15 * time.Second,
	})
	identity2 := waitForIdentity(t, inst2, stateDir2, 10*time.Second)

	view2 := waitForMembership(t, membershipURL, 10*time.Second, func(m membershipView) bool {
		return len(m.Peers) == 1
	})
	if !containsPeer(view2.Peers, identity2.ID) {
		t.Fatalf("expected follower peer %s in membership: %+v", identity2.ID, view2)
	}

	cfg3, stateDir3 := writeFederationConfig(t, baseDir, "inst3", basePort, maxPort)
	inst3 := startFederationInstance(t, "inst3", cfg3, federationStartOptions{
		StartTimeout: 15 * time.Second,
	})
	identity3 := waitForIdentity(t, inst3, stateDir3, 10*time.Second)

	view3 := waitForMembership(t, membershipURL, 10*time.Second, func(m membershipView) bool {
		return len(m.Peers) == 2
	})
	if !containsPeer(view3.Peers, identity2.ID) || !containsPeer(view3.Peers, identity3.ID) {
		t.Fatalf("expected both follower peers present: %+v", view3)
	}

	inst2.Stop(nil)
	waitForPeerRemoval(t, membershipURL, identity2.ID, 12*time.Second)

	view4 := waitForMembership(t, membershipURL, 10*time.Second, func(m membershipView) bool {
		return len(m.Peers) == 1
	})
	if containsPeer(view4.Peers, identity2.ID) {
		t.Fatalf("expected follower %s to be removed from membership", identity2.ID)
	}
	if !containsPeer(view4.Peers, identity3.ID) {
		t.Fatalf("expected follower %s to remain present: %+v", identity3.ID, view4)
	}

	_ = inst1
	_ = inst3
}

func waitForMembership(t *testing.T, url string, timeout time.Duration, predicate func(membershipView) bool) membershipView {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last membershipView
	var lastErr error

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		mv, err := fetchMembership(ctx, url)
		cancel()
		if err == nil {
			last = mv
			if predicate(mv) {
				return mv
			}
			lastErr = nil
		} else {
			lastErr = err
		}
		time.Sleep(150 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("membership fetch failed: %v", lastErr)
	}
	t.Fatalf("membership condition not met: %+v", last)
	return membershipView{}
}

func fetchMembership(ctx context.Context, url string) (membershipView, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return membershipView{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return membershipView{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return membershipView{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var mv membershipView
	if err := json.NewDecoder(resp.Body).Decode(&mv); err != nil {
		return membershipView{}, err
	}
	return mv, nil
}

func waitForPeerRemoval(t *testing.T, url, peerID string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		mv, err := fetchMembership(ctx, url)
		cancel()
		if err == nil && !containsPeer(mv.Peers, peerID) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	mv, err := fetchMembership(ctx, url)
	cancel()
	if err != nil {
		t.Fatalf("final membership fetch failed: %v", err)
	}
	t.Fatalf("peer %s still present after timeout: %+v", peerID, mv)
}

func containsPeer(peers []membershipRecord, id string) bool {
	for _, p := range peers {
		if p.ID == id {
			return true
		}
	}
	return false
}

func waitForIdentity(t *testing.T, fi *federationInstance, stateDir string, timeout time.Duration) federationIdentity {
	t.Helper()

	path := filepath.Join(stateDir, "federation", "identity.json")
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			var ident federationIdentity
			if json.Unmarshal(data, &ident) == nil && ident.ID != "" {
				return ident
			}
		}
		// A child that has exited will never write the file, so report why it
		// died — "no free local slots", a config error, a bind failure — instead
		// of a missing identity that names none of them (smoke-alarm#78).
		if exitErr, exited := fi.exitErr(); exited {
			t.Fatalf("instance %q exited before writing %s: %v\nstdout: %s\nstderr: %s",
				fi.name, path, exitErr, fi.stdout.String(), fi.stderr.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("identity not ready at %s", path)
	return federationIdentity{}
}

func writeFederationConfig(t *testing.T, baseDir, name string, basePort, maxPort int) (string, string) {
	t.Helper()

	instanceDir := filepath.Join(baseDir, name)
	if err := os.MkdirAll(instanceDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", instanceDir, err)
	}

	lockFile := filepath.Join(instanceDir, fmt.Sprintf("%s.lock", name))
	baselineFile := filepath.Join(instanceDir, "known-good.json")
	cfgPath := filepath.Join(instanceDir, "config.yaml")

	content := fmt.Sprintf(`version: "1"
service:
  name: "%s"
  mode: "background"
  log_level: "warn"
  poll_interval: "1s"
  timeout: "1s"
health:
  enabled: false
  listen_addr: "localhost:65535"
  endpoints:
    healthz: "/healthz"
    readyz: "/readyz"
    status: "/status"
runtime:
  lock_file: "%s"
  state_dir: "%s"
  baseline_file: "%s"
  event_history_size: 100
  graceful_shutdown_timeout: "2s"
discovery:
  enabled: false
alerts:
  aggressive: true
  dedupe_window: "5s"
  cooldown: "1s"
auth:
  keystore:
    enabled: false
  redaction:
    enabled: true
    mask: "****"
targets: []
known_state:
  enabled: false
meta_config:
  enabled: false
dynamic_config:
  enabled: false
telemetry:
  enabled: false
federation:
  enabled: true
  base_port: %d
  max_port: %d
  poll_interval: "2s"
  announce_interval: "1s"
  heartbeat_interval: "1s"
  heartbeat_timeout: "3s"
hosted:
  enabled: false
remote_agent:
  managed_updates: false
`, fmt.Sprintf("itest-%s", name), lockFile, instanceDir, baselineFile, basePort, maxPort)

	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config %s: %v", cfgPath, err)
	}

	return cfgPath, instanceDir
}

// federationPortClaimRoot holds one directory per port a run is about to hand
// to its federation children. It lives in the OS temp dir because the claim
// has to be visible to every `go test` process on the machine: the children
// cannot bind the ports while the test holds them, so a probe-then-release
// cannot reserve anything, and concurrent runs otherwise pick the same range
// and one cluster dies with "no free local slots available" (smoke-alarm#78).
func federationPortClaimRoot() string {
	return filepath.Join(os.TempDir(), "smoke-alarm-federation-port-claims")
}

// federationPortClaimTTL is how long a claim survives without being released.
// A live test holds its range for seconds, so the TTL only reclaims claims left
// behind by a killed run — long enough not to steal one that is still busy,
// short enough that a crashed run does not poison the temp dir for good.
const federationPortClaimTTL = 10 * time.Minute

func allocateFederationPortRange(t *testing.T, count int) (int, int) {
	t.Helper()

	if count < 1 {
		t.Fatalf("port count must be positive")
	}

	root := federationPortClaimRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("create federation port claim directory: %v", err)
	}

	for attempt := 0; attempt < 200; attempt++ {
		base := mustFreePortValue(t)
		if !federationPortRangeFree(base, count) {
			continue
		}
		claims, ok := claimFederationPortRange(root, base, count)
		if !ok {
			continue
		}
		t.Cleanup(func() { releaseFederationPortClaims(claims) })
		return base, base + count - 1
	}
	t.Fatalf("unable to allocate contiguous federation port range of %d", count)
	return 0, 0
}

// federationPortRangeFree reports whether every port in the range can be bound
// right now. Passing it is necessary but not sufficient: the range also has to
// be claimed, because this check releases each port before the child binds it.
func federationPortRangeFree(base, count int) bool {
	for offset := 0; offset < count; offset++ {
		addr := fmt.Sprintf("localhost:%d", base+offset)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return false
		}
		_ = ln.Close()
	}
	return true
}

// claimFederationPortRange takes a claim directory per port, or gives back the
// ones it managed to take. Claims are independent and retries pick a fresh base,
// so two runs that overlap cannot deadlock on each other.
func claimFederationPortRange(root string, base, count int) ([]string, bool) {
	claims := make([]string, 0, count)
	for offset := 0; offset < count; offset++ {
		dir := filepath.Join(root, fmt.Sprintf("%d", base+offset))
		if !claimFederationPort(dir) {
			releaseFederationPortClaims(claims)
			return nil, false
		}
		claims = append(claims, dir)
	}
	return claims, true
}

func claimFederationPort(dir string) bool {
	for attempt := 0; attempt < 2; attempt++ {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			return true
		}
		if !errors.Is(err, os.ErrExist) || !federationPortClaimStale(dir) {
			return false
		}
		// Left behind by a run that was killed before its cleanup ran.
		_ = os.RemoveAll(dir)
	}
	return false
}

// federationPortClaimStale reports whether a claim is old enough to be
// abandoned. Activity cannot be refreshed portably, so age is the signal.
func federationPortClaimStale(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return time.Since(info.ModTime()) > federationPortClaimTTL
}

func releaseFederationPortClaims(claims []string) {
	for _, dir := range claims {
		_ = os.RemoveAll(dir)
	}
}

func mustFreePortValue(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("allocate port: %v", err)
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

func waitForHealth(ctx context.Context, readyURL string) error {
	if readyURL == "" {
		return fmt.Errorf("ready URL not provided")
	}

	client := &http.Client{Timeout: 1 * time.Second}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, readyURL, http.NoBody)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timeout for %s: %w", readyURL, ctx.Err())
		case <-ticker.C:
		}
	}
}
