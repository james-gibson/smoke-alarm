package federation

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

// TestServerStartWaitsForAgeOutLoop pins the shutdown ordering: Start must not
// return while the age-out loop can still write the registry snapshot.
//
// Before this was enforced, Start returned as soon as the HTTP server stopped,
// leaving the loop mid-write. The visible symptom was a test cleanup failure
// (`TempDir RemoveAll cleanup: unlinkat .../federation: directory not empty`)
// when the snapshot landed inside a directory being removed — a shutdown that
// reports success while state is still being written is wrong beyond tests.
//
// The loop is held inside the registry callback so the window is deterministic
// rather than a matter of scheduling.
func TestServerStartWaitsForAgeOutLoop(t *testing.T) {
	t.Parallel()

	stateDir := t.TempDir()
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var blockOnce sync.Once

	registry, err := NewRegistry(Identity{
		ID:          "introducer-1",
		ServiceName: "smoke-alarm",
		Hostname:    "localhost",
		StateDir:    stateDir,
		Port:        listener.Addr().(*net.TCPAddr).Port,
		Role:        RoleIntroducer,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}, RegistryOptions{
		StateDir:         stateDir,
		HeartbeatTimeout: time.Millisecond,
		OnChange: func(ev Event) {
			if ev.Type != EventRemoved {
				return
			}
			blockOnce.Do(func() {
				close(entered)
				<-release
			})
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	server, err := NewServer(ServerOptions{
		Listener:       listener,
		Registry:       registry,
		AgeOutInterval: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	// Age the only peer out immediately so the loop has work to do.
	registry.Upsert(InstanceRecord{ID: "peer-1", Role: RoleFollower}, "test")
	time.Sleep(5 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() { started <- server.Start(ctx) }()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("age-out loop never reached the registry callback")
	}

	cancel()

	select {
	case err := <-started:
		close(release)
		t.Fatalf("Start returned (%v) while the age-out loop was still writing", err)
	case <-time.After(100 * time.Millisecond):
		// Still blocked on the callback, as it must be.
	}

	close(release)

	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("Start returned error after the loop drained: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return after the age-out loop drained")
	}
}
