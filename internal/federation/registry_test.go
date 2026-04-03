package federation

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRegistryUpsertAddAndUpdate(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	var captured []Event
	reg, err := NewRegistry(testIdentity("self-introducer", stateDir, 6200, RoleIntroducer), RegistryOptions{
		StateDir:         stateDir,
		AnnounceInterval: 2 * time.Second,
		HeartbeatTimeout: 5 * time.Second,
		OnChange: func(ev Event) {
			captured = append(captured, ev)
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	add := InstanceRecord{
		ID:   "peer-1",
		Port: 6201,
		Role: RoleFollower,
	}
	ev := reg.Upsert(add, "introduction")
	if ev.Type != EventAdded {
		t.Fatalf("expected EventAdded, got %s", ev.Type)
	}
	if ev.Record.ID != "peer-1" {
		t.Fatalf("unexpected record ID %q", ev.Record.ID)
	}
	if len(captured) != 1 || captured[0].Type != EventAdded {
		t.Fatalf("expected OnChange to capture add event, got %#v", captured)
	}

	update := InstanceRecord{
		ID:         "peer-1",
		Port:       6201,
		Role:       RoleFollower,
		LastSeenAt: time.Now().Add(-time.Second),
		Meta:       map[string]string{"k": "v"},
	}
	ev = reg.Upsert(update, "heartbeat")
	if ev.Type != EventUpdated {
		t.Fatalf("expected EventUpdated, got %s", ev.Type)
	}
	if len(captured) != 2 || captured[1].Type != EventUpdated {
		t.Fatalf("expected OnChange to capture update event, got %#v", captured)
	}
	if reg.version <= 2 {
		t.Fatalf("expected registry version to advance, got %d", reg.version)
	}
}

func TestRegistryAgeOutRemovesStalePeers(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	reg, err := NewRegistry(testIdentity("self", stateDir, 6300, RoleIntroducer), RegistryOptions{
		StateDir:         stateDir,
		HeartbeatTimeout: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	rec := InstanceRecord{
		ID:   "peer-stale",
		Port: 6301,
		Role: RoleFollower,
	}
	reg.Upsert(rec, "introduction")

	reg.mu.Lock()
	entry := reg.peers["peer-stale"]
	entry.record.LastSeenAt = time.Now().Add(-200 * time.Millisecond)
	reg.peers["peer-stale"] = entry
	reg.mu.Unlock()

	evs := reg.AgeOut()
	if len(evs) != 1 {
		t.Fatalf("expected one age-out event, got %d", len(evs))
	}
	if evs[0].Type != EventRemoved || evs[0].Record.ID != "peer-stale" {
		t.Fatalf("unexpected event %#v", evs[0])
	}
}

func TestRegistrySnapshotIntroducerFallback(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	reg, err := NewRegistry(testIdentity("self-intro", stateDir, 6400, RoleIntroducer), RegistryOptions{
		StateDir: stateDir,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	reg.Upsert(InstanceRecord{ID: "follower-1", Role: RoleFollower}, "init")
	snap := reg.Snapshot()
	if snap.IntroducerID != "self-intro" {
		t.Fatalf("expected introducer to default to self, got %s", snap.IntroducerID)
	}

	reg.Upsert(InstanceRecord{ID: "peer-intro", Role: RoleIntroducer}, "promotion")
	snap = reg.Snapshot()
	if snap.IntroducerID != "peer-intro" {
		t.Fatalf("expected introducer to switch to peer, got %s", snap.IntroducerID)
	}
}

func TestRegistrySaveSnapshotWritesFile(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	reg, err := NewRegistry(testIdentity("self", stateDir, 6500, RoleIntroducer), RegistryOptions{
		StateDir: stateDir,
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	reg.Upsert(InstanceRecord{ID: "peer", Role: RoleFollower}, "init")
	if err := reg.SaveSnapshot(); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}

	snapshotPath := filepath.Join(stateDir, slotStateDirName, registrySnapshotFile)
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if snap.Self.ID != "self" {
		t.Fatalf("expected snapshot self ID to be %q, got %q", "self", snap.Self.ID)
	}
	if snap.Version == 0 {
		t.Fatalf("expected snapshot version to be > 0")
	}
}

func TestCandidatePortsPrefersPreviousIdentity(t *testing.T) {
	t.Parallel()

	prev := &Identity{Port: 6602}
	ports := candidatePorts(6600, 6603, prev)
	if len(ports) == 0 || ports[0] != 6602 {
		t.Fatalf("expected previous port to be first element, got %v", ports)
	}
}

func TestClaimSlotReusesPersistedIdentity(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	basePort := freePort(t)

	opts := SlotOptions{
		BasePort:    basePort,
		MaxPort:     basePort + 2,
		StateDir:    stateDir,
		ServiceName: "federation-test",
	}

	claim1, err := ClaimSlot(opts)
	if err != nil {
		t.Fatalf("ClaimSlot first attempt failed: %v", err)
	}
	if claim1.Identity.Role != RoleIntroducer {
		t.Fatalf("expected first claim to be introducer, got %s", claim1.Identity.Role)
	}
	if err := claim1.Close(); err != nil {
		t.Fatalf("closing first claim: %v", err)
	}

	claim2, err := ClaimSlot(opts)
	if err != nil {
		t.Fatalf("ClaimSlot second attempt failed: %v", err)
	}
	defer func() {
		if err := claim2.Close(); err != nil {
			t.Fatalf("closing second claim: %v", err)
		}
	}()

	if claim2.Identity.Port != claim1.Identity.Port {
		t.Fatalf("expected second claim to reuse port %d, got %d", claim1.Identity.Port, claim2.Identity.Port)
	}
	if claim2.Identity.ID != claim1.Identity.ID {
		t.Fatalf("expected identity ID to be stable across claims")
	}
}

func TestDurationHelpers(t *testing.T) {
	t.Parallel()

	fallback := 5 * time.Second
	if got := durationOrDefault("", fallback); got != fallback {
		t.Fatalf("expected fallback duration, got %s", got)
	}
	if got := durationOrDefault("invalid", fallback); got != fallback {
		t.Fatalf("expected fallback for invalid duration, got %s", got)
	}
	if got := durationOrDefault("10s", fallback); got != 10*time.Second {
		t.Fatalf("expected parsed duration, got %s", got)
	}

	if got := clampDuration(5*time.Second, 10*time.Second, 0); got != 10*time.Second {
		t.Fatalf("expected clamp to min, got %s", got)
	}
	if got := clampDuration(15*time.Second, 0, 12*time.Second); got != 12*time.Second {
		t.Fatalf("expected clamp to max, got %s", got)
	}
	if got := clampDuration(8*time.Second, 2*time.Second, 12*time.Second); got != 8*time.Second {
		t.Fatalf("expected duration to remain unchanged, got %s", got)
	}
}

func testIdentity(id, stateDir string, port int, role Role) Identity {
	return Identity{
		ID:          id,
		ServiceName: "svc",
		Hostname:    "localhost",
		StateDir:    stateDir,
		Port:        port,
		Role:        role,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
}

func freePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate port: %v", err)
	}
	defer func() {
		if err := ln.Close(); err != nil {
			t.Fatalf("closing listener: %v", err)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.Port
}

// Guard the tests against platforms where binding may be restricted.
func TestMain(m *testing.M) {
	if runtime.GOOS == "js" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
