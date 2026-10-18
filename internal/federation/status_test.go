package federation

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/health"
)

// ---- test doubles -----------------------------------------------------------

// recordingComponents captures the components a PeerStatusSink renders.
type recordingComponents struct {
	mu    sync.Mutex
	comps map[string]health.ComponentStatus
}

func newRecordingComponents() *recordingComponents {
	return &recordingComponents{comps: make(map[string]health.ComponentStatus)}
}

func (r *recordingComponents) SetComponent(name string, healthy bool, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.comps[name] = health.ComponentStatus{Name: name, Healthy: healthy, Detail: detail}
}

func (r *recordingComponents) RemoveComponent(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.comps, name)
}

func (r *recordingComponents) get(name string) (health.ComponentStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.comps[name]
	return c, ok
}

func (r *recordingComponents) hasPrefix(prefix string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name := range r.comps {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// stubListener lets a test assert the non-loopback guard without binding a
// publicly reachable socket.
type stubListener struct{ addr string }

func (s stubListener) Accept() (net.Conn, error) { return nil, os.ErrDeadlineExceeded }
func (s stubListener) Close() error              { return nil }
func (s stubListener) Addr() net.Addr            { return stubAddr(s.addr) }

type stubAddr string

func (a stubAddr) Network() string { return "tcp" }
func (a stubAddr) String() string  { return string(a) }

// TestAlertReachesAnotherFollower is the regression test for the reported bug: a
// follower raises an alert, and a *different* follower learns of it without
// polling anyone. Before the status path existed the alert never left the node
// that raised it, so corroboration was impossible.
func TestAlertReachesAnotherFollower(t *testing.T) {
	t.Parallel()

	const clusterID = "lab-cluster"
	const secret = "shared-secret"

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	introducerStateDir := t.TempDir()
	introducerIdentity := Identity{
		ID:          "introducer-1",
		ServiceName: "smoke-alarm",
		Hostname:    "127.0.0.1",
		StateDir:    introducerStateDir,
		Port:        listener.Addr().(*net.TCPAddr).Port,
		Role:        RoleIntroducer,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	introducerRegistry, err := NewRegistry(introducerIdentity, RegistryOptions{
		StateDir:         introducerStateDir,
		AnnounceInterval: 50 * time.Millisecond,
		HeartbeatTimeout: 500 * time.Millisecond,
		ClusterID:        clusterID,
	})
	if err != nil {
		t.Fatalf("NewRegistry introducer: %v", err)
	}

	introducerSink := newRecordingComponents()
	server, err := NewServer(ServerOptions{
		Listener:          listener,
		Registry:          introducerRegistry,
		AgeOutInterval:    50 * time.Millisecond,
		SharedSecret:      secret,
		PeerStatusTimeout: time.Second,
		OnStatus: func(statuses PeerStatuses) {
			NewPeerStatusSink(introducerSink).ApplyAll(statuses, time.Now().UTC(), time.Second)
		},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = server.Start(ctx) }()

	waitForCondition(t, 2*time.Second, func() bool {
		conn, dialErr := net.DialTimeout("tcp", listener.Addr().String(), 25*time.Millisecond)
		if dialErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, nil)

	// alarm-a raises a honeypot alert — adhd reports these as components.
	alert := health.StatusResponse{
		Service: "smoke-alarm",
		Now:     time.Now().UTC(),
		Live:    true,
		Ready:   true,
		Summary: health.StatusSummary{Total: 1, Unhealthy: 1},
		Components: []health.ComponentStatus{{
			Name:      "security/honeypot_triggered",
			Healthy:   false,
			Detail:    "caller 127.0.0.1 contacted a honeypot tool",
			UpdatedAt: time.Now().UTC(),
		}},
	}
	healthy := health.StatusResponse{
		Service: "smoke-alarm",
		Now:     time.Now().UTC(),
		Live:    true,
		Ready:   true,
		Summary: health.StatusSummary{Total: 1, Healthy: 1},
	}

	alarmAID := startFollower(t, ctx, followerSpec{
		id: "alarm-a", stateDir: t.TempDir(), introducerURL: "http://" + listener.Addr().String(),
		clusterID: clusterID, secret: secret, status: func() health.StatusResponse { return alert },
	})
	startFollower(t, ctx, followerSpec{
		id: "alarm-b", stateDir: t.TempDir(), introducerURL: "http://" + listener.Addr().String(),
		clusterID: clusterID, secret: secret, status: func() health.StatusResponse { return healthy },
	})

	// The introducer must show alarm-a's alert...
	waitForCondition(t, 5*time.Second, func() bool {
		c, ok := introducerSink.get(peerPrefix + "/" + alarmAID + "/security/honeypot_triggered")
		return ok && !c.Healthy
	}, func() {
		t.Logf("introducer components: %+v", introducerSink.comps)
	})

	// ...and so must alarm-b, which is the point of the whole mechanism.
	bSink := followers.sinkFor("alarm-b")
	if bSink == nil {
		t.Fatal("alarm-b never registered a status sink")
	}
	waitForCondition(t, 5*time.Second, func() bool {
		c, ok := bSink.get(peerPrefix + "/" + alarmAID + "/security/honeypot_triggered")
		return ok && !c.Healthy
	}, func() {
		t.Logf("alarm-b components: %+v", bSink.comps)
	})

	// A peer's roll-up must reflect the alert too, not just the leaf entry.
	rollup, ok := bSink.get(peerPrefix + "/" + alarmAID)
	if !ok {
		t.Fatalf("alarm-b is missing the roll-up for %s", alarmAID)
	}
	if rollup.Healthy {
		t.Fatalf("roll-up for %s reported healthy despite an unhealthy component", alarmAID)
	}
}

// ---- follower harness -------------------------------------------------------

type followerSpec struct {
	id            string
	stateDir      string
	introducerURL string
	clusterID     string
	secret        string
	status        func() health.StatusResponse
}

// followers tracks the status sink each test follower rendered into, so the
// test can assert on a *different* follower's view.
var followers = &followerRegistry{sinks: make(map[string]*recordingComponents)}

type followerRegistry struct {
	mu    sync.Mutex
	sinks map[string]*recordingComponents
}

func (f *followerRegistry) register(id string, sink *recordingComponents) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sinks[id] = sink
}

func (f *followerRegistry) sinkFor(id string) *recordingComponents {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sinks[id]
}

func startFollower(t *testing.T, ctx context.Context, spec followerSpec) string {
	t.Helper()

	identity := Identity{
		ID:          spec.id,
		ServiceName: "smoke-alarm",
		Hostname:    "127.0.0.1",
		StateDir:    spec.stateDir,
		Port:        0,
		Role:        RoleFollower,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	registry, err := NewRegistry(identity, RegistryOptions{
		StateDir:         spec.stateDir,
		AnnounceInterval: 50 * time.Millisecond,
		HeartbeatTimeout: 500 * time.Millisecond,
		ClusterID:        spec.clusterID,
	})
	if err != nil {
		t.Fatalf("NewRegistry %s: %v", spec.id, err)
	}

	sink := newRecordingComponents()
	followers.register(spec.id, sink)

	client, err := NewClient(ClientOptions{
		IntroducerURL:     spec.introducerURL,
		Registry:          registry,
		SharedSecret:      spec.secret,
		AnnounceInterval:  50 * time.Millisecond,
		HeartbeatInterval: 75 * time.Millisecond,
		ReportInterval:    50 * time.Millisecond,
		StatusFunc:        spec.status,
		StatusSink:        NewPeerStatusSink(sink),
		PeerStatusTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient %s: %v", spec.id, err)
	}
	go client.Start(ctx)

	return spec.id
}

// ---- guards -----------------------------------------------------------------

func TestIsLoopbackAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:19100", true},
		{"127.0.0.1", true},
		{"localhost", true},
		{"localhost:19100", true},
		{"http://127.0.0.1:58500", true},
		{"http://localhost:19100/", true},
		{"[::1]:19100", true},
		{"::1", true},
		{"0.0.0.0:19100", false},
		{"192.168.7.205:50970", false},
		{"http://192.168.7.205:50970", false},
		{"smoke-alarm.internal:19100", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := IsLoopbackAddr(tc.addr); got != tc.want {
			t.Errorf("IsLoopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

// TestNonLoopbackRequiresSharedSecret pins the fail-closed rule. Exposing the
// introducer beyond loopback without a secret would let any reachable host claim
// cluster membership, and a matching cluster id is a rung-4 trust grant rather
// than a roster entry.
func TestNonLoopbackRequiresSharedSecret(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(Identity{ID: "self", Role: RoleIntroducer, StateDir: t.TempDir()}, RegistryOptions{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	if err := RequireSharedSecret("192.168.7.205:19100", ""); err == nil {
		t.Fatal("expected a non-loopback bind without a secret to be refused")
	}
	if err := RequireSharedSecret("0.0.0.0:19100", ""); err == nil {
		t.Fatal("expected a wildcard bind without a secret to be refused")
	}
	if err := RequireSharedSecret("localhost:19100", ""); err != nil {
		t.Fatalf("loopback bind should not require a secret: %v", err)
	}
	if err := RequireSharedSecret("192.168.7.205:19100", "s3cret"); err != nil {
		t.Fatalf("non-loopback bind with a secret should be allowed: %v", err)
	}

	// NewServer must apply the same rule to the listener it is given.
	if _, err := NewServer(ServerOptions{
		Listener: stubListener{addr: "0.0.0.0:19100"},
		Registry: registry,
	}); err == nil {
		t.Fatal("NewServer accepted a non-loopback listener with no shared secret")
	}

	if _, err := NewServer(ServerOptions{
		Listener:     stubListener{addr: "0.0.0.0:19100"},
		Registry:     registry,
		SharedSecret: "s3cret",
	}); err != nil {
		t.Fatalf("NewServer refused a non-loopback listener with a secret: %v", err)
	}
}

// TestNonLoopbackIntroducerRequiresSharedSecret covers the follower side of the
// same rule: reporting into a remote cluster unauthenticated is either refused
// by the peer or accepted by a host that is not our introducer.
func TestNonLoopbackIntroducerRequiresSharedSecret(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(Identity{ID: "follower", Role: RoleFollower, StateDir: t.TempDir()}, RegistryOptions{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	if _, err := NewClient(ClientOptions{
		IntroducerURL: "http://192.168.7.205:19100",
		Registry:      registry,
	}); err == nil {
		t.Fatal("expected a remote introducer without a secret to be refused")
	}

	if _, err := NewClient(ClientOptions{
		IntroducerURL: "http://192.168.7.205:19100",
		Registry:      registry,
		SharedSecret:  "s3cret",
	}); err != nil {
		t.Fatalf("remote introducer with a secret should be allowed: %v", err)
	}

	// Loopback keeps working with no secret, so single-host setups are unchanged.
	if _, err := NewClient(ClientOptions{
		IntroducerURL: "http://127.0.0.1:19100",
		Registry:      registry,
	}); err != nil {
		t.Fatalf("loopback introducer should not require a secret: %v", err)
	}
}

// TestReportEndpointRejectsWrongSecret exercises the HTTP boundary directly, so
// the guard is pinned even if a caller forgets to configure a secret.
func TestReportEndpointRejectsWrongSecret(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(Identity{ID: "self", Role: RoleIntroducer, StateDir: t.TempDir()}, RegistryOptions{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	server, err := NewServer(ServerOptions{
		Listener:     stubListener{addr: "127.0.0.1:19100"},
		Registry:     registry,
		SharedSecret: "correct",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	body, err := json.Marshal(StatusReport{InstanceID: "peer"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"no secret", "", http.StatusUnauthorized},
		{"wrong secret", "nope", http.StatusUnauthorized},
		{"correct secret", "correct", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/reports", strings.NewReader(string(body)))
			if tc.header != "" {
				req.Header.Set(secretHeader, tc.header)
			}
			rec := httptest.NewRecorder()
			server.httpSrv.Handler.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// ---- sink behavior ----------------------------------------------------------

// TestPeerStatusSinkWithdrawsDepartedPeer pins that a peer's entries do not
// linger: a stale alert left behind would read as a live one.
func TestPeerStatusSinkWithdrawsDepartedPeer(t *testing.T) {
	t.Parallel()

	rec := newRecordingComponents()
	sink := NewPeerStatusSink(rec)

	status := health.StatusResponse{
		Now:     time.Now().UTC(),
		Summary: health.StatusSummary{Total: 1, Unhealthy: 1},
		Components: []health.ComponentStatus{
			{Name: "security/honeypot_triggered", Healthy: false},
		},
	}
	sink.ApplyAll(PeerStatuses{"peer-1": status}, time.Now().UTC(), time.Second)
	if !rec.hasPrefix(peerPrefix + "/peer-1") {
		t.Fatal("expected peer-1 entries to be rendered")
	}

	// Peer leaves the aggregate entirely.
	sink.ApplyAll(PeerStatuses{}, time.Now().UTC(), time.Second)
	if rec.hasPrefix(peerPrefix + "/peer-1") {
		t.Fatal("departed peer left entries behind")
	}
}

// TestPeerStatusSinkMarksStaleStatus pins the staleness rule: a peer that keeps
// heartbeating but stops reporting must not keep reading as healthy.
func TestPeerStatusSinkMarksStaleStatus(t *testing.T) {
	t.Parallel()

	rec := newRecordingComponents()
	sink := NewPeerStatusSink(rec)

	stale := health.StatusResponse{
		Now:     time.Now().Add(-10 * time.Minute).UTC(),
		Summary: health.StatusSummary{Total: 1, Healthy: 1},
	}
	sink.ApplyAll(PeerStatuses{"peer-1": stale}, time.Now().UTC(), time.Minute)

	rollup, ok := rec.get(peerPrefix + "/peer-1")
	if !ok {
		t.Fatal("expected a roll-up entry")
	}
	if rollup.Healthy {
		t.Fatal("a stale peer roll-up reported healthy")
	}
	if !strings.Contains(rollup.Detail, "no status report") {
		t.Fatalf("stale detail = %q, want it to name the silence", rollup.Detail)
	}
}

// TestPeerStatusSinkTrimsButKeepsUnhealthy pins that the component cap drops
// healthy rows before unhealthy ones — a cap must never hide an alert.
func TestPeerStatusSinkTrimsButKeepsUnhealthy(t *testing.T) {
	t.Parallel()

	rec := newRecordingComponents()
	sink := NewPeerStatusSink(rec)

	var comps []health.ComponentStatus
	comps = append(comps, health.ComponentStatus{Name: "security/honeypot_triggered", Healthy: false})
	for i := 0; i < maxPeerComponents+10; i++ {
		comps = append(comps, health.ComponentStatus{Name: "noise/" + string(rune('a'+i%26)), Healthy: true})
	}
	sink.ApplyAll(PeerStatuses{"peer-1": {
		Now:        time.Now().UTC(),
		Summary:    health.StatusSummary{Total: 1, Unhealthy: 1},
		Components: comps,
	}}, time.Now().UTC(), time.Second)

	if _, ok := rec.get(peerPrefix + "/peer-1/security/honeypot_triggered"); !ok {
		t.Fatal("the unhealthy component was trimmed away by the cap")
	}
}
