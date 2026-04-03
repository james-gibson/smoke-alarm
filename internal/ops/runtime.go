package ops

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/alerts"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/health"
	"github.com/james-gibson/smoke-alarm/internal/knownstate"
	"github.com/james-gibson/smoke-alarm/internal/mdns"
	"github.com/james-gibson/smoke-alarm/internal/ui"
)

// Runtime orchestrates monitor lifecycle for foreground and background modes.
type Runtime struct {
	cfg    config.Config
	logger *slog.Logger

	monitor *engine.Engine
	health  *health.Server

	lockPath string
	lockFile *os.File

	startedAt time.Time
}

// NewRuntime constructs a runtime from validated config.
func NewRuntime(cfg config.Config) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logLevel := parseLogLevel(cfg.Service.LogLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	rt := &Runtime{
		cfg:       cfg,
		logger:    logger,
		lockPath:  cfg.Runtime.LockFile,
		startedAt: time.Now().UTC(),
	}
	return rt, nil
}

// Run starts runtime orchestration and blocks until completion.
//
// Foreground mode:
//   - starts monitor + health server
//   - runs Bubble Tea dashboard
//   - exits when UI exits or context is canceled
//
// Background mode:
//   - starts monitor + health server
//   - blocks until context is canceled
func (r *Runtime) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil context")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := r.acquireLock(); err != nil {
		return err
	}
	defer r.releaseLock()

	if err := r.bootstrap(runCtx); err != nil {
		return err
	}

	r.logger.Info("runtime started",
		"mode", r.cfg.Service.Mode,
		"health_enabled", r.cfg.Health.Enabled,
		"targets", len(r.cfg.EnabledTargets()),
		"started_at", r.startedAt.Format(time.RFC3339),
	)

	switch strings.ToLower(strings.TrimSpace(r.cfg.Service.Mode)) {
	case config.ModeForeground:
		return r.runForeground(runCtx, cancel)
	case config.ModeBackground:
		return r.runBackground(runCtx)
	default:
		return fmt.Errorf("unsupported runtime mode %q", r.cfg.Service.Mode)
	}
}

func (r *Runtime) bootstrap(ctx context.Context) error {
	notifier, err := r.buildNotifier()
	if err != nil {
		return err
	}

	var ks *knownstate.Store
	if r.cfg.KnownState.Enabled {
		ks = knownstate.NewStore(
			r.cfg.Runtime.BaselineFile,
			knownstate.WithAutoPersist(r.cfg.KnownState.Persist),
			knownstate.WithSustainSuccess(r.cfg.KnownState.SustainSuccessBeforeMarkHealthy),
		)
	}

	eng, err := engine.New(
		r.cfg,
		engine.WithStore(ks),
		engine.WithNotifier(notifier),
	)
	if err != nil {
		return fmt.Errorf("create monitor engine: %w", err)
	}
	r.monitor = eng

	if r.cfg.Health.Enabled {
		shutdownTimeout, _ := time.ParseDuration(r.cfg.Runtime.GracefulShutdownTimeout)
		r.health = health.NewServer(health.Options{
			ServiceName:     r.cfg.Service.Name,
			ListenAddr:      r.cfg.Health.ListenAddr,
			HealthzPath:     r.cfg.Health.Endpoints.Healthz,
			ReadyzPath:      r.cfg.Health.Endpoints.Readyz,
			StatusPath:      r.cfg.Health.Endpoints.Status,
			ShutdownTimeout: shutdownTimeout,
		})

		// Pre-bind with port scanning so we claim the lowest available port
		// and know the actual address before advertising it via mDNS.
		boundAddr, err := r.health.BindWithRetry(10)
		if err != nil {
			return fmt.Errorf("health server bind: %w", err)
		}
		if boundAddr != r.cfg.Health.ListenAddr {
			r.logger.Warn("health server port in use, using next available",
				"preferred", r.cfg.Health.ListenAddr, "actual", boundAddr)
		}

		if r.cfg.Tuner.Advertise {
			advertiser := mdns.NewAdvertiser(mdns.Options{
				ServiceName: r.cfg.Service.Name,
				ServiceType: "_smoke-alarm._tcp",
				Port:        mdns.ParsePort(boundAddr),
			})
			if err := advertiser.Start(ctx); err != nil {
				r.logger.Warn("mdns advertiser failed to start", "err", err)
			} else {
				r.logger.Info("mdns advertising _smoke-alarm._tcp", "addr", boundAddr)
				defer advertiser.Shutdown()
			}
		}

		r.health.SetComponent("engine", false, "starting")
		r.health.SetComponent("discovery", true, "idle")
		r.health.SetReady(false, "engine warming up")
	}

	// Optional one-shot discovery for startup observability.
	r.runStartupDiscovery(ctx)

	return nil
}

func (r *Runtime) runForeground(ctx context.Context, cancel context.CancelFunc) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	// monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := r.monitor.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- fmt.Errorf("engine failed: %w", err)
			cancel()
		}
	}()

	// health server
	if r.health != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.health.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("health server failed: %w", err)
				cancel()
			}
		}()
	}

	// health sync loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.syncHealthLoop(ctx)
	}()

	// UI blocks in foreground mode.
	uiErr := ui.Run(ctx, r.monitor, ui.Options{
		RefreshInterval: 1 * time.Second,
		HeaderTitle:     "OCD Smoke Alarm",
		MaxEvents:       12,
		ShowHelp:        true,
	})

	// UI exit means runtime should stop.
	cancel()
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
	}

	if uiErr != nil && !errors.Is(uiErr, context.Canceled) {
		return fmt.Errorf("foreground UI failed: %w", uiErr)
	}
	return nil
}

func (r *Runtime) runBackground(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	// monitor
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := r.monitor.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- fmt.Errorf("engine failed: %w", err)
		}
	}()

	// health server
	if r.health != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.health.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("health server failed: %w", err)
			}
		}()
	}

	// health sync loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.syncHealthLoop(ctx)
	}()

	select {
	case <-ctx.Done():
		wg.Wait()
		return nil
	case err := <-errCh:
		wg.Wait()
		return err
	}
}

func (r *Runtime) syncHealthLoop(ctx context.Context) {
	if r.health == nil || r.monitor == nil {
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.health.SetComponent("engine", false, "stopped")
			r.health.SetReady(false, "shutting down")
			return
		case <-ticker.C:
			ready := r.monitor.IsReady()
			if ready {
				r.health.SetComponent("engine", true, "running")
				r.health.SetReady(true, "")
			} else {
				r.health.SetComponent("engine", false, "warming up")
				r.health.SetReady(false, "engine warming up")
			}

			// Mirror monitor target statuses into health /status endpoint.
			for _, st := range r.monitor.SnapshotStatuses() {
				r.health.UpsertTargetStatus(health.TargetStatus{
					ID:         st.TargetID,
					Protocol:   string(st.Protocol),
					Endpoint:   st.Endpoint,
					State:      string(st.State),
					Severity:   string(st.Severity),
					Message:    st.Message,
					Regression: st.Regression,
					CheckedAt:  st.LastCheckedAt,
					LatencyMS:  st.Latency.Milliseconds(),
				})
			}
		}
	}
}

func (r *Runtime) runStartupDiscovery(ctx context.Context) {
	if !r.cfg.Discovery.Enabled {
		return
	}

	d := discovery.New()
	res := d.Discover(ctx, r.cfg)

	if r.health != nil {
		if len(res.Errors) > 0 {
			r.health.SetComponent("discovery", false, "startup discovery had errors")
		} else {
			r.health.SetComponent("discovery", true, "startup discovery ok")
		}
	}

	r.logger.Info("startup discovery complete",
		"records", len(res.Records),
		"errors", len(res.Errors),
	)
	for _, e := range res.Errors {
		r.logger.Warn("startup discovery issue", "error", e)
	}
}

func (r *Runtime) buildNotifier() (engine.Notifier, error) { //nolint:unparam
	notifiers := make([]engine.Notifier, 0, 2)

	if r.cfg.Alerts.Sinks.Log.Enabled {
		dedupeWindow, _ := time.ParseDuration(r.cfg.Alerts.DedupeWindow)
		n := alerts.NewLogNotifier(r.logger, "info", dedupeWindow)
		notifiers = append(notifiers, n)
	}

	if r.cfg.Alerts.Sinks.OSNotification.Enabled {
		dedupeWindow, _ := time.ParseDuration(r.cfg.Alerts.DedupeWindow)
		n := alerts.NewDesktopNotifier(
			r.cfg.Alerts.Sinks.OSNotification.TitlePrefix,
			"warn",
			dedupeWindow,
		)
		notifiers = append(notifiers, n)
	}

	if len(notifiers) == 0 {
		// Always keep at least log notifier in-process.
		n := alerts.NewLogNotifier(r.logger, "info", 0)
		notifiers = append(notifiers, n)
	}

	return alerts.NewNotifierGroup(notifiers...), nil
}

func parseLogLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (r *Runtime) acquireLock() error {
	path := strings.TrimSpace(r.lockPath)
	if path == "" {
		// no lock requested
		return nil
	}

	if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		r.lockFile = f
		_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
		return nil
	}

	if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("acquire lock %q: %w", path, err)
	}

	// Handle stale lock file.
	pid, readErr := readPID(path)
	if readErr != nil {
		return fmt.Errorf("lock exists and cannot read PID (%s): %w", path, readErr)
	}
	if pid > 0 && processAlive(pid) {
		return fmt.Errorf("another instance is already running with pid %d", pid)
	}

	// Stale lock, replace.
	_ = os.Remove(path)
	f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("replace stale lock %q: %w", path, err)
	}
	r.lockFile = f
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	return nil
}

func (r *Runtime) releaseLock() {
	if r.lockFile != nil {
		_ = r.lockFile.Close()
	}
	if strings.TrimSpace(r.lockPath) != "" {
		_ = os.Remove(r.lockPath)
	}
}

func readPID(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return 0, nil
	}
	pid, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Signal 0 checks existence/permission without delivering a signal.
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func filepathDir(path string) string {
	i := strings.LastIndex(path, "/")
	if i <= 0 {
		return "."
	}
	return path[:i]
}
