package health

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestShutdownIsRepeatable pins the contract behind smoke-alarm#40: Shutdown
// may be called after the process has already stopped, and that is a no-op
// rather than a second close of the signal channel. The concurrent case is
// covered by TestShutdownRace.
func TestShutdownIsRepeatable(t *testing.T) {
	t.Parallel()

	srv := NewServer(Options{ListenAddr: "127.0.0.1:0"})
	if _, err := srv.BindWithRetry(1); err != nil {
		t.Fatalf("bind health listener: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := srv.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown #%d = %v, want nil", i+1, err)
		}
	}
	if srv.Snapshot().Live {
		t.Error("Snapshot().Live = true after Shutdown, want false")
	}
}

// TestShutdownRace is smoke-alarm#40. Start calls Shutdown when ctx is
// canceled, and the process's deferred cleanup calls it too, so the two
// overlap on the way out. The guard used to be a select/default around
// close(shutdownSignal), which is not atomic: two callers could both take the
// default branch and race to close the channel — a data race on hchan.closed,
// and "close of closed channel" when the timing fits.
//
// The window is a few instructions wide, so one server is not enough to hit it;
// each round builds a fresh one and releases its callers together, the shape the
// original -count=20 repro needed.
func TestShutdownRace(t *testing.T) {
	t.Parallel()

	const (
		rounds        = 40
		directCallers = 8
	)

	for round := 0; round < rounds; round++ {
		srv := NewServer(Options{ListenAddr: "127.0.0.1:0", ShutdownTimeout: 5 * time.Second})
		if _, err := srv.BindWithRetry(1); err != nil {
			t.Fatalf("round %d: bind health listener: %v", round, err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		serveDone := make(chan error, 1)
		go func() {
			close(started)
			serveDone <- srv.Start(ctx)
		}()
		<-started

		// Race the ctx.Done() path against direct callers, the way a deferred
		// Shutdown races Start's cancellation handling.
		gate := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(directCallers)
		for range directCallers {
			go func() {
				defer wg.Done()
				<-gate
				if err := srv.Shutdown(context.Background()); err != nil {
					t.Errorf("round %d: Shutdown: %v", round, err)
				}
			}()
		}
		close(gate)
		cancel()
		wg.Wait()

		select {
		case err := <-serveDone:
			if err != nil {
				t.Fatalf("round %d: Start = %v, want nil after shutdown", round, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("round %d: Start did not return after Shutdown", round)
		}
		cancel()

		if err := srv.Shutdown(context.Background()); err != nil {
			t.Fatalf("round %d: repeated Shutdown = %v, want nil", round, err)
		}
		if srv.Snapshot().Live {
			t.Fatalf("round %d: Snapshot().Live = true after Shutdown", round)
		}
	}
}
