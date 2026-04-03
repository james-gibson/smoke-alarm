package stepdefinitions

// selfdescription_steps.go — step definitions for features/selfdescription.feature
//
// Two paths to the document:
//   1. theSelfDescriptionDocumentForConfig — builds in-process via
//      health.NewSelfDescriptionFactory; no HTTP server required.
//   2. aGETRequestSentToSelfDescription — HTTP GET to the running health server
//      (hsState.baseURL must be set by a Background or ocdSmokeAlarmIsRunningWithConfig).
//
// Assertion helpers read sdState.doc when available; fall back to parsing
// httpState.lastBody when aGETRequestSentToSelfDescription was used.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/health"
)

// sdState holds the self-description document for the current scenario.
var sdState struct {
	doc        *health.SelfDescription
	schemaPath string
	validErr   error
}

func InitializeSelfDescriptionSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		sdState.doc = nil
		sdState.schemaPath = ""
		sdState.validErr = nil
	})

	// ── endpoint availability ──────────────────────────────────────────────
	ctx.Step(`^health\.enabled is true in the config$`, healthEnabledIsTrueInConfig)
	ctx.Step(`^the Content-Type is "([^"]*)"$`, theContentTypeIs)

	// ── document retrieval ─────────────────────────────────────────────────
	ctx.Step(`^the SelfDescription document for config "([^"]*)"$`, theSelfDescriptionDocumentForConfig)
	ctx.Step(`^a GET request is sent to the self-description endpoint$`, aGETRequestSentToSelfDescription)

	// ── required fields ────────────────────────────────────────────────────
	ctx.Step(`^the document health\.endpoints\.self_description field is "([^"]*)"$`, theDocumentHealthEndpointsSelfDescription)
	ctx.Step(`^the document contains field "([^"]*)"$`, theDocumentContainsField)

	// ── service block ──────────────────────────────────────────────────────
	ctx.Step(`^the document service\.name is "([^"]*)"$`, theDocumentServiceNameIs)
	ctx.Step(`^the document service\.mode is "([^"]*)"$`, theDocumentServiceModeIs)
	ctx.Step(`^the document service\.environment is "([^"]*)"$`, theDocumentServiceEnvironmentIs)
	ctx.Step(`^the document service\.started_at is a valid RFC3339 timestamp$`, theDocumentServiceStartedAtIsRFC3339)

	// ── capabilities ──────────────────────────────────────────────────────
	ctx.Step(`^the document capabilities\.monitoring\.protocols contains "([^"]*)"$`, theDocumentCapabilitiesProtocolsContains)
	ctx.Step(`^the document capabilities\.discovery\.llms_txt is true$`, theDocumentCapabilitiesLlmsTxtIsTrue)
	ctx.Step(`^the document capabilities\.discovery\.llms_txt is false$`, theDocumentCapabilitiesLlmsTxtIsFalse)
	ctx.Step(`^the document capabilities\.hosted\.enabled is true$`, theDocumentCapabilitiesHostedEnabledIsTrue)
	ctx.Step(`^the document capabilities\.dynamic_config\.enabled is true$`, theDocumentCapabilitiesDynamicConfigEnabledIsTrue)
	ctx.Step(`^the document capabilities\.meta_config\.enabled is true$`, theDocumentCapabilitiesMetaConfigEnabledIsTrue)

	// ── targets list ───────────────────────────────────────────────────────
	ctx.Step(`^the document targets list contains an entry with id "([^"]*)"$`, theDocumentTargetsListContains)
	ctx.Step(`^the document targets list does not contain an entry with id "([^"]*)"$`, theDocumentTargetsListDoesNotContain)

	// ── health block ───────────────────────────────────────────────────────
	ctx.Step(`^the document health\.endpoints\.healthz is "([^"]*)"$`, theDocumentHealthEndpointsHealthz)
	ctx.Step(`^the document health\.endpoints\.readyz is "([^"]*)"$`, theDocumentHealthEndpointsReadyz)
	ctx.Step(`^the document health\.endpoints\.status is "([^"]*)"$`, theDocumentHealthEndpointsStatus)
	ctx.Step(`^the document health\.listen_addr is "([^"]*)"$`, theDocumentHealthListenAddr)

	// ── schema validation ──────────────────────────────────────────────────
	ctx.Step(`^the schema at "([^"]*)"$`, theSchemaAt)
	ctx.Step(`^the document is validated against the schema$`, theDocumentIsValidatedAgainstSchema)
	ctx.Step(`^validation passes with no errors$`, validationPassesWithNoErrors)
}

// ── document access helpers ────────────────────────────────────────────────

// getSelfDescDoc returns the self-description document from either the in-process
// build (sdState.doc) or the last HTTP response body.
func getSelfDescDoc() (*health.SelfDescription, error) {
	if sdState.doc != nil {
		return sdState.doc, nil
	}
	if httpState.lastBody != nil {
		var doc health.SelfDescription
		if err := json.Unmarshal(httpState.lastBody, &doc); err != nil {
			return nil, fmt.Errorf("parse self-description from HTTP body: %w", err)
		}
		return &doc, nil
	}
	return nil, fmt.Errorf("no self-description document available; call 'the SelfDescription document for config' or 'a GET request is sent to the self-description endpoint' first")
}

// ── endpoint availability ──────────────────────────────────────────────────

func healthEnabledIsTrueInConfig() error {
	// Satisfied by the Background step "the health server is started on an available port"
	// or "ocd-smoke-alarm is running with config". No-op assertion.
	return nil
}

func theContentTypeIs(ct string) error {
	if httpState.lastResp == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	got := httpState.lastResp.Header.Get("Content-Type")
	if !strings.Contains(got, ct) {
		return fmt.Errorf("expected Content-Type to contain %q, got %q", ct, got)
	}
	return nil
}

// ── document retrieval ─────────────────────────────────────────────────────

// theSelfDescriptionDocumentForConfig builds the self-description document
// in-process from the given config file (no HTTP server required).
func theSelfDescriptionDocumentForConfig(cfgPath string) error {
	abs := resolveConfigPath(cfgPath)
	cfg, err := config.Load(abs)
	if err != nil {
		return fmt.Errorf("load config %q: %w", cfgPath, err)
	}
	fn := health.NewSelfDescriptionFactory(cfg, "test", time.Now().UTC(), nil)
	raw := fn()
	doc, ok := raw.(health.SelfDescription)
	if !ok {
		return fmt.Errorf("unexpected self-description type %T", raw)
	}
	sdState.doc = &doc
	return nil
}

// aGETRequestSentToSelfDescription performs an HTTP GET to the running health
// server's /.well-known/smoke-alarm.json endpoint and parses the response.
func aGETRequestSentToSelfDescription() error {
	if err := aGETRequestSentTo("/.well-known/smoke-alarm.json"); err != nil {
		return err
	}
	// Parse body into sdState.doc for non-HTTP assertions.
	var doc health.SelfDescription
	if err := json.Unmarshal(httpState.lastBody, &doc); err != nil {
		return fmt.Errorf("parse self-description response: %w\nbody: %s", err, httpState.lastBody)
	}
	sdState.doc = &doc
	return nil
}

// ── required fields ────────────────────────────────────────────────────────

func theDocumentHealthEndpointsSelfDescription(path string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	got := doc.Health.Endpoints.SelfDescription
	if got != path {
		return fmt.Errorf("health.endpoints.self_description: expected %q, got %q", path, got)
	}
	return nil
}

func theDocumentContainsField(field string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	// Serialize to map and check key presence.
	raw, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal document: %w", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("unmarshal document to map: %w", err)
	}
	if _, ok := m[field]; !ok {
		return fmt.Errorf("document does not contain field %q; keys present: %v", field, mapKeys(m))
	}
	return nil
}

// ── service block ──────────────────────────────────────────────────────────

func theDocumentServiceNameIs(name string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Service.Name != name {
		return fmt.Errorf("service.name: expected %q, got %q", name, doc.Service.Name)
	}
	return nil
}

func theDocumentServiceModeIs(mode string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Service.Mode != mode {
		return fmt.Errorf("service.mode: expected %q, got %q", mode, doc.Service.Mode)
	}
	return nil
}

func theDocumentServiceEnvironmentIs(env string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Service.Environment != env {
		return fmt.Errorf("service.environment: expected %q, got %q", env, doc.Service.Environment)
	}
	return nil
}

func theDocumentServiceStartedAtIsRFC3339() error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Service.StartedAt == "" {
		return fmt.Errorf("service.started_at is empty")
	}
	if _, err := time.Parse(time.RFC3339, doc.Service.StartedAt); err != nil {
		return fmt.Errorf("service.started_at %q is not a valid RFC3339 timestamp: %w",
			doc.Service.StartedAt, err)
	}
	return nil
}

// ── capabilities ──────────────────────────────────────────────────────────

func theDocumentCapabilitiesProtocolsContains(protocol string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	for _, p := range doc.Capabilities.Monitoring.Protocols {
		if p == protocol {
			return nil
		}
	}
	return fmt.Errorf("capabilities.monitoring.protocols does not contain %q; got: %v",
		protocol, doc.Capabilities.Monitoring.Protocols)
}

func theDocumentCapabilitiesLlmsTxtIsTrue() error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	// llms_txt is "true" when it appears as a source or when LLMSTxt.Enabled implies it.
	// We check the discovery sources slice for "llms_txt".
	for _, src := range doc.Capabilities.Discovery.Sources {
		if src == "llms_txt" {
			return nil
		}
	}
	return fmt.Errorf("capabilities.discovery does not have llms_txt source; sources: %v",
		doc.Capabilities.Discovery.Sources)
}

func theDocumentCapabilitiesLlmsTxtIsFalse() error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	for _, src := range doc.Capabilities.Discovery.Sources {
		if src == "llms_txt" {
			return fmt.Errorf("capabilities.discovery has llms_txt source but expected false; sources: %v",
				doc.Capabilities.Discovery.Sources)
		}
	}
	return nil
}

func theDocumentCapabilitiesHostedEnabledIsTrue() error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if !doc.Capabilities.Hosted.Enabled {
		return fmt.Errorf("capabilities.hosted.enabled is false")
	}
	return nil
}

func theDocumentCapabilitiesDynamicConfigEnabledIsTrue() error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if !doc.Capabilities.DynamicConfig.Enabled {
		return fmt.Errorf("capabilities.dynamic_config.enabled is false")
	}
	return nil
}

func theDocumentCapabilitiesMetaConfigEnabledIsTrue() error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if !doc.Capabilities.MetaConfig.Enabled {
		return fmt.Errorf("capabilities.meta_config.enabled is false")
	}
	return nil
}

// ── targets list ───────────────────────────────────────────────────────────

func theDocumentTargetsListContains(id string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	for _, t := range doc.Targets {
		if t.ID == id {
			return nil
		}
	}
	ids := make([]string, len(doc.Targets))
	for i, t := range doc.Targets {
		ids[i] = t.ID
	}
	return fmt.Errorf("targets list does not contain id %q; ids present: %v", id, ids)
}

func theDocumentTargetsListDoesNotContain(id string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	for _, t := range doc.Targets {
		if t.ID == id {
			return fmt.Errorf("targets list unexpectedly contains id %q", id)
		}
	}
	return nil
}

// ── health block ───────────────────────────────────────────────────────────

func theDocumentHealthEndpointsHealthz(path string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Health.Endpoints.Healthz != path {
		return fmt.Errorf("health.endpoints.healthz: expected %q, got %q",
			path, doc.Health.Endpoints.Healthz)
	}
	return nil
}

func theDocumentHealthEndpointsReadyz(path string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Health.Endpoints.Readyz != path {
		return fmt.Errorf("health.endpoints.readyz: expected %q, got %q",
			path, doc.Health.Endpoints.Readyz)
	}
	return nil
}

func theDocumentHealthEndpointsStatus(path string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Health.Endpoints.Status != path {
		return fmt.Errorf("health.endpoints.status: expected %q, got %q",
			path, doc.Health.Endpoints.Status)
	}
	return nil
}

func theDocumentHealthListenAddr(addr string) error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	if doc.Health.ListenAddr != addr {
		return fmt.Errorf("health.listen_addr: expected %q, got %q",
			addr, doc.Health.ListenAddr)
	}
	return nil
}

// ── schema validation ──────────────────────────────────────────────────────

// theSchemaAt records the schema file path and verifies it exists.
// NOTE: No JSON Schema library is available in the project dependencies.
// Full validation uses a manual struct-level check against the required
// fields defined in the schema. See TASKS.md NEXT for the gap.
func theSchemaAt(path string) error {
	abs := resolveConfigPath(path)
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("schema file %q not found: %w", abs, err)
	}
	sdState.schemaPath = abs
	return nil
}

// theDocumentIsValidatedAgainstSchema performs a manual struct-level check
// against the required fields in the self-description schema.
func theDocumentIsValidatedAgainstSchema() error {
	doc, err := getSelfDescDoc()
	if err != nil {
		return err
	}
	sdState.validErr = validateSelfDescriptionDocument(doc)
	return nil
}

func validationPassesWithNoErrors() error {
	if sdState.validErr != nil {
		return fmt.Errorf("schema validation failed: %w", sdState.validErr)
	}
	return nil
}

// validateSelfDescriptionDocument checks required fields per
// configs/schema/self-description.schema.json without a JSON Schema library.
func validateSelfDescriptionDocument(doc *health.SelfDescription) error {
	var errs []string

	if doc.Version != "1" {
		errs = append(errs, fmt.Sprintf("version: expected \"1\", got %q", doc.Version))
	}
	if doc.Service.Name == "" {
		errs = append(errs, "service.name is required")
	}
	switch doc.Service.Mode {
	case "foreground", "background", "headless":
	case "":
		errs = append(errs, "service.mode is required")
	default:
		errs = append(errs, fmt.Sprintf("service.mode %q not in enum [foreground, background, headless]", doc.Service.Mode))
	}
	for i, t := range doc.Targets {
		if t.ID == "" {
			errs = append(errs, fmt.Sprintf("targets[%d].id is required", i))
		}
		switch t.Protocol {
		case "mcp", "acp", "http":
		default:
			errs = append(errs, fmt.Sprintf("targets[%d].protocol %q not in enum", i, t.Protocol))
		}
		switch t.Transport {
		case "http", "https", "sse", "websocket", "stdio":
		default:
			errs = append(errs, fmt.Sprintf("targets[%d].transport %q not in enum", i, t.Transport))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// ── utility ───────────────────────────────────────────────────────────────

func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
