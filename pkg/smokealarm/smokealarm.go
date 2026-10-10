// Package smokealarm is a small, importable facade over smoke-alarm's internal
// subsystems. It lets a host process — for example lezz — start a smoke-alarm
// instance in-process instead of shelling out to the ocd-smoke-alarm binary
// with a generated YAML config.
//
// This first iteration runs the monitor engine and the health endpoints
// (/healthz, /readyz, /status) and, when tuner advertising is enabled,
// advertises the instance over mDNS. Hosted MCP/ACP validation, discovery,
// dynamic config, and federation are intentionally out of scope for now.
package smokealarm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/health"
	"github.com/james-gibson/smoke-alarm/internal/mdns"
)

// Config is smoke-alarm's configuration type. Aliasing it lets embedders build
// or load the same shape the CLI uses, with the same defaults and validation.
type Config = config.Config

// LoadConfig reads, defaults, and validates a smoke-alarm config from a YAML file.
func LoadConfig(path string) (Config, error) {
	return config.Load(path)
}

// ParseConfig reads, defaults, and validates smoke-alarm config bytes.
func ParseConfig(data []byte) (Config, error) {
	return config.LoadBytes(data)
}

// Options tunes an embedded instance.
type Options struct {
	// Version is reported by the health/status endpoints. Defaults to "dev".
	Version string
	// Logger receives structured logs. Defaults to slog.Default().
	Logger *slog.Logger
	// HealthSyncInterval controls how often engine status is mirrored onto the
	// health server. Defaults to 1s.
	HealthSyncInterval time.Duration
}

// Instance is a runnable in-process smoke-alarm.
type Instance struct {
	cfg    Config
	opts   Options
	logger *slog.Logger
	eng    *engine.Engine
	hs     *health.Server

	mu         sync.Mutex
	started    bool
	addr       string
	cancel     context.CancelFunc
	advertiser *mdns.Advertiser
}

// New builds an instance from a config. It does not start anything; call Start.
func New(cfg Config, opts Options) (*Instance, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.HealthSyncInterval <= 0 {
		opts.HealthSyncInterval = time.Second
	}

	cfg = withSelfCheck(cfg)
	eng, err := engine.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("smokealarm: build engine: %w", err)
	}

	in := &Instance{cfg: cfg, opts: opts, logger: opts.Logger, eng: eng}
	if cfg.Health.Enabled {
		hs := health.NewServer(health.Options{
			ServiceName:     cfg.Service.Name,
			Version:         opts.Version,
			ListenAddr:      cfg.Health.ListenAddr,
			HealthzPath:     cfg.Health.Endpoints.Healthz,
			ReadyzPath:      cfg.Health.Endpoints.Readyz,
			StatusPath:      cfg.Health.Endpoints.Status,
			ShutdownTimeout: durationOrDefault(cfg.Runtime.GracefulShutdownTimeout, 10*time.Second),
		})
		hs.SetSelfDescription(health.NewSelfDescriptionFactory(cfg, opts.Version, time.Now().UTC(), hs))
		hs.SetComponent("engine", false, "waiting for initial target checks")
		hs.SetReady(false, "engine not ready")
		in.hs = hs
	}
	return in, nil
}

// withSelfCheck mirrors cmd/ocd-smoke-alarm: when health.self_check is enabled,
// a target is added that probes this instance's own /status endpoint.
func withSelfCheck(cfg Config) Config {
	if !cfg.Health.Enabled || !cfg.Health.SelfCheck {
		return cfg
	}
	cfg.Targets = append(cfg.Targets, config.TargetConfig{
		ID:        "self-health-check",
		Name:      "Self Health Check",
		Enabled:   true,
		Protocol:  config.ProtocolHTTP,
		Transport: config.TransportHTTP,
		Endpoint:  fmt.Sprintf("http://%s%s", cfg.Health.ListenAddr, cfg.Health.Endpoints.Status),
		Check: config.TargetCheckConfig{
			Interval: "30s",
			Timeout:  "5s",
			Retries:  3,
		},
	})
	return cfg
}

// Start binds the health listener, starts the engine, and (if enabled) begins
// mDNS advertising. It returns once the instance is serving; the instance then
// runs until ctx is canceled or Shutdown is called.
func (in *Instance) Start(ctx context.Context) error {
	in.mu.Lock()
	if in.started {
		in.mu.Unlock()
		return errors.New("smokealarm: instance already started")
	}
	in.started = true
	runCtx, cancel := context.WithCancel(ctx)
	in.cancel = cancel
	in.mu.Unlock()

	if in.hs != nil {
		boundAddr, err := in.hs.BindWithRetry(10)
		if err != nil {
			cancel()
			return fmt.Errorf("smokealarm: bind health server: %w", err)
		}
		in.mu.Lock()
		in.addr = boundAddr
		in.mu.Unlock()

		if in.cfg.Tuner.Advertise {
			advertiser := mdns.NewAdvertiser(mdns.Options{
				ServiceName: in.cfg.Service.Name,
				ServiceType: in.cfg.Tuner.ServiceType,
				Port:        mdns.ParsePort(boundAddr),
			})
			if advErr := advertiser.Start(runCtx); advErr != nil {
				in.logger.Warn("smokealarm: mdns advertiser failed to start", "error", advErr)
			} else {
				in.mu.Lock()
				in.advertiser = advertiser
				in.mu.Unlock()
			}
		}

		go func() {
			if err := in.hs.Start(runCtx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
				in.logger.Error("smokealarm: health server stopped", "error", err)
			}
		}()
		go syncHealthLoop(runCtx, in.hs, in.eng, in.opts.HealthSyncInterval)
	}

	go func() {
		if err := in.eng.Start(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			in.logger.Error("smokealarm: engine stopped", "error", err)
		}
	}()

	return nil
}

// Shutdown stops advertising, cancels the engine, and gracefully stops the
// health server. It is safe to call more than once.
func (in *Instance) Shutdown(ctx context.Context) error {
	in.mu.Lock()
	if !in.started {
		in.mu.Unlock()
		return nil
	}
	in.started = false
	cancel := in.cancel
	advertiser := in.advertiser
	hs := in.hs
	in.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if advertiser != nil {
		advertiser.Shutdown()
	}
	if hs != nil {
		return hs.Shutdown(ctx)
	}
	return nil
}

// Addr returns the bound health listen address (for example "0.0.0.0:19101"),
// or "" before Start.
func (in *Instance) Addr() string {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.addr
}

// StatusURL returns the absolute /status URL, or "" before Start.
func (in *Instance) StatusURL() string {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.addr == "" {
		return ""
	}
	path := in.cfg.Health.Endpoints.Status
	if path == "" {
		path = "/status"
	}
	return "http://" + in.addr + path
}

// syncHealthLoop mirrors engine status onto the health server. It mirrors the
// loop used by cmd/ocd-smoke-alarm.
func syncHealthLoop(ctx context.Context, hs *health.Server, eng *engine.Engine, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, st := range eng.SnapshotStatuses() {
				hs.UpsertTargetStatus(health.TargetStatus{
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
			if eng.IsReady() {
				hs.SetComponent("engine", true, "checks active")
				hs.SetReady(true, "")
			} else {
				hs.SetComponent("engine", false, "waiting for initial target checks")
				hs.SetReady(false, "engine not ready")
			}
		}
	}
}

func durationOrDefault(raw string, def time.Duration) time.Duration {
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
