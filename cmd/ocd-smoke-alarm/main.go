package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/alerts"
	"github.com/james-gibson/smoke-alarm/internal/auth"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/dynamicconfig"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/federation"
	"github.com/james-gibson/smoke-alarm/internal/health"
	"github.com/james-gibson/smoke-alarm/internal/hosted"
	"github.com/james-gibson/smoke-alarm/internal/knownstate"
	"github.com/james-gibson/smoke-alarm/internal/mdns"
	"github.com/james-gibson/smoke-alarm/internal/meta"
	"github.com/james-gibson/smoke-alarm/internal/safety"
	"github.com/james-gibson/smoke-alarm/internal/targets"
	"github.com/james-gibson/smoke-alarm/internal/telemetry"
	"github.com/james-gibson/smoke-alarm/internal/ui"
)

const (
	appName = "ocd-smoke-alarm"
	version = "0.1.0"
)

var (
	demoMode            bool
	demoForceForeground bool
)

func main() {
	args := os.Args[1:]

	// Default behavior: if no subcommand is provided, run in serve mode.
	if len(args) == 0 {
		if err := cmdServe(nil); err != nil {
			fatal(err)
		}
		return
	}

	switch args[0] {
	case "serve":
		if err := cmdServe(args[1:]); err != nil {
			fatal(err)
		}
	case "tui":
		if err := cmdTUI(args[1:]); err != nil {
			fatal(err)
		}
	case "check":
		if err := cmdCheck(args[1:]); err != nil {
			fatal(err)
		}
	case "discover":
		if err := cmdDiscover(args[1:]); err != nil {
			fatal(err)
		}
	case "validate":
		if err := cmdValidate(args[1:]); err != nil {
			fatal(err)
		}
	case "gen-meta":
		if err := cmdGenMeta(args[1:]); err != nil {
			fatal(err)
		}
	case "dynamic-config":
		if err := cmdDynamicConfig(args[1:]); err != nil {
			fatal(err)
		}
	case "demo":
		if err := cmdDemo(args[1:]); err != nil {
			fatal(err)
		}
	case "ops":
		if err := cmdOps(args[1:]); err != nil {
			fatal(err)
		}
	case "tuner":
		if err := cmdTuner(args[1:]); err != nil {
			fatal(err)
		}
	case "version", "--version", "-v":
		fmt.Println(version)
	default:
		printRootUsage()
		os.Exit(2)
	}
}

func printRootUsage() {
	fmt.Fprintf(os.Stderr, `%s %s

Usage:
  %s serve          --config=... [--mode=foreground|background|headless] [--health-addr=host:port] [--json]
  %s tui            --addr=host:port [--refresh=1s] [--json]
  %s check          --config=... [--json]
  %s discover       --config=... [--json]
  %s validate       --config=...
  %s gen-meta       --config=... [--out-dir=...] [--formats=yaml,json]
  %s dynamic-config <persist|list|show|index> [flags]
  %s demo           [--config=...] [--health-addr=host:port] [--mode=foreground]
  %s ops            <stop|status|reload|self-check> [flags]
  %s tuner          <discover|status|audience> [flags]
  %s version

`, appName, version, appName, appName, appName, appName, appName, appName, appName, appName, appName, appName, appName)
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
	modeOverride := fs.String("mode", "", "Run mode override: foreground|background|headless")
	healthAddrOverride := fs.String("health-addr", "", "Health listen addr override, e.g. 127.0.0.1:8088")
	telemetryOverride := fs.String("telemetry", "", "OTEL collector endpoint (e.g., http://localhost:4318/v1/metrics)")
	federationOverride := fs.String("federation", "", "Federation endpoints (upstream:127.0.0.1:8088,downstream:127.0.0.1:8081,127.0.0.1:8082)")
	stateDir := fs.String("state-dir", "", "State directory (default: ./state)")
	lockFile := fs.String("lock-file", "", "Lock file path (default: /tmp/<service-name>.lock)")
	nameOverride := fs.String("name", "", "Service name (default: ocd-smoke-alarm)")
	enableTUI := fs.Bool("tui", true, "Enable TUI (ignored in headless mode)")
	jsonLogs := fs.Bool("json", false, "Output JSON logs to stdout (default: false, TUI owns console)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *modeOverride != "" {
		cfg.Service.Mode = *modeOverride
	}
	validModes := map[string]bool{
		config.ModeForeground: true,
		config.ModeBackground: true,
		config.ModeHeadless:   true,
	}
	if !validModes[strings.ToLower(cfg.Service.Mode)] {
		return fmt.Errorf("invalid mode %q: must be one of: foreground, background, headless (or use 'tui' command for remote TUI)", cfg.Service.Mode)
	}
	if *healthAddrOverride != "" {
		cfg.Health.ListenAddr = *healthAddrOverride
	}
	if *telemetryOverride != "" {
		cfg.Telemetry.Enabled = true
		cfg.Telemetry.Endpoint = *telemetryOverride
	}
	if *federationOverride != "" {
		parseFederationFlag(*federationOverride, &cfg)
	}
	if *nameOverride != "" {
		cfg.Service.Name = *nameOverride
	}
	if *stateDir != "" {
		cfg.Runtime.StateDir = *stateDir
	}
	if *lockFile != "" {
		cfg.Runtime.LockFile = *lockFile
	}
	cfg.ApplyDefaults()
	applyDemoOverrides(&cfg)
	ensureDockerLLMSTxt(&cfg)
	if err := cfg.Validate(); err != nil {
		return err
	}

	logger := buildLogger(cfg.Service.LogLevel, *jsonLogs)
	if logger != nil {
		logger.Info("starting service",
			"service", cfg.Service.Name,
			"mode", cfg.Service.Mode,
			"health_addr", cfg.Health.ListenAddr,
			"targets", len(cfg.EnabledTargets()),
		)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	lock, err := acquireLock(cfg.Runtime.LockFile)
	if err != nil {
		return fmt.Errorf("acquire runtime lock: %w", err)
	}
	defer releaseLock(lock, cfg.Runtime.LockFile)

	pidFile := filepath.Join(cfg.Runtime.StateDir, appName+".pid")
	if err := writePIDFile(pidFile); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	defer func() { _ = os.Remove(pidFile) }()

	var engineOpts []engine.Option

	if cfg.KnownState.Enabled {
		store := knownstate.NewStore(
			cfg.Runtime.BaselineFile,
			knownstate.WithAutoPersist(cfg.KnownState.Persist),
			knownstate.WithSustainSuccess(cfg.KnownState.SustainSuccessBeforeMarkHealthy),
		)
		engineOpts = append(engineOpts, engine.WithStore(store))
	}

	groupNotifier := buildNotifiers(cfg, logger, *jsonLogs)
	if groupNotifier != nil {
		engineOpts = append(engineOpts, engine.WithNotifier(groupNotifier))
	}

	if cfg.Telemetry.Enabled && cfg.Telemetry.Endpoint != "" {
		telem, err := telemetry.NewExporter(cfg.Telemetry.Endpoint, cfg.Telemetry.ServiceName)
		if err != nil && logger != nil {
			logger.Warn("telemetry init failed", "error", err.Error())
		} else if telem != nil {
			engineOpts = append(engineOpts, engine.WithTelemetry(telem))
			if logger != nil {
				logger.Info("telemetry enabled", "endpoint", cfg.Telemetry.Endpoint)
			}
		}
	}

	if cfg.Health.Enabled && cfg.Health.SelfCheck {
		selfCheckTarget := config.TargetConfig{
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
		}
		cfg.Targets = append(cfg.Targets, selfCheckTarget)
	}

	eng, err := engine.New(cfg, engineOpts...)
	if err != nil {
		return fmt.Errorf("build engine: %w", err)
	}

	var hostedSrv *hosted.Server
	if cfg.Hosted.Enabled {
		hostedSrv = hosted.NewServer(hosted.Options{
			ServiceName: cfg.Service.Name,
			Version:     version,
			ListenAddr:  cfg.Hosted.ListenAddr,

			EnableHTTP: hasStringFold(cfg.Hosted.Transports, "http"),
			EnableSSE:  hasStringFold(cfg.Hosted.Transports, "sse"),

			EnableMCP: hasStringFold(cfg.Hosted.Protocols, "mcp"),
			EnableACP: hasStringFold(cfg.Hosted.Protocols, "acp"),
			EnableA2A: hasStringFold(cfg.Hosted.Protocols, "a2a"),

			MCPEndpoint: cfg.Hosted.Endpoints.MCP,
			ACPEndpoint: cfg.Hosted.Endpoints.ACP,
			A2AEndpoint: cfg.Hosted.Endpoints.A2A,

			ShutdownTimeout: mustDuration(cfg.Runtime.GracefulShutdownTimeout, 10*time.Second),
		})
	}

	var oauthMockSrv *auth.MockRedirectServer
	if cfg.Auth.OAuth.MockRedirect.Enabled {
		oauthMockSrv = auth.NewMockRedirectServer(auth.MockRedirectOptions{
			ListenAddr:      cfg.Auth.OAuth.MockRedirect.ListenAddr,
			Path:            cfg.Auth.OAuth.MockRedirect.Path,
			Mode:            auth.MockRedirectMode(strings.ToLower(strings.TrimSpace(cfg.Auth.OAuth.MockRedirect.Mode))),
			ShutdownTimeout: mustDuration(cfg.Runtime.GracefulShutdownTimeout, 10*time.Second),
		})
	}

	var hs *health.Server
	if cfg.Health.Enabled {
		startedAt := time.Now().UTC()
		hs = health.NewServer(health.Options{
			ServiceName:     cfg.Service.Name,
			Version:         version,
			ListenAddr:      cfg.Health.ListenAddr,
			HealthzPath:     cfg.Health.Endpoints.Healthz,
			ReadyzPath:      cfg.Health.Endpoints.Readyz,
			StatusPath:      cfg.Health.Endpoints.Status,
			ShutdownTimeout: mustDuration(cfg.Runtime.GracefulShutdownTimeout, 10*time.Second),
		})
		// Wire self-description after server creation so the factory can read runtime state.
		hs.SetSelfDescription(health.NewSelfDescriptionFactory(cfg, version, startedAt, hs))
		hs.SetComponent("engine", false, "waiting for first checks")
		hs.SetReady(false, "engine not ready")

		// Pre-bind with port scanning so we advertise the actual bound port via mDNS.
		boundAddr, err := hs.BindWithRetry(10)
		if err != nil {
			return fmt.Errorf("health server bind: %w", err)
		}
		if boundAddr != cfg.Health.ListenAddr {
			if logger != nil {
				logger.Warn("health server port in use, using next available",
					"preferred", cfg.Health.ListenAddr, "actual", boundAddr)
			}
		}

		// Start mDNS advertiser if enabled for tuner discovery.
		if cfg.Tuner.Advertise {
			advertiser := mdns.NewAdvertiser(mdns.Options{
				ServiceName: cfg.Service.Name,
				ServiceType: cfg.Tuner.ServiceType,
				Port:        mdns.ParsePort(boundAddr),
			})
			if err := advertiser.Start(ctx); err != nil {
				if logger != nil {
					logger.Warn("mdns advertiser failed to start", "err", err)
				}
			} else {
				if logger != nil {
					logger.Info("mdns advertising service", "type", cfg.Tuner.ServiceType, "addr", boundAddr)
				}
				defer advertiser.Shutdown()
			}
		}
	}

	// Optional one-shot discovery status (fast, low overhead).
	if cfg.Discovery.Enabled {
		ds := discovery.New()
		res := ds.Discover(ctx, cfg)

		if hs != nil {
			if len(res.Errors) > 0 {
				hs.SetComponent("discovery", false, strings.Join(res.Errors, "; "))
			} else {
				hs.SetComponent("discovery", true, fmt.Sprintf("%d records discovered", len(res.Records)))
			}
		}

		if cfg.DynamicConfig.Enabled && len(res.Records) > 0 {
			store := dynamicconfig.NewStoreFromConfig(cfg.DynamicConfig)
			artifacts, saveErr := store.SaveDiscoveryRecords(ctx, res.Records)
			if saveErr != nil {
				if logger != nil {
					logger.Warn("dynamic config persistence failed", "error", saveErr.Error())
				}
				if hs != nil {
					hs.SetComponent("dynamic_config", false, saveErr.Error())
				}
			} else {
				if logger != nil {
					logger.Info("dynamic configs persisted from startup discovery", "artifacts", len(artifacts))
				}
				if hs != nil {
					hs.SetComponent("dynamic_config", true, fmt.Sprintf("%d artifacts persisted", len(artifacts)))
				}
			}
		}
	}

	errCh := make(chan error, 5)

	// Start support services first so self-hosted targets are reachable before probes begin.
	if hs != nil {
		go func() {
			if err := hs.Start(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("health server: %w", err)
			}
		}()
	}

	if hostedSrv != nil {
		if hs != nil {
			hs.SetComponent("hosted", true, "running")
		}
		go func() {
			if err := hostedSrv.Start(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
				if hs != nil {
					hs.SetComponent("hosted", false, err.Error())
				}
				errCh <- fmt.Errorf("hosted server: %w", err)
			}
		}()
	}

	if oauthMockSrv != nil {
		if hs != nil {
			hs.SetComponent("oauth_redirect_mock", true, "running")
		}
		go func() {
			if err := oauthMockSrv.Start(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
				if hs != nil {
					hs.SetComponent("oauth_redirect_mock", false, err.Error())
				}
				errCh <- fmt.Errorf("oauth redirect mock server: %w", err)
			}
		}()
	}

	// Start monitor engine after hosted/mock services to reduce startup race false outages.
	go func() {
		if err := eng.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- fmt.Errorf("engine: %w", err)
		}
	}()

	// Start deterministic federation orchestration if enabled.
	if cfg.Federation.Enabled {
		slot, err := federation.ClaimSlot(federation.SlotOptions{
			BasePort:    cfg.Federation.BasePort,
			MaxPort:     cfg.Federation.MaxPort,
			StateDir:    cfg.Runtime.StateDir,
			ServiceName: cfg.Service.Name,
		})
		if err != nil {
			return fmt.Errorf("claim federation slot: %w", err)
		}
		defer func() { _ = slot.Close() }()

		announceInterval := mustDuration(cfg.Federation.AnnounceInterval, 10*time.Second)
		heartbeatInterval := mustDuration(cfg.Federation.HeartbeatInterval, 15*time.Second)
		heartbeatTimeout := mustDuration(cfg.Federation.HeartbeatTimeout, 45*time.Second)

		var registry *federation.Registry
		registryOpts := federation.RegistryOptions{
			StateDir:         cfg.Runtime.StateDir,
			AnnounceInterval: announceInterval,
			HeartbeatTimeout: heartbeatTimeout,
		}
		registryOpts.OnChange = func(ev federation.Event) {
			if hs != nil && registry != nil {
				snap := registry.Snapshot()
				hs.SetComponent("federation", true, fmt.Sprintf("role=%s peers=%d", slot.Identity.Role, len(snap.Peers)))
			}
		}
		registry, err = federation.NewRegistry(slot.Identity, registryOpts)
		if err != nil {
			return fmt.Errorf("federation registry: %w", err)
		}
		if logger != nil {
			logger.Info("federation slot claimed",
				"instance_id", slot.Identity.ID,
				"role", slot.Identity.Role,
				"port", slot.Identity.Port,
			)
		}
		if hs != nil {
			hs.SetComponent("federation", true, fmt.Sprintf("role=%s peers=0", slot.Identity.Role))
		}

		switch slot.Identity.Role {
		case federation.RoleIntroducer:
			cfg.Federation.Rank = slot.Identity.Port
			server, err := federation.NewServer(federation.ServerOptions{
				Listener: slot.Listener,
				Registry: registry,
				Logger:   logger,
			})
			if err != nil {
				return fmt.Errorf("federation server: %w", err)
			}
			go func() {
				if err := server.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
					errCh <- fmt.Errorf("federation introducer: %w", err)
				}
			}()
		default:
			introducerURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.Federation.BasePort)
			if logger != nil {
				logger.Info("federation follower using default introducer url",
					"introducer_url", introducerURL,
				)
			}
			client, err := federation.NewClient(federation.ClientOptions{
				IntroducerURL:     introducerURL,
				Registry:          registry,
				Logger:            logger,
				AnnounceInterval:  announceInterval,
				HeartbeatInterval: heartbeatInterval,
			})
			if err != nil {
				return fmt.Errorf("federation client: %w", err)
			}
			go client.Start(ctx)
		}
	}

	if hs != nil {
		go syncHealthLoop(ctx, hs, eng)
	}

	switch strings.ToLower(cfg.Service.Mode) {
	case config.ModeForeground:
		if *enableTUI {
			title := "OCD Smoke Alarm"
			if demoMode {
				title = "OCD Smoke Alarm • Demo"
			}
			if err := ui.Run(ctx, eng, ui.Options{
				RefreshInterval: 1 * time.Second,
				HeaderTitle:     title,
				MaxEvents:       12,
				ShowHelp:        true,
				DemoMode:        demoMode,
			}); err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("foreground ui: %w", err)
			}
			cancel()
			break
		}
		fallthrough
	case config.ModeBackground, config.ModeHeadless:
		select {
		case <-ctx.Done():
		case err := <-errCh:
			cancel()
			return err
		}

	default:
		return fmt.Errorf("unsupported mode %q", cfg.Service.Mode)
	}

	if oauthMockSrv != nil {
		_ = oauthMockSrv.Shutdown(context.Background())
	}
	if hostedSrv != nil {
		_ = hostedSrv.Shutdown(context.Background())
	}
	if hs != nil {
		_ = hs.Shutdown(context.Background())
	}
	return nil
}

func applyDemoOverrides(cfg *config.Config) {
	if cfg == nil || !demoMode {
		return
	}

	if demoForceForeground {
		cfg.Service.Mode = config.ModeForeground
	}

	// Ensure hosted exploratory services are available.
	cfg.Hosted.Enabled = true
	cfg.Hosted.ListenAddr = fallbackStr(cfg.Hosted.ListenAddr, "127.0.0.1:18091")
	cfg.Hosted.Transports = ensureStringFoldContains(cfg.Hosted.Transports, "http")
	cfg.Hosted.Transports = ensureStringFoldContains(cfg.Hosted.Transports, "sse")
	cfg.Hosted.Protocols = ensureStringFoldContains(cfg.Hosted.Protocols, "mcp")
	cfg.Hosted.Protocols = ensureStringFoldContains(cfg.Hosted.Protocols, "acp")

	// Enable discovery and dynamic config persistence so demo interactions are recorded.
	cfg.Discovery.Enabled = true
	cfg.DynamicConfig.Enabled = true

	// Enable deterministic OAuth callback handling for exploration demos.
	cfg.Auth.OAuth.MockRedirect.Enabled = true
	cfg.Auth.OAuth.MockRedirect.ListenAddr = fallbackStr(cfg.Auth.OAuth.MockRedirect.ListenAddr, "127.0.0.1:8877")
	cfg.Auth.OAuth.MockRedirect.Path = fallbackStr(cfg.Auth.OAuth.MockRedirect.Path, "/oauth/callback")
	cfg.Auth.OAuth.MockRedirect.Mode = fallbackStr(cfg.Auth.OAuth.MockRedirect.Mode, "allow")
}

func ensureDockerLLMSTxt(cfg *config.Config) {
	if cfg == nil {
		return
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		return
	}

	var hasDockerConfig bool
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)

		if strings.EqualFold(name, "Dockerfile") ||
			strings.HasPrefix(lower, "dockerfile") ||
			strings.HasPrefix(lower, "docker-compose") ||
			strings.HasSuffix(lower, ".dockerfile") ||
			(entry.IsDir() && (lower == "docker" || strings.HasPrefix(lower, "docker-"))) {
			hasDockerConfig = true
			break
		}
	}

	if !hasDockerConfig {
		return
	}

	const dockerLLMSTxt = "https://docs.docker.com/llms.txt"

	if !cfg.Discovery.Enabled {
		cfg.Discovery.Enabled = true
	}

	llms := &cfg.Discovery.LLMSTxt
	if !llms.Enabled {
		llms.Enabled = true
	}
	if !llms.RequireHTTPS {
		llms.RequireHTTPS = true
	}

	for _, existing := range llms.RemoteURIs {
		if strings.EqualFold(existing, dockerLLMSTxt) {
			return
		}
	}

	llms.RemoteURIs = append(llms.RemoteURIs, dockerLLMSTxt)
}

func fallbackStr(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return v
}

func syncHealthLoop(ctx context.Context, hs *health.Server, eng *engine.Engine) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			statuses := eng.SnapshotStatuses()
			for _, st := range statuses {
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

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
	jsonOut := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := runOneShotChecks(ctx, cfg)
	if err != nil {
		return err
	}

	if *jsonOut {
		return writeJSON(os.Stdout, map[string]any{
			"service": cfg.Service.Name,
			"time":    time.Now().UTC(),
			"results": results,
		})
	}

	printCheckTable(results)

	failed := 0
	for _, r := range results {
		if r.IsFailure() {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d checks failing", failed, len(results))
	}
	return nil
}

func cmdDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
	jsonOut := fs.Bool("json", true, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ds := discovery.New()
	res := ds.Discover(ctx, cfg)

	var artifacts []dynamicconfig.SavedArtifact
	if cfg.DynamicConfig.Enabled && len(res.Records) > 0 {
		store := dynamicconfig.NewStoreFromConfig(cfg.DynamicConfig)
		var saveErr error
		artifacts, saveErr = store.SaveDiscoveryRecords(ctx, res.Records)
		if saveErr != nil {
			return fmt.Errorf("persist dynamic configs from discovery: %w", saveErr)
		}
	}

	if *jsonOut {
		return writeJSON(os.Stdout, map[string]any{
			"discovery": res,
			"dynamic_config": map[string]any{
				"enabled":   cfg.DynamicConfig.Enabled,
				"artifacts": artifacts,
			},
		})
	}

	fmt.Printf("started:  %s\n", res.StartedAt.Format(time.RFC3339))
	fmt.Printf("finished: %s\n", res.FinishedAt.Format(time.RFC3339))
	fmt.Printf("records:  %d\n", len(res.Records))
	if len(res.Errors) > 0 {
		fmt.Printf("errors:   %d\n", len(res.Errors))
	}
	for _, rec := range res.Records {
		fmt.Printf("- %-24s %-6s %-9s %s\n",
			rec.Target.ID, rec.Target.Protocol, rec.Source, rec.Target.Endpoint)
	}
	if len(artifacts) > 0 {
		fmt.Printf("dynamic configs persisted: %d\n", len(artifacts))
		for _, a := range artifacts {
			fmt.Printf("  - [%s] %s (id=%s, target=%s)\n", a.Format, a.Path, a.ID, a.TargetID)
		}
	}
	return nil
}

func cmdDynamicConfig(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("dynamic-config requires subcommand: persist|list|show|index")
	}

	switch args[0] {
	case "persist":
		fs := flag.NewFlagSet("dynamic-config persist", flag.ContinueOnError)
		configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
		jsonOut := fs.Bool("json", false, "Output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}
		if !cfg.DynamicConfig.Enabled {
			return fmt.Errorf("dynamic_config.enabled is false in %s", *configPath)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		res := discovery.New().Discover(ctx, cfg)
		store := dynamicconfig.NewStoreFromConfig(cfg.DynamicConfig)
		artifacts, err := store.SaveDiscoveryRecords(ctx, res.Records)
		if err != nil {
			return err
		}

		if *jsonOut {
			return writeJSON(os.Stdout, map[string]any{
				"discovery_records": len(res.Records),
				"errors":            res.Errors,
				"artifacts":         artifacts,
			})
		}

		fmt.Printf("persisted dynamic configs: %d artifact(s)\n", len(artifacts))
		for _, a := range artifacts {
			fmt.Printf("- [%s] %s (id=%s, target=%s, serve=%s)\n", a.Format, a.Path, a.ID, a.TargetID, a.ServeURL)
		}
		if len(res.Errors) > 0 {
			fmt.Printf("discovery warnings: %d\n", len(res.Errors))
			for _, e := range res.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}
		return nil

	case "list":
		fs := flag.NewFlagSet("dynamic-config list", flag.ContinueOnError)
		configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
		dir := fs.String("dir", "", "Directory override")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		targetDir := strings.TrimSpace(*dir)
		if targetDir == "" {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			targetDir = cfg.DynamicConfig.Directory
		}

		entries, err := os.ReadDir(targetDir)
		if err != nil {
			return err
		}

		type item struct {
			name string
		}
		var items []item
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(strings.ToLower(name), ".json") || strings.HasSuffix(strings.ToLower(name), ".md") {
				items = append(items, item{name: name})
			}
		}
		sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })

		fmt.Printf("dynamic config files in %s:\n", targetDir)
		for _, it := range items {
			fmt.Printf("- %s\n", filepath.Join(targetDir, it.name))
		}
		return nil

	case "show":
		fs := flag.NewFlagSet("dynamic-config show", flag.ContinueOnError)
		configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
		dir := fs.String("dir", "", "Directory override")
		id := fs.String("id", "", "Dynamic config ID")
		format := fs.String("format", "json", "Format: json|markdown")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(*id) == "" {
			return fmt.Errorf("--id is required")
		}

		targetDir := strings.TrimSpace(*dir)
		if targetDir == "" {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			targetDir = cfg.DynamicConfig.Directory
		}

		ext := ".json"
		if strings.EqualFold(strings.TrimSpace(*format), "markdown") || strings.EqualFold(strings.TrimSpace(*format), "md") {
			ext = ".md"
		}
		path := filepath.Join(targetDir, strings.TrimSpace(*id)+ext)

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Print(string(b))
		return nil

	case "index":
		fs := flag.NewFlagSet("dynamic-config index", flag.ContinueOnError)
		configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
		dir := fs.String("dir", "", "Directory override")
		out := fs.String("out", "", "Output markdown index path (default: <dir>/index.md)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}

		targetDir := strings.TrimSpace(*dir)
		if targetDir == "" {
			targetDir = cfg.DynamicConfig.Directory
		}
		if strings.TrimSpace(targetDir) == "" {
			return fmt.Errorf("dynamic config directory is empty")
		}

		entries, err := os.ReadDir(targetDir)
		if err != nil {
			return err
		}

		baseURL := strings.TrimRight(cfg.DynamicConfig.ServeBaseURL, "/")
		if baseURL == "" {
			baseURL = "/dynamic-config"
		}

		type fileItem struct {
			name string
		}
		var files []fileItem
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(strings.ToLower(name), ".json") || strings.HasSuffix(strings.ToLower(name), ".md") {
				files = append(files, fileItem{name: name})
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })

		var b strings.Builder
		b.WriteString("# Dynamic Config Index\n\n")
		b.WriteString(fmt.Sprintf("- Generated at: `%s`\n", time.Now().UTC().Format(time.RFC3339)))
		b.WriteString(fmt.Sprintf("- Directory: `%s`\n", targetDir))
		b.WriteString(fmt.Sprintf("- Serve Base URL: `%s`\n\n", baseURL))
		for _, f := range files {
			b.WriteString(fmt.Sprintf("- [%s](%s/%s)\n", f.name, baseURL, f.name))
		}

		outPath := strings.TrimSpace(*out)
		if outPath == "" {
			outPath = filepath.Join(targetDir, "index.md")
		}
		if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote dynamic config index: %s\n", outPath)
		return nil

	default:
		return fmt.Errorf("unknown dynamic-config subcommand %q", args[0])
	}
}

func cmdDemo(args []string) error {
	demoMode = true

	// Force foreground unless explicitly overridden.
	hasMode := false
	for _, a := range args {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a)), "--mode=") {
			hasMode = true
			break
		}
	}
	demoForceForeground = !hasMode

	defer func() {
		demoMode = false
		demoForceForeground = false
	}()

	if demoForceForeground {
		args = append(args, "--mode=foreground")
	}

	fmt.Println("demo mode enabled: hosted MCP/ACP + OAuth redirect mock + dynamic config persistence")
	fmt.Println("tip: connect external agents to hosted endpoints and watch TUI/topology updates in real time")
	return cmdServe(args)
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
	machine := fs.Bool("machine", false, "Output machine-parsable JSON format")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		if *machine {
			out := map[string]any{
				"valid":    false,
				"error":    err.Error(),
				"file":     *configPath,
				"taxonomy": "CONFIG_LOAD_FAILURE",
			}
			b, _ := json.Marshal(out)
			fmt.Println(string(b))
		}
		return err
	}

	issues := validateConfigIssues(cfg)
	if len(issues) > 0 {
		if *machine {
			out := map[string]any{
				"valid":    false,
				"file":     *configPath,
				"issues":   issues,
				"taxonomy": "VALIDATION_FAILURE",
			}
			b, _ := json.Marshal(out)
			fmt.Println(string(b))
			return nil
		}
		fmt.Printf("config:       %s\n", *configPath)
		fmt.Printf("service:      %s\n", cfg.Service.Name)
		fmt.Printf("mode:         %s\n", cfg.Service.Mode)
		fmt.Printf("health:       %v (%s)\n", cfg.Health.Enabled, cfg.Health.ListenAddr)
		fmt.Printf("targets:      %d enabled / %d total\n", len(cfg.EnabledTargets()), len(cfg.Targets))
		fmt.Printf("aggressive:   %v\n", cfg.Alerts.Aggressive)
		fmt.Printf("known_state:  %v\n", cfg.KnownState.Enabled)
		fmt.Printf("remote_agent: %v\n", cfg.RemoteAgent.ManagedUpdates)
		fmt.Println("status:       invalid")
		fmt.Println("\nValidation issues:")
		for _, issue := range issues {
			fmt.Printf("  - [%s] %s\n", issue.Code, issue.Message)
		}
		return nil
	}

	if *machine {
		out := map[string]any{
			"valid":         true,
			"file":          *configPath,
			"service":       cfg.Service.Name,
			"mode":          cfg.Service.Mode,
			"enabled_count": len(cfg.EnabledTargets()),
			"total_count":   len(cfg.Targets),
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
		return nil
	}

	fmt.Printf("config:       %s\n", *configPath)
	fmt.Printf("service:      %s\n", cfg.Service.Name)
	fmt.Printf("mode:         %s\n", cfg.Service.Mode)
	fmt.Printf("health:       %v (%s)\n", cfg.Health.Enabled, cfg.Health.ListenAddr)
	fmt.Printf("targets:      %d enabled / %d total\n", len(cfg.EnabledTargets()), len(cfg.Targets))
	fmt.Printf("aggressive:   %v\n", cfg.Alerts.Aggressive)
	fmt.Printf("known_state:  %v\n", cfg.KnownState.Enabled)
	fmt.Printf("remote_agent: %v\n", cfg.RemoteAgent.ManagedUpdates)
	fmt.Println("status:       valid")
	return nil
}

type ValidationIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func validateConfigIssues(cfg config.Config) []ValidationIssue {
	var issues []ValidationIssue

	for i, tgt := range cfg.Targets {
		if !tgt.Enabled {
			continue
		}
		if tgt.Endpoint == "" && tgt.Stdio.Command == "" {
			issues = append(issues, ValidationIssue{
				Code:    "TARGET_MISSING_ENDPOINT",
				Message: fmt.Sprintf("target %q has neither endpoint nor command", tgt.ID),
				Field:   fmt.Sprintf("targets[%d].endpoint/stdio.command", i),
			})
		}
		if tgt.Auth.Type == "oauth" {
			if tgt.Auth.ClientID == "" {
				issues = append(issues, ValidationIssue{
					Code:    "OAUTH_MISSING_CLIENT_ID",
					Message: fmt.Sprintf("target %q oauth missing client_id", tgt.ID),
					Field:   fmt.Sprintf("targets[%d].auth.client_id", i),
				})
			}
			if tgt.Auth.TokenURL == "" {
				issues = append(issues, ValidationIssue{
					Code:    "OAUTH_MISSING_TOKEN_URL",
					Message: fmt.Sprintf("target %q oauth missing token_url", tgt.ID),
					Field:   fmt.Sprintf("targets[%d].auth.token_url", i),
				})
			}
		}
	}

	if cfg.Health.Enabled && cfg.Health.ListenAddr == "" {
		issues = append(issues, ValidationIssue{
			Code:    "HEALTH_MISSING_ADDR",
			Message: "health enabled but listen_addr not set",
			Field:   "health.listen_addr",
		})
	}

	return issues
}

func cmdGenMeta(args []string) error {
	fs := flag.NewFlagSet("gen-meta", flag.ContinueOnError)
	configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
	outDir := fs.String("out-dir", "", "Optional output directory override")
	formats := fs.String("formats", "", "Comma-separated formats: yaml,json")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	if *outDir != "" {
		cfg.MetaConfig.OutputDir = *outDir
	}
	if strings.TrimSpace(*formats) != "" {
		cfg.MetaConfig.Formats = splitCSV(*formats)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ds := discovery.New()
	res := ds.Discover(ctx, cfg)

	gen := meta.NewGenerator(cfg.MetaConfig)
	doc := gen.GenerateFromDiscovery(res.Records)
	if err := meta.ValidateDocument(doc); err != nil {
		return fmt.Errorf("generated meta config invalid: %w", err)
	}

	paths, err := gen.Write(ctx, doc)
	if err != nil {
		return err
	}

	fmt.Printf("generated %d meta config file(s):\n", len(paths))
	for _, p := range paths {
		fmt.Printf("- %s\n", p)
	}
	return nil
}

func cmdOps(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("ops requires subcommand: stop|status|reload|self-check")
	}

	switch args[0] {
	case "stop":
		return cmdOpsStop(args[1:])
	case "status":
		return cmdOpsStatus(args[1:])
	case "reload":
		return cmdOpsReload(args[1:])
	case "self-check":
		return cmdOpsSelfCheck(args[1:])
	default:
		return fmt.Errorf("unknown ops subcommand %q", args[0])
	}
}

func cmdOpsStop(args []string) error {
	fs := flag.NewFlagSet("ops stop", flag.ContinueOnError)
	pidFile := fs.String("pid-file", "./state/"+appName+".pid", "PID file path")
	wait := fs.Duration("wait", 15*time.Second, "Graceful wait timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pid, err := readPID(*pidFile)
	if err != nil {
		return err
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal SIGTERM pid=%d: %w", pid, err)
	}

	deadline := time.Now().Add(*wait)
	for time.Now().Before(deadline) {
		if !isProcessRunning(pid) {
			_ = os.Remove(*pidFile)
			fmt.Printf("stopped pid=%d\n", pid)
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	// Escalate to SIGKILL.
	_ = proc.Signal(syscall.SIGKILL)
	time.Sleep(300 * time.Millisecond)
	if isProcessRunning(pid) {
		return fmt.Errorf("failed to stop pid=%d", pid)
	}
	_ = os.Remove(*pidFile)
	fmt.Printf("stopped pid=%d (forced)\n", pid)
	return nil
}

func cmdOpsStatus(args []string) error {
	fs := flag.NewFlagSet("ops status", flag.ContinueOnError)
	pidFile := fs.String("pid-file", "./state/"+appName+".pid", "PID file path")
	healthURL := fs.String("health-url", "http://127.0.0.1:8088/healthz", "Health URL")
	if err := fs.Parse(args); err != nil {
		return err
	}

	pid, err := readPID(*pidFile)
	if err != nil {
		fmt.Printf("runtime: stopped (%v)\n", err)
		return nil
	}

	running := isProcessRunning(pid)
	fmt.Printf("pid:     %d\n", pid)
	fmt.Printf("running: %v\n", running)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(*healthURL)
	if err != nil {
		fmt.Printf("health:  unreachable (%v)\n", err)
		return nil
	}
	defer resp.Body.Close()
	fmt.Printf("health:  %s (%d)\n", *healthURL, resp.StatusCode)
	return nil
}

func cmdOpsReload(args []string) error {
	fs := flag.NewFlagSet("ops reload", flag.ContinueOnError)
	configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if !cfg.RemoteAgent.ManagedUpdates {
		return fmt.Errorf("remote_agent.managed_updates is false")
	}

	steps := []struct {
		name string
		cmd  string
	}{
		{"stop", cfg.RemoteAgent.Update.StopCommand},
		{"start", cfg.RemoteAgent.Update.StartCommand},
		{"verify", cfg.RemoteAgent.Update.VerifyCommand},
	}

	for _, step := range steps {
		if strings.TrimSpace(step.cmd) == "" {
			continue
		}
		fmt.Printf("[%s] %s\n", step.name, step.cmd)
		if err := runShell(step.cmd); err != nil {
			return fmt.Errorf("%s failed: %w", step.name, err)
		}
	}
	fmt.Println("reload workflow complete")
	return nil
}

func cmdOpsSelfCheck(args []string) error {
	fs := flag.NewFlagSet("ops self-check", flag.ContinueOnError)
	configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
	jsonOut := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := runOneShotChecks(ctx, cfg)
	if err != nil {
		return err
	}

	failures := 0
	regressions := 0
	for _, r := range results {
		if r.IsFailure() {
			failures++
		}
		if r.State == targets.StateRegression || r.Regression {
			regressions++
		}
	}

	if *jsonOut {
		return writeJSON(os.Stdout, map[string]any{
			"service":     cfg.Service.Name,
			"time":        time.Now().UTC(),
			"targets":     len(results),
			"failures":    failures,
			"regressions": regressions,
			"results":     results,
		})
	}

	fmt.Printf("targets:     %d\n", len(results))
	fmt.Printf("failures:    %d\n", failures)
	fmt.Printf("regressions: %d\n", regressions)
	if failures > 0 {
		return fmt.Errorf("self-check failed")
	}
	fmt.Println("self-check passed")
	return nil
}

func runOneShotChecks(ctx context.Context, cfg config.Config) ([]targets.CheckResult, error) {
	prober := engine.NewStdioProber()
	authMgr := auth.NewManager()
	safetyScanner := safety.NewScanner()

	enabled := cfg.EnabledTargets()
	out := make([]targets.CheckResult, 0, len(enabled))

	for _, t := range enabled {
		compiled, err := toTarget(t, cfg)
		if err != nil {
			out = append(out, mapOneShotConfigFailure(t, err))
			continue
		}

		if len(compiled.Check.HURLTests) > 0 {
			_ = safetyScanner.RegisterTarget(compiled)
			safetyReport := safetyScanner.RunTarget(ctx, compiled)
			if safetyReport.HasBlocking {
				out = append(out, mapOneShotSafetyFailure(compiled, safetyReport))
				continue
			}
		}

		cctx, cancel := context.WithTimeout(ctx, compiled.Check.Timeout)
		mat, authErr := authMgr.BuildHeaders(cctx, compiled.Auth)
		cancel()
		if authErr != nil {
			out = append(out, mapOneShotAuthFailure(compiled, authErr))
			continue
		}

		cctx2, cancel2 := context.WithTimeout(ctx, compiled.Check.Timeout)
		res, probeErr := prober.Probe(cctx2, compiled, mat.Headers)
		cancel2()
		if probeErr != nil {
			out = append(out, mapOneShotProbeFailure(compiled, probeErr))
			continue
		}
		out = append(out, res)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].TargetID < out[j].TargetID })
	return out, nil
}

func mapOneShotConfigFailure(t config.TargetConfig, err error) targets.CheckResult {
	return targets.CheckResult{
		TargetID:     t.ID,
		Protocol:     targets.Protocol(strings.ToLower(t.Protocol)),
		State:        targets.StateUnhealthy,
		Severity:     targets.SeverityWarn,
		FailureClass: targets.FailureConfig,
		Message:      "invalid target configuration: " + err.Error(),
		CheckedAt:    time.Now().UTC(),
	}
}

func mapOneShotAuthFailure(t targets.Target, err error) targets.CheckResult {
	msg := strings.ToLower(err.Error())

	state := targets.StateUnhealthy
	severity := targets.SeverityWarn
	failureClass := targets.FailureAuth

	switch {
	case strings.Contains(msg, "secret provider unsupported"),
		strings.Contains(msg, "secret not found"),
		strings.Contains(msg, "invalid secret reference"),
		strings.Contains(msg, "client secret"):
		// Local setup/auth material issue: actionable but not necessarily target outage.
		state = targets.StateDegraded
		severity = targets.SeverityWarn
		failureClass = targets.FailureConfig
	case strings.Contains(msg, "oauth"):
		state = targets.StateUnhealthy
		severity = targets.SeverityWarn
		failureClass = targets.FailureAuth
	}

	return targets.CheckResult{
		TargetID:     t.ID,
		Protocol:     t.Protocol,
		State:        state,
		Severity:     severity,
		FailureClass: failureClass,
		Message:      "auth validation failed: " + err.Error(),
		CheckedAt:    time.Now().UTC(),
	}
}

func mapOneShotProbeFailure(t targets.Target, err error) targets.CheckResult {
	msg := strings.ToLower(err.Error())

	state := targets.StateUnhealthy
	severity := targets.SeverityWarn
	failureClass := targets.FailureUnknown

	switch {
	case errors.Is(err, context.DeadlineExceeded),
		strings.Contains(msg, "timeout"),
		strings.Contains(msg, "i/o timeout"):
		state = targets.StateDegraded
		severity = targets.SeverityWarn
		failureClass = targets.FailureTimeout
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "network is unreachable"),
		strings.Contains(msg, "dial tcp"):
		state = targets.StateOutage
		severity = targets.SeverityCritical
		failureClass = targets.FailureNetwork
	case strings.Contains(msg, "tls"),
		strings.Contains(msg, "x509"):
		state = targets.StateUnhealthy
		severity = targets.SeverityCritical
		failureClass = targets.FailureTLS
	default:
		state = targets.StateUnhealthy
		severity = targets.SeverityWarn
		failureClass = targets.FailureUnknown
	}

	return targets.CheckResult{
		TargetID:     t.ID,
		Protocol:     t.Protocol,
		State:        state,
		Severity:     severity,
		FailureClass: failureClass,
		Message:      "probe failed: " + err.Error(),
		CheckedAt:    time.Now().UTC(),
	}
}

func mapOneShotSafetyFailure(t targets.Target, report safety.Report) targets.CheckResult {
	message := "pre-protocol HURL safety checks failed"
	failureClass := targets.FailureProtocol

	for _, r := range report.Results {
		if r.Outcome == safety.OutcomeFail {
			if strings.TrimSpace(r.Message) != "" {
				message = r.Message
			}
			if r.FailureClass != "" && r.FailureClass != targets.FailureNone {
				failureClass = r.FailureClass
			}
			break
		}
	}

	return targets.CheckResult{
		TargetID:     t.ID,
		Protocol:     t.Protocol,
		State:        targets.StateUnhealthy,
		Severity:     targets.SeverityWarn,
		FailureClass: failureClass,
		Message:      message,
		CheckedAt:    time.Now().UTC(),
		Details: map[string]any{
			"stage":   safety.Stage,
			"passed":  report.Passed,
			"failed":  report.Failed,
			"skipped": report.Skipped,
		},
	}
}

func printCheckTable(results []targets.CheckResult) {
	fmt.Println("TARGET                  STATE       SEV       STATUS  LAT(ms)  MESSAGE")
	fmt.Println("-----------------------------------------------------------------------")
	for _, r := range results {
		fmt.Printf("%-22s  %-10s %-8s %-6d  %-7d  %s\n",
			truncate(r.TargetID, 22),
			string(r.State),
			string(r.Severity),
			r.StatusCode,
			r.Latency.Milliseconds(),
			truncate(r.Message, 60),
		)
	}
}

func toTarget(t config.TargetConfig, cfg config.Config) (targets.Target, error) {
	interval := mustDuration(t.Check.Interval, mustDuration(cfg.Service.PollInterval, 15*time.Second))
	timeout := mustDuration(t.Check.Timeout, mustDuration(cfg.Service.Timeout, 5*time.Second))

	target := targets.Target{
		ID:        t.ID,
		Enabled:   t.Enabled,
		Protocol:  targets.Protocol(strings.ToLower(t.Protocol)),
		Name:      t.Name,
		Endpoint:  t.Endpoint,
		Transport: targets.Transport(strings.ToLower(t.Transport)),
		Expected: targets.ExpectedBehavior{
			HealthyStatusCodes: append([]int(nil), t.Expected.HealthyStatusCodes...),
			MinCapabilities:    append([]string(nil), t.Expected.MinCapabilities...),
			KnownAgentCountMin: t.Expected.KnownAgentCountMin,
			ExpectedVersion:    t.Expected.ExpectedVersion,
		},
		Auth: targets.AuthConfig{
			Type:        targets.AuthType(strings.ToLower(t.Auth.Type)),
			Header:      t.Auth.Header,
			KeyName:     t.Auth.KeyName,
			SecretRef:   t.Auth.SecretRef,
			ClientID:    t.Auth.ClientID,
			TokenURL:    t.Auth.TokenURL,
			RedirectURL: t.Auth.RedirectURL,
			CallbackID:  t.Auth.CallbackID,
			Scopes:      append([]string(nil), t.Auth.Scopes...),
		},
		Stdio: targets.StdioCommand{
			Command: t.Stdio.Command,
			Args:    append([]string(nil), t.Stdio.Args...),
			Env:     t.Stdio.Env,
			Cwd:     t.Stdio.Cwd,
		},
		Check: targets.CheckPolicy{
			Interval:         interval,
			Timeout:          timeout,
			Retries:          t.Check.Retries,
			HandshakeProfile: t.Check.HandshakeProfile,
			RequiredMethods:  append([]string(nil), t.Check.RequiredMethods...),
			HURLTests:        mapOneShotHURLTests(t.Check.HURLTests),
		},
	}
	if err := target.Validate(); err != nil {
		return targets.Target{}, err
	}
	return target, nil
}

func mapOneShotHURLTests(in []config.HURLTestConfig) []targets.HURLTest {
	if len(in) == 0 {
		return nil
	}

	out := make([]targets.HURLTest, 0, len(in))
	for _, ht := range in {
		out = append(out, targets.HURLTest{
			Name:     ht.Name,
			File:     ht.File,
			Endpoint: ht.Endpoint,
			Method:   ht.Method,
			Headers:  ht.Headers,
			Body:     ht.Body,
		})
	}
	return out
}

func buildLogger(level string, enableJSON bool) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}

	if !enableJSON {
		return nil
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})
	return slog.New(handler)
}

func buildNotifiers(cfg config.Config, logger *slog.Logger, enableJSON bool) engine.Notifier {
	var sinks []engine.Notifier
	dedupe := mustDuration(cfg.Alerts.DedupeWindow, 2*time.Minute)

	if cfg.Alerts.Sinks.Log.Enabled {
		if logger != nil {
			sinks = append(sinks, alerts.NewLogNotifier(logger, "info", dedupe))
		}
	}
	if cfg.Alerts.Sinks.OSNotification.Enabled {
		sinks = append(sinks, alerts.NewDesktopNotifier(
			cfg.Alerts.Sinks.OSNotification.TitlePrefix,
			"warn",
			dedupe,
		))
	}
	if len(sinks) == 0 {
		return nil
	}
	return alerts.NewNotifierGroup(sinks...)
}

func acquireLock(path string) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("lock path is required")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	create := func() (*os.File, error) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
		return f, nil
	}

	f, err := create()
	if err == nil {
		return f, nil
	}

	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	// Lock exists. Attempt stale lock recovery:
	// - if pid is unreadable, keep lock in place (safe default)
	// - if pid is running, keep lock in place
	// - if pid is stale/not running, remove and retry once
	pid, readErr := readPID(path)
	if readErr != nil || pid <= 0 {
		return nil, fmt.Errorf("lock exists and cannot validate owner: %s", path)
	}
	if isProcessRunning(pid) {
		return nil, fmt.Errorf("lock exists: %s (pid=%d running)", path, pid)
	}

	if rmErr := os.Remove(path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to remove stale lock %s: %w", path, rmErr)
	}

	f, err = create()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock after stale cleanup: %w", err)
	}
	return f, nil
}

func releaseLock(f *os.File, path string) {
	if f != nil {
		_ = f.Close()
	}
	_ = os.Remove(path)
}

func writePIDFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
}

func readPID(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	v := strings.TrimSpace(string(b))
	pid, err := strconv.Atoi(v)
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("invalid pid in %s", path)
	}
	return pid, nil
}

func isProcessRunning(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}

func runShell(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func writeJSON(dst *os.File, v any) error {
	enc := json.NewEncoder(dst)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func hasStringFold(items []string, needle string) bool {
	needle = strings.TrimSpace(strings.ToLower(needle))
	if needle == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), needle) {
			return true
		}
	}
	return false
}

func ensureStringFoldContains(items []string, value string) []string {
	if hasStringFold(items, value) {
		return items
	}
	return append(items, value)
}

func mustDuration(raw string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func isTUIEnabledFromMode(mode string, tuiFlag *bool) bool {
	if mode == config.ModeHeadless {
		return false
	}
	if mode == config.ModeForeground && tuiFlag != nil && !*tuiFlag {
		return false
	}
	return true
}

func cmdTuner(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("tuner requires subcommand: discover|status|audience")
	}

	switch args[0] {
	case "discover":
		fmt.Println("discovering tuner services via mDNS... (not yet implemented)")
		fmt.Println("hint: tuner instances advertise _tuner._tcp")
		return nil
	case "status":
		fs := flag.NewFlagSet("tuner status", flag.ContinueOnError)
		configPath := fs.String("config", "configs/sample.yaml", "Path to config file")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}
		fmt.Printf("tuner integration: enabled=%v advertise=%v service_type=%s\n",
			cfg.Tuner.Enabled, cfg.Tuner.Advertise, cfg.Tuner.ServiceType)
		fmt.Printf("audience endpoint:  enabled=%v path=%s\n",
			cfg.Tuner.Audience.Enabled, cfg.Tuner.Audience.Endpoint)
		fmt.Printf("caller hook:        enabled=%v mcp_response=%v\n",
			cfg.Tuner.CallerHook.Enabled, cfg.Tuner.CallerHook.MCPResponse)
		return nil
	case "audience":
		fs := flag.NewFlagSet("tuner audience", flag.ContinueOnError)
		addr := fs.String("addr", "127.0.0.1:18088", "Hosted server address")
		channel := fs.String("channel", "", "Channel name")
		count := fs.Int("count", 1, "Audience count")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *channel == "" {
			return fmt.Errorf("--channel required")
		}
		body := fmt.Sprintf(`{"channel":%q,"count":%d,"signal":1.0}`, *channel, *count)
		resp, err := http.Post(
			fmt.Sprintf("http://%s/tuner/audience", *addr),
			"application/json",
			strings.NewReader(body),
		)
		if err != nil {
			return fmt.Errorf("post audience: %w", err)
		}
		defer resp.Body.Close()
		fmt.Printf("audience push: %s\n", resp.Status)
		return nil
	default:
		return fmt.Errorf("unknown tuner subcommand %q (use: discover|status|audience)", args[0])
	}
}

func cmdTUI(args []string) error {
	fs := flag.NewFlagSet("tui", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8088", "Health server address")
	refresh := fs.String("refresh", "5s", "Refresh interval (default 5s for background use)")
	jsonOutput := fs.Bool("json", false, "Output JSON logs to stdout")
	verbose := fs.Bool("v", false, "Show verbose details (components, events, debug)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	refreshInterval, err := time.ParseDuration(*refresh)
	if err != nil {
		return fmt.Errorf("invalid refresh interval: %w", err)
	}

	if !*jsonOutput {
		fmt.Fprintf(os.Stderr, "Connecting to %s, press q to quit... (v for verbose)\n", *addr)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	var statuses []health.TargetStatus
	var components []health.ComponentStatus

	for {
		select {
		case <-ticker.C:
			resp, err := client.Get(fmt.Sprintf("http://%s/status", *addr))
			if err != nil {
				if *jsonOutput {
					fmt.Fprintf(os.Stdout, `{"level":"error","msg":"failed to fetch status","error":"%v"}%s`, err, "\n")
				}
				continue
			}
			var statusResp health.StatusResponse
			if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			statuses = statusResp.Targets
			components = statusResp.Components

			if !*jsonOutput {
				fmt.Printf("\033[2J\033[H")
				fmt.Printf("OCD Smoke Alarm — Remote TUI | %s | uptime: %ds\n\n", *addr, statusResp.UptimeSec)
				if *verbose {
					printRemoteComponents(components)
					fmt.Println()
				}
				printRemoteStatusTable(statuses)
			}
		case <-getChar():
			return nil
		}
	}
}

func printRemoteComponents(components []health.ComponentStatus) {
	fmt.Printf("COMPONENTS\n")
	fmt.Printf("----------\n")
	for _, c := range components {
		status := "✓"
		if !c.Healthy {
			status = "✗"
		}
		fmt.Printf("  %s %-20s %s\n", status, c.Name, c.Detail)
	}
}

func getChar() chan bool {
	ch := make(chan bool)
	go func() {
		buf := make([]byte, 1)
		os.Stdin.Read(buf)
		ch <- true
	}()
	return ch
}

func printRemoteStatusTable(targets []health.TargetStatus) {
	fmt.Printf("%-20s %-12s %-10s %s\n", "TARGET", "STATE", "SEVERITY", "MESSAGE")
	fmt.Println(strings.Repeat("-", 80))
	for _, t := range targets {
		stateColor := ""
		switch t.State {
		case "healthy":
			stateColor = "\033[32m"
		case "degraded":
			stateColor = "\033[33m"
		case "unhealthy", "outage":
			stateColor = "\033[31m"
		case "regression":
			stateColor = "\033[35m"
		default:
			stateColor = "\033[90m"
		}
		reset := "\033[0m"
		fmt.Printf("%-20s %s%-12s%s %-10s %s\n",
			truncate(t.ID, 20),
			stateColor, t.State, reset,
			t.Severity,
			truncate(t.Message, 40),
		)
	}
}

func parseFederationFlag(flag string, cfg *config.Config) {
	parts := strings.Split(flag, ",")
	fmt.Printf("[MAIN] federation flag parse: parts=%v\n", parts)
	for _, part := range parts {
		kv := strings.Split(part, ":")
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "upstream":
			cfg.Federation.Upstream = kv[1]
		case "downstream":
			cfg.Federation.Downstream = strings.Split(kv[1], ",")
		}
	}
	cfg.Federation.Enabled = true
	cfg.Federation.Rank = federation.DetectRank(cfg.Health.ListenAddr)
	cfg.Federation.PollInterval = "30s"
	fmt.Printf("[MAIN] federation enabled: downstream=%v, rank=%d\n", cfg.Federation.Downstream, cfg.Federation.Rank)
}
