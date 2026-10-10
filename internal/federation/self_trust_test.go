package federation

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/james-gibson/isotope"
)

// recordingSink is a goroutine-safe IsotopeSink used to observe the isotopes a
// membership observer would register.
type recordingSink struct {
	mu      sync.Mutex
	records map[string]isotope.Record
}

func newRecordingSink() *recordingSink {
	return &recordingSink{records: make(map[string]isotope.Record)}
}

func (s *recordingSink) UpsertIsotope(rec isotope.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[rec.Name] = rec
}

func (s *recordingSink) RemoveIsotope(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, name)
}

func (s *recordingSink) get(name string) (isotope.Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[name]
	return rec, ok
}

// TestSameClusterFollowerBecomesTrustedIsotope is an end-to-end check of the
// self-trust path: a follower that shares the introducer's cluster identity is
// mirrored into the isotope sink as a mesh member, while the shared identity on
// the wire is what makes it trusted.
func TestSameClusterFollowerBecomesTrustedIsotope(t *testing.T) {
	t.Parallel()

	const clusterID = "base-test"

	introducerStateDir := t.TempDir()
	followerStateDir := t.TempDir()

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	sink := newRecordingSink()

	introducerIdentity := Identity{
		ID:          "introducer-1",
		ServiceName: "smoke-alarm",
		Hostname:    "localhost",
		StateDir:    introducerStateDir,
		Port:        listener.Addr().(*net.TCPAddr).Port,
		Role:        RoleIntroducer,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}

	var introducerRegistry *Registry
	introducerRegistry, err = NewRegistry(introducerIdentity, RegistryOptions{
		StateDir:         introducerStateDir,
		AnnounceInterval: 75 * time.Millisecond,
		HeartbeatTimeout: 250 * time.Millisecond,
		MaxPeers:         8,
		ClusterID:        clusterID,
		OnChange: func(ev Event) {
			if introducerRegistry == nil {
				return
			}
			MirrorMember(sink, introducerRegistry.Self(), ev)
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry introducer: %v", err)
	}

	server, err := NewServer(ServerOptions{
		Listener:       listener,
		Registry:       introducerRegistry,
		AgeOutInterval: 75 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()

	serverErrCh := make(chan error, 1)
	go func() { serverErrCh <- server.Start(serverCtx) }()

	waitForCondition(t, 2*time.Second, func() bool {
		conn, dialErr := net.DialTimeout("tcp", listener.Addr().String(), 25*time.Millisecond)
		if dialErr != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, nil)

	followerIdentity := Identity{
		ID:          "follower-1",
		ServiceName: "smoke-alarm",
		Hostname:    "localhost",
		StateDir:    followerStateDir,
		Port:        0,
		Role:        RoleFollower,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	followerRegistry, err := NewRegistry(followerIdentity, RegistryOptions{
		StateDir:         followerStateDir,
		AnnounceInterval: 100 * time.Millisecond,
		HeartbeatTimeout: 250 * time.Millisecond,
		MaxPeers:         8,
		ClusterID:        clusterID,
	})
	if err != nil {
		t.Fatalf("NewRegistry follower: %v", err)
	}

	client, err := NewClient(ClientOptions{
		IntroducerURL:     fmt.Sprintf("http://%s", listener.Addr().String()),
		Registry:          followerRegistry,
		AnnounceInterval:  100 * time.Millisecond,
		HeartbeatInterval: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	clientStopped := make(chan struct{})
	go func() {
		client.Start(clientCtx)
		close(clientStopped)
	}()

	waitForCondition(t, 4*time.Second, func() bool {
		rec, ok := sink.get(followerIdentity.ID)
		return ok && rec.TrustRung == MeshMemberRung
	}, func() {
		t.Logf("introducer snapshot: %+v", introducerRegistry.Snapshot())
	})

	rec, ok := sink.get(followerIdentity.ID)
	if !ok {
		t.Fatalf("follower %q was not mirrored as an isotope", followerIdentity.ID)
	}
	if rec.TrustRung != MeshMemberRung {
		t.Fatalf("TrustRung = %d, want %d", rec.TrustRung, MeshMemberRung)
	}
	if rec.RungName == "" {
		t.Fatal("mirrored isotope is missing its rung name")
	}

	clientCancel()
	select {
	case <-clientStopped:
	case <-time.After(5 * time.Second):
		t.Fatal("client.Start did not return after context cancellation")
	}
}
