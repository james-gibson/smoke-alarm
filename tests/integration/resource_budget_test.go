package integration_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestResourceBudget_EngineSingleTargetFootprint(t *testing.T) {
	t.Parallel()

	const (
		sampleWindow          = 1200 * time.Millisecond
		sampleInterval        = 20 * time.Millisecond
		maxPeakGoroutineDelta = 40
		maxFinalGoroutineLeak = 15
		maxHeapAllocDelta     = int64(24 << 20) // 24 MiB
		maxTotalAllocDelta    = int64(64 << 20) // 64 MiB
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	cfg := baseConfigForEndpoint(t, srv.URL, func(c *config.Config) {
		c.KnownState.Enabled = false
		c.Health.Enabled = false

		// Keep the probe cadence quick but bounded for a short, deterministic budget test.
		c.Service.PollInterval = "30ms"
		c.Service.Timeout = "250ms"
		c.Service.MaxWorkers = 2
		c.Targets[0].Check.Interval = "30ms"
		c.Targets[0].Check.Timeout = "250ms"
		c.Targets[0].Check.Retries = 0
	})

	eng, err := engine.New(cfg)
	if err != nil {
		t.Fatalf("engine.New failed: %v", err)
	}

	// Establish baseline snapshots before engine start.
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	baseG := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- eng.Start(ctx)
	}()

	// Sample goroutines while engine is active to estimate peak concurrency overhead.
	peakG := baseG
	deadline := time.Now().Add(sampleWindow)
	for time.Now().Before(deadline) {
		g := runtime.NumGoroutine()
		if g > peakG {
			peakG = g
		}
		_ = eng.SnapshotStatuses() // keep read path exercised
		time.Sleep(sampleInterval)
	}

	// Ensure the monitor actually ran checks and reached healthy once.
	st := waitForTargetStatus(t, eng, "t1", 2*time.Second, func(s engine.TargetRuntimeStatus) bool {
		return s.State == targets.StateHealthy
	})
	if st.State != targets.StateHealthy {
		t.Fatalf("expected healthy status, got %s", st.State)
	}

	// Stop and wait for clean shutdown.
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("engine returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("engine did not stop in time")
	}

	// Give runtime a moment to settle and collect post-run memory stats.
	time.Sleep(150 * time.Millisecond)
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	peakDelta := peakG - baseG
	if peakDelta > maxPeakGoroutineDelta {
		t.Fatalf(
			"goroutine peak delta too high: peak=%d base=%d delta=%d limit=%d",
			peakG, baseG, peakDelta, maxPeakGoroutineDelta,
		)
	}

	finalG := runtime.NumGoroutine()
	finalDelta := finalG - baseG
	if finalDelta > maxFinalGoroutineLeak {
		t.Fatalf(
			"goroutine leak too high after shutdown: final=%d base=%d delta=%d limit=%d",
			finalG, baseG, finalDelta, maxFinalGoroutineLeak,
		)
	}

	heapAllocDelta := int64(memAfter.HeapAlloc) - int64(memBefore.HeapAlloc)
	if heapAllocDelta < 0 {
		heapAllocDelta = 0
	}
	if heapAllocDelta > maxHeapAllocDelta {
		t.Fatalf(
			"heap alloc delta too high: before=%d after=%d delta=%d limit=%d",
			memBefore.HeapAlloc, memAfter.HeapAlloc, heapAllocDelta, maxHeapAllocDelta,
		)
	}

	totalAllocDelta := int64(memAfter.TotalAlloc) - int64(memBefore.TotalAlloc)
	if totalAllocDelta < 0 {
		totalAllocDelta = 0
	}
	if totalAllocDelta > maxTotalAllocDelta {
		t.Fatalf(
			"total alloc delta too high: before=%d after=%d delta=%d limit=%d",
			memBefore.TotalAlloc, memAfter.TotalAlloc, totalAllocDelta, maxTotalAllocDelta,
		)
	}
}
