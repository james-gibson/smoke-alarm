package federation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClientIntroducesAndHeartbeats(t *testing.T) {
	t.Parallel()

	introducerStateDir := t.TempDir()
	followerStateDir := t.TempDir()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() {
		_ = listener.Close()
	}()

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
	introducerRegistry, err := NewRegistry(introducerIdentity, RegistryOptions{
		StateDir:         introducerStateDir,
		AnnounceInterval: 75 * time.Millisecond,
		HeartbeatTimeout: 250 * time.Millisecond,
		MaxPeers:         8,
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
	go func() {
		serverErrCh <- server.Start(serverCtx)
	}()

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
	})
	if err != nil {
		t.Fatalf("NewRegistry follower: %v", err)
	}

	introducerURL := fmt.Sprintf("http://%s", listener.Addr().String())
	client, err := NewClient(ClientOptions{
		IntroducerURL:     introducerURL,
		Registry:          followerRegistry,
		AnnounceInterval:  100 * time.Millisecond,
		HeartbeatInterval: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	go client.Start(clientCtx)

	waitForCondition(t, 4*time.Second, func() bool {
		snap := introducerRegistry.Snapshot()
		return len(snap.Peers) == 1 && snap.Peers[0].ID == followerIdentity.ID
	}, func() {
		logMembership(t, introducerURL)
		t.Logf("introducer snapshot: %+v", introducerRegistry.Snapshot())
	})

	introducerSnap := introducerRegistry.Snapshot()
	followerRegistry.Upsert(introducerSnap.Self, "introducer_membership")

	waitForCondition(t, 4*time.Second, func() bool {
		snapshotPath := filepath.Join(followerStateDir, slotStateDirName, registrySnapshotFile)
		if _, err := os.Stat(snapshotPath); err != nil {
			return false
		}
		snap := followerRegistry.Snapshot()
		return snap.IntroducerID == introducerIdentity.ID && len(snap.Peers) > 0
	}, func() {
		logMembership(t, introducerURL)
		t.Logf("follower snapshot: %+v", followerRegistry.Snapshot())
	})

	clientCancel()

	waitForCondition(t, 7*time.Second, func() bool {
		snap := introducerRegistry.Snapshot()
		return len(snap.Peers) == 0
	}, func() {
		logMembership(t, introducerURL)
		t.Logf("introducer snapshot during removal: %+v", introducerRegistry.Snapshot())
	})

	serverCancel()
	if err := server.Shutdown(); err != nil {
		t.Fatalf("server shutdown: %v", err)
	}

	select {
	case err := <-serverErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("server returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("server did not exit")
	}
}

func logMembership(t *testing.T, baseURL string) {
	resp, err := http.Get(baseURL + "/membership")
	if err != nil {
		t.Logf("membership fetch failed: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Logf("membership read failed: %v", err)
		return
	}
	t.Logf("membership response status=%s body=%s", resp.Status, string(body))
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool, onFailure func()) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	attempts := 0
	for time.Now().Before(deadline) {
		attempts++
		if fn() {
			return
		}
		if attempts%50 == 0 {
			remaining := time.Until(deadline)
			if remaining < 0 {
				remaining = 0
			}
			t.Logf("waiting for federation condition (attempt %d, ~%s remaining)", attempts, remaining)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if onFailure != nil {
		onFailure()
	}
	t.Logf("federation wait condition not satisfied after %s", timeout)
	t.Fatalf("condition not met within %s", timeout)
}
