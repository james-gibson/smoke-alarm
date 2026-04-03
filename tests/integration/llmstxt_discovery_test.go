package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestDiscovery_LLMSTxtAutoRegistersMCPTargetsAndACPTargets(t *testing.T) {
	t.Parallel()

	llmsContent := `# Example Platform

> LLM-friendly docs and endpoints.

## MCP Endpoints

- [Primary MCP API](/mcp): Main MCP endpoint
- [Secondary MCP API](https://external.example.com/mcp): Backup MCP endpoint

## ACP Endpoints

- [ACP Agent Gateway](/acp): OAuth-protected Agent Client Protocol endpoint

## Optional

- [MCP Optional](https://optional.example.com/mcp): Optional MCP endpoint
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(llmsContent))
	}))
	defer srv.Close()

	cfg := config.Config{
		Version: "1",
		Service: config.ServiceConfig{
			Name:         "llmstxt-itest",
			Mode:         config.ModeBackground,
			PollInterval: "3s",
			Timeout:      "1s",
			MaxWorkers:   2,
		},
		Discovery: config.DiscoveryConfig{
			Enabled: true,
			LLMSTxt: config.LLMSTxtDiscoveryConfig{
				Enabled:               true,
				RemoteURIs:            []string{srv.URL + "/llms.txt"},
				FetchTimeout:          "2s",
				RequireHTTPS:          false,
				AutoRegisterAsTargets: true,
				AutoRegisterOAuth:     true,
			},
			LocalProxyScan: config.LocalProxyScanConfig{
				Enabled: false,
			},
			CloudCatalog: config.CloudCatalogConfig{
				Enabled: false,
			},
		},
		Alerts: config.AlertsConfig{
			DedupeWindow: "1m",
			Cooldown:     "30s",
			Severity: config.SeverityConfig{
				Healthy:    "info",
				Degraded:   "warn",
				Regression: "critical",
				Outage:     "critical",
			},
		},
		Health: config.HealthConfig{
			Enabled: false,
		},
		Runtime: config.RuntimeConfig{
			GracefulShutdownTimeout: "2s",
			EventHistorySize:        64,
		},
		KnownState: config.KnownStateConfig{
			Enabled:                            true,
			SustainSuccessBeforeMarkHealthy:    1,
			OutageThresholdConsecutiveFailures: 2,
			ClassifyNewFailuresAfterHealthyAs:  "regression",
		},
	}

	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	disc := discovery.New()
	result := disc.Discover(ctx, cfg)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected discovery errors: %v", result.Errors)
	}

	var llmsRecords []discovery.Record
	for _, rec := range result.Records {
		if rec.Source == "llms_txt" {
			llmsRecords = append(llmsRecords, rec)
		}
	}

	if len(llmsRecords) == 0 {
		t.Fatalf("expected llms_txt discovery records, got none")
	}

	foundMCP := false
	foundACP := false
	foundOAuthScaffold := false

	for _, rec := range llmsRecords {
		if rec.Target.Protocol == targets.ProtocolMCP {
			foundMCP = true
		}
		if rec.Target.Protocol == targets.ProtocolACP {
			foundACP = true

			// With AutoRegisterOAuth enabled and OAuth hints in llms.txt, at least one ACP candidate
			// should be scaffolded with oauth auth settings.
			if rec.Target.Auth.Type == targets.AuthOAuth {
				foundOAuthScaffold = true
				if strings.TrimSpace(rec.Target.Auth.ClientID) == "" {
					t.Fatalf("expected oauth scaffold client_id for %q", rec.Target.ID)
				}
				if strings.TrimSpace(rec.Target.Auth.TokenURL) == "" {
					t.Fatalf("expected oauth scaffold token_url for %q", rec.Target.ID)
				}
				if len(rec.Target.Auth.Scopes) == 0 {
					t.Fatalf("expected oauth scaffold scopes for %q", rec.Target.ID)
				}
			}
		}

		if rec.Target.Transport != targets.TransportHTTP && rec.Target.Transport != targets.TransportWebSocket {
			t.Fatalf("unexpected transport for llms target %q: %s", rec.Target.ID, rec.Target.Transport)
		}
		if rec.Target.Auth.Type != targets.AuthNone && rec.Target.Auth.Type != targets.AuthOAuth {
			t.Fatalf("expected llms target auth type none or oauth, got %s", rec.Target.Auth.Type)
		}
		if !strings.HasPrefix(rec.Target.Endpoint, "http://") && !strings.HasPrefix(rec.Target.Endpoint, "https://") &&
			!strings.HasPrefix(rec.Target.Endpoint, "ws://") && !strings.HasPrefix(rec.Target.Endpoint, "wss://") {
			t.Fatalf("unexpected endpoint format for %q: %s", rec.Target.ID, rec.Target.Endpoint)
		}
		if rec.Confidence <= 0 {
			t.Fatalf("expected positive confidence score for %q", rec.Target.ID)
		}
	}

	if !foundMCP {
		t.Fatalf("expected at least one MCP candidate from llms.txt")
	}
	if !foundACP {
		t.Fatalf("expected at least one ACP candidate from llms.txt")
	}
	if !foundOAuthScaffold {
		t.Fatalf("expected at least one OAuth scaffolded target from llms.txt hints")
	}
}

func TestDiscovery_LLMSTxtDisabledAutoRegisterDoesNotAddTargets(t *testing.T) {
	t.Parallel()

	llmsContent := `# Example
## Endpoints
- [MCP Endpoint](/mcp): MCP
- [ACP Endpoint](/acp): ACP
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(llmsContent))
	}))
	defer srv.Close()

	cfg := config.Config{
		Version: "1",
		Service: config.ServiceConfig{
			Name:         "llmstxt-itest",
			Mode:         config.ModeBackground,
			PollInterval: "3s",
			Timeout:      "1s",
			MaxWorkers:   2,
		},
		Discovery: config.DiscoveryConfig{
			Enabled: true,
			LLMSTxt: config.LLMSTxtDiscoveryConfig{
				Enabled:               true,
				RemoteURIs:            []string{srv.URL + "/llms.txt"},
				FetchTimeout:          "2s",
				RequireHTTPS:          false,
				AutoRegisterAsTargets: false,
			},
			LocalProxyScan: config.LocalProxyScanConfig{
				Enabled: false,
			},
			CloudCatalog: config.CloudCatalogConfig{
				Enabled: false,
			},
		},
		Alerts: config.AlertsConfig{
			DedupeWindow: "1m",
			Cooldown:     "30s",
			Severity: config.SeverityConfig{
				Healthy:    "info",
				Degraded:   "warn",
				Regression: "critical",
				Outage:     "critical",
			},
		},
		Health: config.HealthConfig{
			Enabled: false,
		},
		Runtime: config.RuntimeConfig{
			GracefulShutdownTimeout: "2s",
			EventHistorySize:        64,
		},
		KnownState: config.KnownStateConfig{
			Enabled:                            true,
			SustainSuccessBeforeMarkHealthy:    1,
			OutageThresholdConsecutiveFailures: 2,
			ClassifyNewFailuresAfterHealthyAs:  "regression",
		},
	}

	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := discovery.New().Discover(ctx, cfg)

	for _, rec := range result.Records {
		if rec.Source == "llms_txt" {
			t.Fatalf("did not expect llms_txt records when auto_register_as_targets=false")
		}
	}
}
