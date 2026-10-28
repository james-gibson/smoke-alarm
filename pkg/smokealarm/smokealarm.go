// Package smokealarm is a small, importable facade over smoke-alarm's internal
// subsystems. It lets a host process — for example lezz — start a smoke-alarm
// instance in-process instead of shelling out to the ocd-smoke-alarm binary
// with a generated YAML config.
//
// An instance runs the monitor engine and the health endpoints (/healthz,
// /readyz, /status) and, when tuner advertising is enabled, advertises itself
// over mDNS. The engine is wired from the config the way the CLI wires it, so
// known-state baselines (regression detection), alert sinks, and the telemetry
// exporter all take effect. Set Options.FeaturesDir to also register Gherkin
// feature files for dashboard certification status.
//
// # Subsystems that are not run in-process
//
// A smoke-alarm config can enable subsystems this facade does not run. When it
// does, New reports them — via Instance.Unsupported, a warning log, or an error
// when Options.Strict is set — rather than ignoring them silently:
//
//   - hosted MCP/ACP/HTTP/SSE validation server (hosted.enabled)
//   - discovery and dynamic config persistence (discovery.enabled,
//     dynamic_config.enabled)
//   - the federation mesh (federation.enabled)
//   - the OAuth mock redirect server (auth.oauth.mock_redirect.enabled)
//   - remote-agent managed updates (remote_agent.managed_updates), which
//     describe managing a *process* and so have no meaning in-process
//   - the foreground TUI (service.mode: foreground)
//
// The runtime lock file and PID file are also not written. An embedded instance
// is a guest in the host's process, so writing the host's PID would make
// `ops stop -pid-file` signal the host itself. A host that needs a managed
// lifecycle (ops stop/reload) should run the CLI as a subprocess instead.
package smokealarm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/alerts"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/health"
	"github.com/james-gibson/smoke-alarm/internal/knownstate"
	"github.com/james-gibson/smoke-alarm/internal/mdns"
	"github.com/james-gibson/smoke-alarm/internal/telemetry"
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
	// FeaturesDir, when non-empty, registers the *.feature files it contains
	// with the health server so dashboards can display this instance's
	// certification status. The CLI registers from "features".
	FeaturesDir string
	// Strict makes New return an error, instead of logging a warning, when the
	// config enables a subsystem this facade does not run. See the package doc
	// for the list.
	Strict bool
}

// Instance is a runnable in-process smoke-alarm.
type Instance struct {
	cfg    Config
	opts   Options
	logger *slog.Logger
	eng    *engine.Engine
	hs     *health.Server

	unsupported []string

	mu         sync.Mutex
	started    bool
	addr       string
	cancel     context.CancelFunc
	advertiser *mdns.Advertiser
	// engineDone is closed when the goroutine running eng.Start returns. It
	// exists so Shutdown can join that goroutine: the engine saves the
	// known-state baseline on its way out, and that write must not land after
	// the caller has torn down the directory the baseline lives in.
	engineDone chan struct{}
}

// New builds an instance from a config. It does not start anything; call Start.
//
// New applies config defaults, then wires the engine exactly as
// cmd/ocd-smoke-alarm does. If the config enables a subsystem this facade does
// not run, New reports it instead of ignoring it — see Options.Strict.
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

	// New may be handed a hand-built Config as well as one from LoadConfig, so
	// apply defaults here as the CLI does before wiring anything. ApplyDefaults
	// only fills blank scalars — it never turns an Enabled flag on.
	cfg.ApplyDefaults()
	cfg = withSelfCheck(cfg)

	unsupported := unsupportedEnabled(cfg)
	if len(unsupported) > 0 {
		if opts.Strict {
			return nil, fmt.Errorf(
				"smokealarm: config enables subsystems this facade does not run: %s",
				strings.Join(unsupported, ", "))
		}
		opts.Logger.Warn(
			"smokealarm: config enables subsystems this facade does not run; they will be inactive (set Options.Strict to make this an error)",
			"subsystems", strings.Join(unsupported, ", "))
	}

	eng, err := engine.New(cfg, engineOptions(cfg, opts.Logger)...)
	if err != nil {
		return nil, fmt.Errorf("smokealarm: build engine: %w", err)
	}

	in := &Instance{
		cfg:         cfg,
		opts:        opts,
		logger:      opts.Logger,
		eng:         eng,
		unsupported: unsupported,
	}
	if cfg.Health.Enabled {
		hs := health.NewServer(health.Options{
			ServiceName:     cfg.Service.Name,
			Version:         opts.Version,
			ListenAddr:      cfg.Health.ListenAddr,
			HealthzPath:     cfg.Health.Endpoints.Healthz,
			ReadyzPath:      cfg.Health.Endpoints.Readyz,
			StatusPath:      cfg.Health.Endpoints.Status,
			ShutdownTimeout: durationOrDefault(cfg.Runtime.GracefulShutdownTimeout, 10*time.Second),
			IsotopeTTL:      durationOrDefault(cfg.Health.IsotopeTTL, health.DefaultIsotopeTTL),
		})
		hs.SetSelfDescription(health.NewSelfDescriptionFactory(cfg, opts.Version, time.Now().UTC(), hs))
		if opts.FeaturesDir != "" {
			health.RegisterFeaturesFromDir(hs, opts.FeaturesDir)
		}
		hs.SetComponent("engine", false, "waiting for initial target checks")
		hs.SetReady(false, "engine not ready")
		in.hs = hs
	}
	return in, nil
}

// engineOptions mirrors the options cmd/ocd-smoke-alarm passes to engine.New.
//
// It matters: the known-state store, the notifier, and the telemetry exporter
// are reachable only through options, so passing none (as a bare
// engine.New(cfg) does) leaves known_state.enabled, runtime.baseline_file, and
// the alerts sinks parsed, validated, and silently inert.
func engineOptions(cfg Config, logger *slog.Logger) []engine.Option {
	var opts []engine.Option

	// The engine guards on store != nil, so without this there is no baseline
	// and therefore no regression detection.
	if cfg.KnownState.Enabled {
		opts = append(opts, engine.WithStore(knownstate.NewStore(
			cfg.Runtime.BaselineFile,
			knownstate.WithAutoPersist(cfg.KnownState.Persist),
			knownstate.WithSustainSuccess(cfg.KnownState.SustainSuccessBeforeMarkHealthy),
		)))
	}

	if notifier := alerts.NotifierFromConfig(cfg, logger); notifier != nil {
		opts = append(opts, engine.WithNotifier(notifier))
	}

	if cfg.Telemetry.Enabled && cfg.Telemetry.Endpoint != "" {
		telem, err := telemetry.NewExporter(cfg.Telemetry.Endpoint, cfg.Telemetry.ServiceName)
		if err != nil {
			// Telemetry must not stop an instance from running; warn and carry
			// on, as the CLI does.
			logger.Warn("smokealarm: telemetry disabled", "endpoint", cfg.Telemetry.Endpoint, "error", err)
		} else if telem != nil {
			opts = append(opts, engine.WithTelemetry(telem))
		}
	}

	return opts
}

// unsupportedEnabled names the config's enabled-but-unimplemented subsystems, so
// the omission is reported rather than silent. Sorted, so logs and tests are
// deterministic.
func unsupportedEnabled(cfg Config) []string {
	var out []string
	if cfg.Hosted.Enabled {
		out = append(out, "hosted validation server (hosted.enabled)")
	}
	if cfg.Discovery.Enabled {
		out = append(out, "discovery (discovery.enabled)")
	}
	if cfg.DynamicConfig.Enabled {
		out = append(out, "dynamic config persistence (dynamic_config.enabled)")
	}
	if cfg.Federation.Enabled {
		out = append(out, "federation (federation.enabled)")
	}
	if cfg.Auth.OAuth.MockRedirect.Enabled {
		out = append(out, "oauth mock redirect server (auth.oauth.mock_redirect.enabled)")
	}
	if cfg.RemoteAgent.ManagedUpdates {
		out = append(out, "remote agent managed updates (remote_agent.managed_updates)")
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Service.Mode), config.ModeForeground) {
		out = append(out, "foreground TUI (service.mode)")
	}
	sort.Strings(out)
	return out
}

// Unsupported returns the subsystems this instance's config enabled that the
// facade does not run, in a stable order. Empty when everything requested is
// active.
func (in *Instance) Unsupported() []string {
	return append([]string(nil), in.unsupported...)
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
	in.engineDone = make(chan struct{})
	engineDone := in.engineDone
	in.mu.Unlock()

	if in.hs != nil {
		boundAddr, err := in.hs.BindWithRetry(10)
		if err != nil {
			cancel()
			// Nothing below launches the engine, so close the channel the
			// goroutine would have: a Shutdown after a failed Start must not
			// wait on a join that can never happen.
			close(engineDone)
			return fmt.Errorf("smokealarm: bind health server: %w", err)
		}
		in.mu.Lock()
		in.addr = boundAddr
		in.mu.Unlock()

		// A self-check target names the configured health address. When the health
		// listener yields to the next free port, the probe must follow it or the
		// instance reports its own /status as an outage (smoke-alarm#53).
		in.mu.Lock()
		configuredAddr := in.cfg.Health.ListenAddr
		in.mu.Unlock()
		if boundAddr != configuredAddr {
			in.mu.Lock()
			rewritten := config.RewriteSelfReferences(in.cfg, map[string]string{configuredAddr: boundAddr})
			in.mu.Unlock()
			if _, reloadErr := in.eng.ReloadConfig(rewritten); reloadErr != nil {
				in.logger.Warn("smokealarm: could not point self-check target at the bound health port",
					"bound", boundAddr, "error", reloadErr)
			} else {
				in.mu.Lock()
				in.cfg = rewritten
				in.mu.Unlock()
				in.logger.Warn("smokealarm: health port in use, using next available",
					"preferred", configuredAddr, "actual", boundAddr)
			}
		}

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
		defer close(engineDone)
		if err := in.eng.Start(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			in.logger.Error("smokealarm: engine stopped", "error", err)
		}
	}()

	return nil
}

// Shutdown stops advertising, cancels the engine, and gracefully stops the
// health server. It is safe to call more than once.
//
// Shutdown does not return until the engine has stopped. That matters for more
// than tidiness: the engine persists the known-state baseline as it exits, so a
// caller that removed or unmounted runtime.baseline_file's directory the moment
// Shutdown returned would race that last write.
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
	engineDone := in.engineDone
	in.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if advertiser != nil {
		advertiser.Shutdown()
	}

	var hsErr error
	if hs != nil {
		hsErr = hs.Shutdown(ctx)
	}

	// Joining the engine is what makes the baseline write above safe; when ctx
	// runs out first, report that rather than pretending the instance stopped.
	if err := waitClosed(ctx, engineDone); err != nil {
		return errors.Join(hsErr, err)
	}
	return hsErr
}

// waitClosed blocks until ch is closed or ctx ends. A nil channel means the
// goroutine it would have tracked was never started, so there is nothing to
// wait for.
func waitClosed(ctx context.Context, ch <-chan struct{}) error {
	if ch == nil {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Reload applies a new configuration in place, mirroring the CLI's SIGHUP path:
// the engine's target set is swapped without restarting, and targets the reload
// dropped are withdrawn from the health server (which only ever upserts).
//
// The config is normalized the way New normalizes one — defaults plus the
// self-check target — so a reload cannot silently lose the self-check probe.
// Listeners bound at Start (health, mDNS) are left untouched.
func (in *Instance) Reload(cfg Config) error {
	cfg.ApplyDefaults()
	cfg = withSelfCheck(cfg)

	removed, err := in.eng.ReloadConfig(cfg)
	if err != nil {
		return err
	}

	in.mu.Lock()
	in.cfg = cfg
	hs := in.hs
	in.mu.Unlock()

	if hs != nil {
		for _, id := range removed {
			hs.RemoveTarget(id)
		}
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
// loop used by cmd/ocd-smoke-alarm, including the ownership-based withdrawal of
// targets that a Reload dropped: the health server only ever upserts, and
// federated peer targets are never owned here, so pruning leaves them alone.
func syncHealthLoop(ctx context.Context, hs *health.Server, eng *engine.Engine, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	owned := make(map[string]struct{})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := make(map[string]struct{})
			for _, id := range eng.TargetIDs() {
				current[id] = struct{}{}
			}
			for _, st := range eng.SnapshotStatuses() {
				current[st.TargetID] = struct{}{}
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
			for id := range owned {
				if _, ok := current[id]; !ok {
					hs.RemoveTarget(id)
				}
			}
			owned = current

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
