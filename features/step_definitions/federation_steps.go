package stepdefinitions

// federation_steps.go — step definitions for features/federation.feature
// see: common_steps.go for shared steps (binary installed, HTTP status/body)
//
// These steps drive the live internal/federation types — ClaimSlot, Registry,
// Server and Client — rather than a mock, so a green run means the shipped
// implementation behaves as the feature says. They are re-run in isolation by
// TestFederationFeaturesStrict (federation_strict_test.go) with godog Strict
// enabled, so a pending or undefined step in this file fails the build.
//
// Scenario state is per-scenario: BeforeScenario allocates a temp state_dir and
// AfterScenario closes every listener, server and client. The slot-election and
// server scenarios claim real ports (5100-5103) because the feature names them;
// fedPortFree checks the range before a claim so a busy port reports as a
// failed precondition rather than a mystery follower role.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/federation"
)

const (
	// fedBasePort is the port the feature names as base_port. The slot
	// scenarios claim it for real.
	fedBasePort = 5100
	fedMaxPort  = 5103

	fedWaitDeadline = 3 * time.Second
	fedWaitInterval = 10 * time.Millisecond
)

// fedClaimResult is one process's answer to "can you claim a federation slot?".
// It is produced by the re-executed helper process (see
// federation_claim_helper_test.go) so the slot-lock scenario exercises an
// inter-process lock rather than an in-process mutex.
type fedClaimResult struct {
	Port int    `json:"port"`
	Role string `json:"role"`
	ID   string `json:"id"`
	Err  string `json:"err"`
}

// fedLogWriter is a concurrency-safe capture target for the federation logger.
// A running Client writes warnings from its own goroutine while a Then step
// reads them.
type fedLogWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *fedLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *fedLogWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *fedLogWriter) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Reset()
}

// fedScenario is the per-scenario federation state.
var fedScenario struct {
	stateDir     string
	basePort     int
	maxPort      int
	claimOptions *federation.SlotOptions
	cfg          config.Config

	// slots
	claim       *federation.SlotClaim
	claims      []*federation.SlotClaim
	occupiers   []net.Listener
	portClaims  []string
	claimedPort map[int]bool
	lastErr     error
	ids         []string
	lockResults []fedClaimResult

	// membership
	registry         *federation.Registry
	server           *federation.Server
	serverCancel     context.CancelFunc
	serverDone       chan struct{}
	serverURL        string
	follower         federation.InstanceRecord
	pendingFollowers []federation.InstanceRecord
	lastSeen         time.Time
	peerBaseline     int
	eventBaseline    int
	lastRemoved      string

	// tuning
	heartbeatTimeout time.Duration
	ageOutInterval   time.Duration
	maxPeers         int

	// follower client
	client           *federation.Client
	clientCancel     context.CancelFunc
	clientDone       chan struct{}
	followerRegistry *federation.Registry
	announceInterval time.Duration
	heartbeatEvery   time.Duration
	introducedAt     time.Time
	introStartedAt   time.Time

	// stub introducer for the "unreachable / failing" client scenarios
	stubServer        *http.Server
	stubListener      net.Listener
	stubIntroductions atomic.Int64
	stubHeartbeats    atomic.Int64

	logs fedLogWriter

	mu     sync.Mutex
	events []federation.Event
}

func InitializeFederationScenario(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		fedResetScenario()
	})
	ctx.AfterScenario(func(_ *godog.Scenario, _ error) {
		fedCleanupScenario()
	})

	// background
	ctx.Step(`^federation is enabled in the config$`, federationIsEnabledInTheConfig)

	// slot election
	ctx.Step(`^no other instance is running on the federation port range$`, noOtherInstanceRunningOnPortRange)
	ctx.Step(`^an instance starts with base_port (\d+) and max_port (\d+)$`, anInstanceStartsWithPorts)
	ctx.Step(`^the instance binds port (\d+)$`, theInstanceBindsPort)
	ctx.Step(`^the instance identity role is "([^"]*)"$`, theInstanceIdentityRoleIs)
	ctx.Step(`^the identity is persisted to "([^"]*)"$`, theIdentityIsPersistedTo)
	ctx.Step(`^an introducer is already bound on port (\d+)$`, anIntroducerAlreadyBoundOnPort)
	ctx.Step(`^a second instance starts with the same base_port and max_port$`, aSecondInstanceStartsSamePorts)
	ctx.Step(`^the second instance binds port (\d+) \+ (\d+)$`, theSecondInstanceBindsPortPlusDelta)
	ctx.Step(`^the second instance identity role is "([^"]*)"$`, theSecondInstanceIdentityRoleIs)
	ctx.Step(`^all ports from base_port to max_port are already bound$`, allPortsAlreadyBound)
	ctx.Step(`^ClaimSlot is called$`, claimSlotIsCalled)
	ctx.Step(`^the slot claim fails with "([^"]*)"$`, theSlotClaimFailsWith)
	ctx.Step(`^a config with no federation\.base_port field$`, aConfigWithNoFederationBasePort)
	ctx.Step(`^ports (\d+) through (\d+) are the candidates$`, portsAreTheCandidates)

	// instance identity
	ctx.Step(`^two ClaimSlot calls with identical hostname, service_name, state_dir, and port$`, twoClaimSlotCallsIdentical)
	ctx.Step(`^both instance IDs are computed$`, bothInstanceIDsAreComputed)
	ctx.Step(`^both IDs are equal$`, bothIDsAreEqual)
	ctx.Step(`^two ClaimSlot calls with identical hostname and service_name but different ports$`, twoClaimSlotCallsDifferentPorts)
	ctx.Step(`^the two instance IDs are different$`, theTwoInstanceIDsAreDifferent)
	ctx.Step(`^a persisted identity\.json exists with port (\d+) in the candidate range$`, aPersistedIdentityExistsWithPort)
	ctx.Step(`^ClaimSlot is called and port (\d+) is free$`, claimSlotCalledAndPortIsFree)
	ctx.Step(`^the instance claims port (\d+) before trying other candidates$`, theInstanceClaimsPortFirst)

	// slot lock
	ctx.Step(`^two processes attempt ClaimSlot simultaneously on the same port range$`, twoProcessesAttemptClaimSlotSimultaneously)
	ctx.Step(`^exactly one process succeeds as introducer$`, exactlyOneProcessSucceedsAsIntroducer)
	ctx.Step(`^the other process becomes a follower or receives ErrNoFreeSlots$`, theOtherProcessBecomesFollowerOrError)
	ctx.Step(`^a slot\.lock file exists containing the PID of a process that is no longer running$`, aStaleLockFileExists)
	ctx.Step(`^the stale lock is removed$`, theStaleLockIsRemoved)
	ctx.Step(`^ClaimSlot proceeds normally$`, claimSlotProceedsNormally)

	// introducer server
	ctx.Step(`^the introducer holds a SlotClaim with listener on port (\d+)$`, theIntroducerHoldsSlotClaimOnPort)
	ctx.Step(`^the federation server starts$`, theFederationServerStarts)
	ctx.Step(`^POST /introductions is served on that port$`, postIntroductionsIsServed)
	ctx.Step(`^POST /heartbeats is served on that port$`, postHeartbeatsIsServed)
	ctx.Step(`^GET /membership is served on that port$`, getMembershipIsServed)
	ctx.Step(`^the federation server is running as introducer$`, theFederationServerIsRunningAsIntroducer)
	ctx.Step(`^a follower POSTs to /introductions with a valid InstanceRecord$`, aFollowerPostsToIntroductions)
	ctx.Step(`^the peer appears in the registry$`, thePeerAppearsInRegistry)
	ctx.Step(`^a POST is sent to /introductions with an empty "([^"]*)" field$`, aPostSentWithEmptyField)
	ctx.Step(`^a GET request is sent to the introducer's /introductions$`, aGETRequestSentToIntroducerIntroductions)
	ctx.Step(`^a GET request is sent to /membership$`, getMembershipIsServed)
	ctx.Step(`^a follower is already registered with the introducer$`, aFollowerAlreadyRegistered)
	ctx.Step(`^a follower is registered with the introducer$`, aFollowerAlreadyRegistered)
	ctx.Step(`^the follower POSTs to /heartbeats with its InstanceRecord$`, theFollowerPostsToHeartbeats)
	ctx.Step(`^the peer's last_seen_at is updated in the registry$`, thePeersLastSeenAtIsUpdated)
	ctx.Step(`^the federation server has (\d+) registered peers$`, theFederationServerHasRegisteredPeers)
	ctx.Step(`^the "([^"]*)" array length is (\d+)$`, theArrayLengthIs)

	// age-out
	ctx.Step(`^the registry heartbeat timeout is "([^"]*)"$`, theRegistryHeartbeatTimeoutIs)
	ctx.Step(`^the follower sends no heartbeat for (\d+) milliseconds$`, theFollowerSendsNoHeartbeatForMilliseconds)
	ctx.Step(`^the follower is removed from the registry$`, theFollowerIsRemovedFromRegistry)
	ctx.Step(`^the removal reason is "([^"]*)"$`, theRemovalReasonIs)
	ctx.Step(`^the removal is reflected in GET /membership$`, theRemovalIsReflectedInMembership)
	ctx.Step(`^the follower sends a heartbeat at (\d+) milliseconds$`, theFollowerSendsHeartbeatAtMilliseconds)
	ctx.Step(`^the follower remains in the registry after (\d+) milliseconds$`, theFollowerRemainsInRegistryAfterMilliseconds)

	// peer cap
	ctx.Step(`^the registry already contains (\d+) peers at the MaxPeers limit$`, registryAtMaxPeers)
	ctx.Step(`^Upsert is called with a new peer record$`, upsertCalledWithNewPeerRecord)
	ctx.Step(`^the registry peer count does not increase$`, registryPeerCountDoesNotIncrease)
	ctx.Step(`^the upsert reports no error$`, theUpsertReportsNoError)
	ctx.Step(`^Upsert is called with a record whose ID matches the registry's own identity$`, upsertCalledWithOwnID)
	ctx.Step(`^the registry peer count does not change$`, registryPeerCountDoesNotChange)
	ctx.Step(`^no event is fired$`, noEventIsFired)

	// follower client
	ctx.Step(`^a follower client is configured with an introducer URL$`, aFollowerClientConfiguredWithIntroducerURL)
	ctx.Step(`^Start is called$`, startIsCalled)
	ctx.Step(`^a POST to /introductions is sent before the first announce_interval elapses$`, postIntroductionsSentBeforeFirstInterval)
	ctx.Step(`^a follower client has not yet successfully introduced itself$`, aFollowerClientNotYetIntroduced)
	ctx.Step(`^the heartbeat_interval elapses$`, theHeartbeatIntervalElapses)
	ctx.Step(`^no POST to /heartbeats is sent$`, noPostToHeartbeatsSent)
	ctx.Step(`^the introducer's introduction response contains (\d+) peer records$`, theIntroducerResponseContainsPeerRecords)
	ctx.Step(`^the follower processes the introduction response$`, theFollowerProcessesIntroductionResponse)
	ctx.Step(`^both peers are upserted into the follower's local registry$`, bothPeersAreUpserted)
	ctx.Step(`^the follower's registry snapshot is saved to disk$`, theFollowerRegistrySnapshotSaved)
	ctx.Step(`^the introducer URL is unreachable$`, theIntroducerURLIsUnreachable)
	ctx.Step(`^the follower attempts to send an introduction$`, theFollowerAttemptsToSendIntroduction)
	ctx.Step(`^the client retries on the next announce_interval$`, theClientRetriesOnNextInterval)
	ctx.Step(`^the follower is introduced and the introducer becomes unreachable$`, theFollowerIntroducedAndIntroducerUnreachable)
	ctx.Step(`^the federation log contains "([^"]*)"$`, theFederationLogContains)

	// registry snapshot
	ctx.Step(`^SaveSnapshot is called$`, saveSnapshotIsCalled)
	ctx.Step(`^the file "([^"]*)" exists$`, theFileExists)
	ctx.Step(`^no temporary file remains after the write$`, noTemporaryFileRemains)
	ctx.Step(`^the snapshot JSON contains "([^"]*)"$`, theSnapshotJSONContains)
	ctx.Step(`^the registry is at version (\d+)$`, theRegistryIsAtVersion)
	ctx.Step(`^a peer is upserted$`, aPeerIsUpserted)
	ctx.Step(`^the registry version is (\d+) \+ (\d+)$`, theRegistryVersionIsPlus)

	// registry events
	ctx.Step(`^the registry has no peer with id "([^"]*)"$`, theRegistryHasNoPeerWithID)
	ctx.Step(`^Upsert is called with a record with id "([^"]*)"$`, upsertCalledWithID)
	ctx.Step(`^the OnChange callback receives an event with type "([^"]*)"$`, theOnChangeCallbackReceivesEvent)
	ctx.Step(`^the registry already has a peer with id "([^"]*)"$`, theRegistryAlreadyHasPeerWithID)
	ctx.Step(`^Upsert is called again with the same id$`, upsertCalledAgainSameID)
	ctx.Step(`^the registry has a peer with id "([^"]*)"$`, theRegistryHasPeerWithID)
	ctx.Step(`^Remove is called with that id$`, removeCalledWithThatID)
}

// ── scenario lifecycle ───────────────────────────────────────────────────────

func fedResetScenario() {
	fedScenario.stateDir = ""
	fedScenario.basePort = 0
	fedScenario.maxPort = 0
	fedScenario.claimOptions = nil
	fedScenario.cfg = config.Config{}
	fedScenario.claim = nil
	fedScenario.claims = nil
	fedScenario.occupiers = nil
	fedScenario.portClaims = nil
	fedScenario.claimedPort = nil
	fedScenario.lastErr = nil
	fedScenario.ids = nil
	fedScenario.lockResults = nil
	fedScenario.registry = nil
	fedScenario.server = nil
	fedScenario.serverCancel = nil
	fedScenario.serverDone = nil
	fedScenario.serverURL = ""
	fedScenario.follower = federation.InstanceRecord{}
	fedScenario.pendingFollowers = nil
	fedScenario.lastSeen = time.Time{}
	fedScenario.peerBaseline = 0
	fedScenario.eventBaseline = 0
	fedScenario.lastRemoved = ""
	fedScenario.heartbeatTimeout = 0
	fedScenario.ageOutInterval = 0
	fedScenario.maxPeers = 0
	fedScenario.client = nil
	fedScenario.clientCancel = nil
	fedScenario.clientDone = nil
	fedScenario.followerRegistry = nil
	fedScenario.announceInterval = 0
	fedScenario.heartbeatEvery = 0
	fedScenario.introducedAt = time.Time{}
	fedScenario.introStartedAt = time.Time{}
	fedScenario.stubServer = nil
	fedScenario.stubListener = nil
	fedScenario.stubIntroductions.Store(0)
	fedScenario.stubHeartbeats.Store(0)
	fedScenario.logs.Reset()
	fedScenario.mu.Lock()
	fedScenario.events = nil
	fedScenario.mu.Unlock()
}

func fedCleanupScenario() {
	fedStopClient()
	if fedScenario.stubServer != nil {
		_ = fedScenario.stubServer.Close()
		fedScenario.stubServer = nil
	}
	if fedScenario.stubListener != nil {
		_ = fedScenario.stubListener.Close()
		fedScenario.stubListener = nil
	}
	fedStopServer()
	for _, c := range fedScenario.claims {
		_ = c.Close()
	}
	for _, l := range fedScenario.occupiers {
		_ = l.Close()
	}
	fedReleasePortClaims()
	if fedScenario.stateDir != "" {
		_ = os.RemoveAll(fedScenario.stateDir)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func fedStateDir() (string, error) {
	if fedScenario.stateDir != "" {
		return fedScenario.stateDir, nil
	}
	dir, err := os.MkdirTemp("", "federation-steps-*")
	if err != nil {
		return "", fmt.Errorf("create federation state dir: %w", err)
	}
	fedScenario.stateDir = dir
	return dir, nil
}

// federationPortClaimRoot is the cross-process claim root shared with
// tests/integration/federation_test.go: one directory per port, created with
// Mkdir so two `go test` processes on the same machine cannot hand the same
// port to two federated instances (smoke-alarm#78, #80).
func federationPortClaimRoot() string {
	return filepath.Join(os.TempDir(), "smoke-alarm-federation-port-claims")
}

// federationPortClaimTTL reclaims claims left behind by a killed run. A live
// run holds its range for seconds, so only a crash reaches the TTL.
const federationPortClaimTTL = 10 * time.Minute

// fedEnsurePortClaims claims every port in [base,maxPort] for this scenario.
// Repeated calls for the same range are free, so a Given and the When that
// follows it can both ask.
func fedEnsurePortClaims(base, maxPort int) error {
	if base <= 0 || maxPort < base {
		return nil
	}
	root := federationPortClaimRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create federation port claim root: %w", err)
	}
	if fedScenario.claimedPort == nil {
		fedScenario.claimedPort = make(map[int]bool)
	}

	deadline := time.Now().Add(fedWaitDeadline)
	for port := base; port <= maxPort; port++ {
		if fedScenario.claimedPort[port] {
			continue
		}
		dir := filepath.Join(root, strconv.Itoa(port))
		for {
			mkErr := os.Mkdir(dir, 0o700)
			if mkErr == nil {
				fedScenario.claimedPort[port] = true
				fedScenario.portClaims = append(fedScenario.portClaims, dir)
				break
			}
			if !errors.Is(mkErr, os.ErrExist) {
				return fmt.Errorf("claim federation port %d: %w", port, mkErr)
			}
			if info, statErr := os.Stat(dir); statErr == nil && time.Since(info.ModTime()) > federationPortClaimTTL {
				_ = os.RemoveAll(dir)
				continue
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("port %d is claimed by another federation run (%s); "+
					"the slot scenarios need %d-%d to themselves", port, dir, base, maxPort)
			}
			time.Sleep(fedWaitInterval)
		}
	}
	return nil
}

func fedReleasePortClaims() {
	for _, dir := range fedScenario.portClaims {
		_ = os.RemoveAll(dir)
	}
	fedScenario.portClaims = nil
	fedScenario.claimedPort = nil
}

func fedPortFree(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func fedClaim(opts federation.SlotOptions) (*federation.SlotClaim, error) {
	dir, err := fedStateDir()
	if err != nil {
		return nil, err
	}
	if opts.StateDir == "" {
		opts.StateDir = dir
	}
	if opts.ServiceName == "" {
		opts.ServiceName = "ocd-smoke-alarm"
	}
	if err := fedEnsurePortClaims(opts.BasePort, opts.MaxPort); err != nil {
		return nil, err
	}
	return federation.ClaimSlot(opts)
}

// fedClaimAndHold claims a slot and keeps the listener bound for the rest of
// the scenario, so a second claim in the same range elects a follower.
func fedClaimAndHold(opts federation.SlotOptions) (*federation.SlotClaim, error) {
	claim, err := fedClaim(opts)
	if err != nil {
		return nil, err
	}
	fedScenario.claims = append(fedScenario.claims, claim)
	fedScenario.claim = claim
	return claim, nil
}

func fedOptionsForRange() federation.SlotOptions {
	base, maxPort := fedScenario.basePort, fedScenario.maxPort
	if base == 0 {
		base = fedBasePort
	}
	if maxPort == 0 {
		maxPort = fedMaxPort
	}
	return federation.SlotOptions{BasePort: base, MaxPort: maxPort}
}

func fedLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&fedScenario.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func fedAppendEvent(ev federation.Event) {
	fedScenario.mu.Lock()
	defer fedScenario.mu.Unlock()
	fedScenario.events = append(fedScenario.events, ev)
}

func fedEvents() []federation.Event {
	fedScenario.mu.Lock()
	defer fedScenario.mu.Unlock()
	return append([]federation.Event(nil), fedScenario.events...)
}

func fedResetEvents() {
	fedScenario.mu.Lock()
	defer fedScenario.mu.Unlock()
	fedScenario.events = nil
}

func fedEnsureBareRegistry() error {
	if fedScenario.registry != nil {
		return nil
	}
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	maxPeers := fedScenario.maxPeers
	self := federation.Identity{
		ID:          "introducer-self",
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		StateDir:    dir,
		Port:        fedBasePort,
		Role:        federation.RoleIntroducer,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	reg, err := federation.NewRegistry(self, federation.RegistryOptions{
		StateDir:         dir,
		HeartbeatTimeout: fedScenario.heartbeatTimeout,
		MaxPeers:         maxPeers,
		OnChange:         fedAppendEvent,
	})
	if err != nil {
		return err
	}
	fedScenario.registry = reg
	return nil
}

// fedEnsureIntroducer starts the real introducer server on the claimed slot,
// registering any followers the scenario declared before the server existed.
func fedEnsureIntroducer() (string, error) {
	if fedScenario.serverURL != "" {
		return fedScenario.serverURL, nil
	}
	dir, err := fedStateDir()
	if err != nil {
		return "", err
	}
	if fedScenario.claim == nil {
		if _, claimErr := fedClaimAndHold(fedOptionsForRange()); claimErr != nil {
			return "", fmt.Errorf("claim introducer slot: %w", claimErr)
		}
	}

	self := fedScenario.claim.Identity
	reg, err := federation.NewRegistry(self, federation.RegistryOptions{
		StateDir:         dir,
		HeartbeatTimeout: fedScenario.heartbeatTimeout,
		MaxPeers:         fedScenario.maxPeers,
		OnChange:         fedAppendEvent,
	})
	if err != nil {
		return "", err
	}
	ageOut := fedScenario.ageOutInterval
	if ageOut <= 0 {
		ageOut = 50 * time.Millisecond
	}
	srv, err := federation.NewServer(federation.ServerOptions{
		Listener:       fedScenario.claim.Listener,
		Registry:       reg,
		Logger:         fedLogger(),
		AgeOutInterval: ageOut,
	})
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	fedScenario.registry = reg
	fedScenario.server = srv
	fedScenario.serverCancel = cancel
	fedScenario.serverDone = done
	fedScenario.serverURL = "http://" + fedScenario.claim.Listener.Addr().String()

	go func() {
		defer close(done)
		_ = srv.Start(ctx)
	}()

	for _, rec := range fedScenario.pendingFollowers {
		reg.Upsert(rec, "introduction")
	}
	fedScenario.pendingFollowers = nil

	url := fedScenario.serverURL
	if err := fedWaitFor(func() bool {
		resp, getErr := fedTryRequest(http.MethodGet, url+"/membership", nil)
		if getErr != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode == http.StatusOK
	}); err != nil {
		return "", fmt.Errorf("introducer did not serve /membership: %w", err)
	}
	return url, nil
}

func fedStopServer() {
	if fedScenario.serverCancel != nil {
		fedScenario.serverCancel()
		fedScenario.serverCancel = nil
	}
	if fedScenario.serverDone != nil {
		select {
		case <-fedScenario.serverDone:
		case <-time.After(time.Second):
		}
		fedScenario.serverDone = nil
	}
	fedScenario.server = nil
	fedScenario.serverURL = ""
}

func fedWaitFor(cond func() bool) error {
	deadline := time.Now().Add(fedWaitDeadline)
	for {
		if cond() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("condition not met within %s", fedWaitDeadline)
		}
		time.Sleep(fedWaitInterval)
	}
}

func fedTryRequest(method, url string, payload any) (*http.Response, error) {
	var body io.Reader = http.NoBody
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), fedWaitDeadline)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

// fedRequest performs a request and records it in the shared httpState so the
// common "the response status code is N" / "body contains X" steps apply.
func fedRequest(method, path string, payload any) error {
	url, err := fedEnsureIntroducer()
	if err != nil {
		return err
	}
	resp, err := fedTryRequest(method, url+path, payload)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	_ = resp.Body.Close()
	httpState.lastResp = resp
	httpState.lastBody = data
	return nil
}

func fedFollowerRecord() federation.InstanceRecord {
	if fedScenario.follower.ID != "" {
		return fedScenario.follower
	}
	rec := federation.InstanceRecord{
		ID:          "follower-1",
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		Port:        fedBasePort + 1,
		Role:        federation.RoleFollower,
		PID:         os.Getpid(),
		StartedAt:   time.Now().UTC(),
	}
	if fedScenario.basePort != 0 {
		rec.Port = fedScenario.basePort + 1
	}
	fedScenario.follower = rec
	return rec
}

func fedSnapshot() federation.Snapshot {
	if fedScenario.registry == nil {
		return federation.Snapshot{}
	}
	return fedScenario.registry.Snapshot()
}

func fedPeerIDs() map[string]bool {
	out := map[string]bool{}
	for _, p := range fedSnapshot().Peers {
		out[p.ID] = true
	}
	return out
}

// fedNewFollower prepares a follower registry plus a client pointed at the
// running introducer, without starting the client.
func fedNewFollower(announce, heartbeat time.Duration) error {
	url, err := fedEnsureIntroducer()
	if err != nil {
		return err
	}
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	self := federation.Identity{
		ID:          "follower-1",
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		StateDir:    filepath.Join(dir, "follower"),
		Port:        fedBasePort + 1,
		Role:        federation.RoleFollower,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	reg, err := federation.NewRegistry(self, federation.RegistryOptions{
		StateDir:         self.StateDir,
		HeartbeatTimeout: time.Minute,
		OnChange:         fedAppendEvent,
	})
	if err != nil {
		return err
	}
	client, err := federation.NewClient(federation.ClientOptions{
		IntroducerURL:     url,
		Registry:          reg,
		Logger:            fedLogger(),
		AnnounceInterval:  announce,
		HeartbeatInterval: heartbeat,
	})
	if err != nil {
		return err
	}
	fedScenario.followerRegistry = reg
	fedScenario.client = client
	fedScenario.announceInterval = announce
	fedScenario.heartbeatEvery = heartbeat
	return nil
}

func fedStartClient() {
	if fedScenario.clientCancel != nil || fedScenario.client == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	fedScenario.clientCancel = cancel
	fedScenario.clientDone = done
	client := fedScenario.client
	fedScenario.introStartedAt = time.Now()
	go func() {
		defer close(done)
		client.Start(ctx)
	}()
}

func fedStopClient() {
	if fedScenario.clientCancel != nil {
		fedScenario.clientCancel()
		fedScenario.clientCancel = nil
	}
	if fedScenario.clientDone != nil {
		select {
		case <-fedScenario.clientDone:
		case <-time.After(2 * time.Second):
		}
		fedScenario.clientDone = nil
	}
}

// fedStartStubIntroducer serves an introducer that rejects every introduction
// with 500 and counts both endpoints. Used for the "client cannot introduce
// itself" scenarios.
func fedStartStubIntroducer() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for stub introducer: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/introductions", func(w http.ResponseWriter, _ *http.Request) {
		fedScenario.stubIntroductions.Add(1)
		http.Error(w, "nope", http.StatusInternalServerError)
	})
	mux.HandleFunc("/heartbeats", func(w http.ResponseWriter, _ *http.Request) {
		fedScenario.stubHeartbeats.Add(1)
		http.Error(w, "nope", http.StatusInternalServerError)
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	fedScenario.stubListener = ln
	fedScenario.stubServer = srv
	go func() { _ = srv.Serve(ln) }()
	return nil
}

func fedStubURL() string {
	if fedScenario.stubListener == nil {
		return ""
	}
	return "http://" + fedScenario.stubListener.Addr().String()
}

// ── background ───────────────────────────────────────────────────────────────

func federationIsEnabledInTheConfig() error {
	cfg, err := config.LoadBytes([]byte("version: \"1\"\nservice:\n  name: \"ocd-smoke-alarm\"\nfederation:\n  enabled: true\n"))
	if err != nil {
		return fmt.Errorf("load config with federation enabled: %w", err)
	}
	if !cfg.Federation.Enabled {
		return errors.New("federation.enabled did not survive config loading")
	}
	fedScenario.cfg = cfg
	return nil
}

// ── slot election ────────────────────────────────────────────────────────────

func noOtherInstanceRunningOnPortRange() error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	fedScenario.basePort, fedScenario.maxPort = fedBasePort, fedMaxPort
	for port := fedScenario.basePort; port <= fedScenario.maxPort; port++ {
		if !fedPortFree(port) {
			return fmt.Errorf("port %d is already bound by a non-test process; these scenarios claim %d-%d for real",
				port, fedBasePort, fedMaxPort)
		}
	}
	return fedEnsurePortClaims(fedScenario.basePort, fedScenario.maxPort)
}

func anInstanceStartsWithPorts(base, maxPort int) error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	fedScenario.basePort, fedScenario.maxPort = base, maxPort
	claim, err := fedClaimAndHold(federation.SlotOptions{BasePort: base, MaxPort: maxPort})
	if err != nil {
		return fmt.Errorf("start instance on %d-%d: %w", base, maxPort, err)
	}
	if claim.Identity.Port != base {
		return fmt.Errorf("expected introducer to bind base_port %d, got %d", base, claim.Identity.Port)
	}
	return nil
}

func theInstanceBindsPort(port int) error {
	if fedScenario.claim == nil {
		return errors.New("no slot was claimed")
	}
	if fedScenario.claim.Identity.Port != port {
		return fmt.Errorf("expected bind on port %d, got %d", port, fedScenario.claim.Identity.Port)
	}
	if _, err := net.DialTimeout("tcp", fedScenario.claim.Listener.Addr().String(), 200*time.Millisecond); err != nil {
		return fmt.Errorf("listener on %s is not accepting: %w", fedScenario.claim.Listener.Addr(), err)
	}
	return nil
}

func theInstanceIdentityRoleIs(role string) error {
	if fedScenario.claim == nil {
		return errors.New("no slot was claimed")
	}
	if got := string(fedScenario.claim.Identity.Role); got != role {
		return fmt.Errorf("expected role %q, got %q", role, got)
	}
	return nil
}

func theIdentityIsPersistedTo(path string) error {
	abs := resolveTestFilePath(path)
	data, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("identity file %q: %w", abs, err)
	}
	var ident federation.Identity
	if err := json.Unmarshal(data, &ident); err != nil {
		return fmt.Errorf("decode identity file %q: %w", abs, err)
	}
	if fedScenario.claim != nil && ident.Port != fedScenario.claim.Identity.Port {
		return fmt.Errorf("persisted identity reports port %d, claimed %d", ident.Port, fedScenario.claim.Identity.Port)
	}
	if ident.ID == "" {
		return errors.New("persisted identity has an empty id")
	}
	return nil
}

func anIntroducerAlreadyBoundOnPort(port int) error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	// The introducer holds base_port; the rest of the range is still free for a
	// follower to claim.
	fedScenario.basePort, fedScenario.maxPort = port, fedMaxPort
	claim, err := fedClaimAndHold(federation.SlotOptions{BasePort: port, MaxPort: port})
	if err != nil {
		return fmt.Errorf("bind introducer on port %d: %w", port, err)
	}
	if claim.Identity.Role != federation.RoleIntroducer {
		return fmt.Errorf("first claim on %d became %q, expected introducer", port, claim.Identity.Role)
	}
	return nil
}

func aSecondInstanceStartsSamePorts() error {
	if fedScenario.basePort == 0 {
		fedScenario.basePort, fedScenario.maxPort = fedBasePort, fedMaxPort
	}
	claim, err := fedClaimAndHold(fedOptionsForRange())
	if err != nil {
		return fmt.Errorf("start second instance: %w", err)
	}
	fedScenario.claim = claim
	return nil
}

func theSecondInstanceBindsPortPlusDelta(base, delta int) error {
	if fedScenario.claim == nil {
		return errors.New("no slot was claimed for the second instance")
	}
	if want := base + delta; fedScenario.claim.Identity.Port != want {
		return fmt.Errorf("expected second instance on port %d, got %d", want, fedScenario.claim.Identity.Port)
	}
	return nil
}

func theSecondInstanceIdentityRoleIs(role string) error {
	return theInstanceIdentityRoleIs(role)
}

func allPortsAlreadyBound() error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	if fedScenario.basePort == 0 {
		fedScenario.basePort, fedScenario.maxPort = fedBasePort, fedMaxPort
	}
	for port := fedScenario.basePort; port <= fedScenario.maxPort; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
		if err != nil {
			return fmt.Errorf("occupy port %d: %w", port, err)
		}
		fedScenario.occupiers = append(fedScenario.occupiers, ln)
	}
	return nil
}

// fedClaimRecordingFailure claims a slot and records the outcome instead of
// returning it: several scenarios assert on the failure (ErrNoFreeSlots, a busy
// lock, a stale lock) rather than treating it as a step error.
func fedClaimRecordingFailure(opts federation.SlotOptions) {
	claim, err := fedClaim(opts)
	fedScenario.lastErr = err
	if err == nil {
		fedScenario.claim = claim
		fedScenario.claims = append(fedScenario.claims, claim)
	}
}

func claimSlotIsCalled() error {
	opts := fedOptionsForRange()
	if fedScenario.claimOptions != nil {
		opts = *fedScenario.claimOptions
	}
	fedClaimRecordingFailure(opts)
	return nil
}

func theSlotClaimFailsWith(message string) error {
	if fedScenario.lastErr == nil {
		return fmt.Errorf("expected claim to fail with %q, but it succeeded", message)
	}
	if fedScenario.lastErr.Error() != message {
		return fmt.Errorf("expected error %q, got %w", message, fedScenario.lastErr)
	}
	return nil
}

func aConfigWithNoFederationBasePort() error {
	cfg, err := config.LoadBytes([]byte("version: \"1\"\nservice:\n  name: \"ocd-smoke-alarm\"\nfederation:\n  enabled: true\n"))
	if err != nil {
		return fmt.Errorf("load config without federation.base_port: %w", err)
	}
	if cfg.Federation.BasePort != federation.DefaultBasePort {
		return fmt.Errorf("config default base_port is %d, federation.DefaultBasePort is %d",
			cfg.Federation.BasePort, federation.DefaultBasePort)
	}
	fedScenario.cfg = cfg
	fedScenario.claimOptions = &federation.SlotOptions{
		BasePort: cfg.Federation.BasePort,
		MaxPort:  cfg.Federation.MaxPort,
	}
	return nil
}

func portsAreTheCandidates(from, to int) error {
	if federation.DefaultBasePort != from {
		return fmt.Errorf("expected candidate base %d, got %d", from, federation.DefaultBasePort)
	}
	if last := federation.DefaultBasePort + federation.DefaultSlotCount - 1; last != to {
		return fmt.Errorf("expected last candidate %d, got %d", to, last)
	}
	if fedScenario.cfg.Federation.BasePort != from || fedScenario.cfg.Federation.MaxPort != to {
		return fmt.Errorf("config derived candidates %d-%d, expected %d-%d",
			fedScenario.cfg.Federation.BasePort, fedScenario.cfg.Federation.MaxPort, from, to)
	}
	if fedScenario.claim == nil {
		return errors.New("ClaimSlot did not claim a slot")
	}
	if fedScenario.claim.Identity.Port != from {
		return fmt.Errorf("expected ClaimSlot to take the first candidate %d, got %d",
			from, fedScenario.claim.Identity.Port)
	}
	return nil
}

// ── instance identity ────────────────────────────────────────────────────────

func twoClaimSlotCallsIdentical() error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	fedScenario.claimOptions = &federation.SlotOptions{BasePort: fedBasePort, MaxPort: fedBasePort}
	return nil
}

func bothInstanceIDsAreComputed() error {
	if fedScenario.claimOptions == nil {
		return errors.New("no ClaimSlot options were prepared")
	}
	ids, err := fedClaimTwice(*fedScenario.claimOptions)
	if err != nil {
		return err
	}
	fedScenario.ids = ids
	return nil
}

func fedClaimTwice(opts federation.SlotOptions) ([]string, error) {
	var ids []string
	for i := 0; i < 2; i++ {
		claim, err := fedClaim(opts)
		if err != nil {
			return nil, fmt.Errorf("claim %d: %w", i+1, err)
		}
		ids = append(ids, claim.Identity.ID)
		_ = claim.Close()
	}
	return ids, nil
}

func bothIDsAreEqual() error {
	if len(fedScenario.ids) != 2 {
		return fmt.Errorf("expected 2 instance IDs, got %d", len(fedScenario.ids))
	}
	if fedScenario.ids[0] != fedScenario.ids[1] {
		return fmt.Errorf("identical claims produced different IDs: %q vs %q", fedScenario.ids[0], fedScenario.ids[1])
	}
	return nil
}

func twoClaimSlotCallsDifferentPorts() error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	first, err := fedClaim(federation.SlotOptions{BasePort: fedBasePort, MaxPort: fedBasePort})
	if err != nil {
		return fmt.Errorf("claim on port %d: %w", fedBasePort, err)
	}
	firstID := first.Identity.ID
	_ = first.Close()

	second, err := fedClaim(federation.SlotOptions{BasePort: fedBasePort + 1, MaxPort: fedBasePort + 1})
	if err != nil {
		return fmt.Errorf("claim on port %d: %w", fedBasePort+1, err)
	}
	secondID := second.Identity.ID
	_ = second.Close()

	fedScenario.ids = []string{firstID, secondID}
	return nil
}

func theTwoInstanceIDsAreDifferent() error {
	if len(fedScenario.ids) != 2 {
		return fmt.Errorf("expected 2 instance IDs, got %d", len(fedScenario.ids))
	}
	if fedScenario.ids[0] == fedScenario.ids[1] {
		return fmt.Errorf("claims on different ports produced the same ID %q", fedScenario.ids[0])
	}
	return nil
}

func aPersistedIdentityExistsWithPort(port int) error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	slotDir := filepath.Join(dir, "federation")
	if mkErr := os.MkdirAll(slotDir, 0o755); mkErr != nil {
		return fmt.Errorf("create slot dir: %w", mkErr)
	}
	ident := federation.Identity{
		ID:          "previous-identity",
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		StateDir:    dir,
		Port:        port,
		Role:        federation.RoleFollower,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	data, err := json.MarshalIndent(ident, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(slotDir, "identity.json"), data, 0o600); err != nil {
		return fmt.Errorf("write identity.json: %w", err)
	}
	fedScenario.claimOptions = &federation.SlotOptions{BasePort: port, MaxPort: fedMaxPort}
	return nil
}

func claimSlotCalledAndPortIsFree(port int) error {
	if !fedPortFree(port) {
		return fmt.Errorf("port %d is not free; cannot exercise identity restore", port)
	}
	if fedScenario.claimOptions == nil {
		fedScenario.claimOptions = &federation.SlotOptions{BasePort: port, MaxPort: fedMaxPort}
	}
	return claimSlotIsCalled()
}

func theInstanceClaimsPortFirst(port int) error {
	if fedScenario.lastErr != nil {
		return fmt.Errorf("ClaimSlot failed: %w", fedScenario.lastErr)
	}
	if fedScenario.claim == nil {
		return errors.New("no slot was claimed")
	}
	if fedScenario.claim.Identity.Port != port {
		return fmt.Errorf("expected the persisted identity's port %d to be tried first, got %d",
			port, fedScenario.claim.Identity.Port)
	}
	return nil
}

// ── slot lock ────────────────────────────────────────────────────────────────

func twoProcessesAttemptClaimSlotSimultaneously() error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	base, maxPort := fedBasePort, fedMaxPort
	if fedScenario.basePort != 0 {
		base, maxPort = fedScenario.basePort, fedScenario.maxPort
	}
	results := make([]fedClaimResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = fedRunClaimHelper(dir, base, maxPort)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("helper process %d: %w", i+1, err)
		}
	}
	fedScenario.lockResults = results
	return nil
}

func exactlyOneProcessSucceedsAsIntroducer() error {
	introducers := 0
	for _, res := range fedScenario.lockResults {
		if res.Err == "" && res.Role == string(federation.RoleIntroducer) {
			introducers++
		}
	}
	if introducers != 1 {
		return fmt.Errorf("expected exactly one introducer, got %d (results: %+v)", introducers, fedScenario.lockResults)
	}
	return nil
}

func theOtherProcessBecomesFollowerOrError() error {
	for _, res := range fedScenario.lockResults {
		if res.Err == "" && res.Role == string(federation.RoleIntroducer) {
			continue
		}
		if res.Err != "" || res.Role == string(federation.RoleFollower) {
			return nil
		}
		return fmt.Errorf("second process neither failed nor became a follower: %+v", res)
	}
	return errors.New("no second process result recorded")
}

func aStaleLockFileExists() error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	slotDir := filepath.Join(dir, "federation")
	if mkErr := os.MkdirAll(slotDir, 0o755); mkErr != nil {
		return fmt.Errorf("create slot dir: %w", mkErr)
	}
	// A PID that is certainly not running: start a short-lived process and wait
	// for it to exit.
	cmd := exec.Command("sleep", "0.01")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn throwaway process: %w", err)
	}
	deadPID := cmd.Process.Pid
	_ = cmd.Wait()

	lockPath := filepath.Join(slotDir, "slot.lock")
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(deadPID)+"\n"), 0o600); err != nil {
		return fmt.Errorf("write stale slot.lock: %w", err)
	}
	fedScenario.claimOptions = &federation.SlotOptions{BasePort: fedBasePort, MaxPort: fedMaxPort}
	return nil
}

func theStaleLockIsRemoved() error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	lockPath := filepath.Join(dir, "federation", "slot.lock")
	if _, err := os.Stat(lockPath); err == nil {
		return fmt.Errorf("stale lock %q still present after ClaimSlot", lockPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %q: %w", lockPath, err)
	}
	return nil
}

func claimSlotProceedsNormally() error {
	if fedScenario.lastErr != nil {
		return fmt.Errorf("ClaimSlot did not recover from the stale lock: %w", fedScenario.lastErr)
	}
	if fedScenario.claim == nil {
		return errors.New("ClaimSlot returned no claim")
	}
	return nil
}

// ── introducer server ────────────────────────────────────────────────────────

func theIntroducerHoldsSlotClaimOnPort(port int) error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	fedScenario.basePort, fedScenario.maxPort = port, port
	if _, err := fedClaimAndHold(federation.SlotOptions{BasePort: port, MaxPort: port}); err != nil {
		return fmt.Errorf("claim introducer slot on %d: %w", port, err)
	}
	return nil
}

func theFederationServerStarts() error {
	_, err := fedEnsureIntroducer()
	return err
}

func postIntroductionsIsServed() error {
	rec := fedFollowerRecord()
	if err := fedRequest(http.MethodPost, "/introductions", map[string]any{"record": rec}); err != nil {
		return err
	}
	return fedExpectOK()
}

func postHeartbeatsIsServed() error {
	rec := fedFollowerRecord()
	if err := fedRequest(http.MethodPost, "/heartbeats", rec); err != nil {
		return err
	}
	return fedExpectOK()
}

func getMembershipIsServed() error {
	if err := fedRequest(http.MethodGet, "/membership", nil); err != nil {
		return err
	}
	return fedExpectOK()
}

func fedExpectOK() error {
	if httpState.lastResp == nil {
		return errors.New("no HTTP response recorded")
	}
	if httpState.lastResp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status %d, got %d (body: %s)",
			http.StatusOK, httpState.lastResp.StatusCode, httpState.lastBody)
	}
	return nil
}

func theFederationServerIsRunningAsIntroducer() error {
	_, err := fedEnsureIntroducer()
	return err
}

func aFollowerPostsToIntroductions() error {
	rec := fedFollowerRecord()
	if err := fedRequest(http.MethodPost, "/introductions", map[string]any{"record": rec}); err != nil {
		return err
	}
	return fedExpectOK()
}

func thePeerAppearsInRegistry() error {
	rec := fedFollowerRecord()
	return fedWaitFor(func() bool { return fedPeerIDs()[rec.ID] })
}

func aPostSentWithEmptyField(field string) error {
	if field != "id" {
		return fmt.Errorf("unsupported empty field %q; only record.id is validated", field)
	}
	return fedRequest(http.MethodPost, "/introductions", map[string]any{
		"record": federation.InstanceRecord{ID: ""},
	})
}

func aGETRequestSentToIntroducerIntroductions() error {
	return fedRequest(http.MethodGet, "/introductions", nil)
}

// aFollowerAlreadyRegistered queues a follower without starting the introducer,
// so a later step can set heartbeats.timeout before the registry exists. The
// record is upserted when the introducer starts (fedEnsureIntroducer).
func aFollowerAlreadyRegistered() error {
	rec := fedFollowerRecord()
	if fedScenario.registry != nil {
		fedScenario.registry.Upsert(rec, "introduction")
		return nil
	}
	fedScenario.pendingFollowers = append(fedScenario.pendingFollowers, rec)
	return nil
}

func theFollowerPostsToHeartbeats() error {
	rec := fedFollowerRecord()
	if _, err := fedEnsureIntroducer(); err != nil {
		return err
	}
	for _, p := range fedSnapshot().Peers {
		if p.ID == rec.ID {
			fedScenario.lastSeen = p.LastSeenAt
		}
	}
	// Distinct timestamps: LastSeenAt has nanosecond resolution, but the
	// heartbeat must be strictly later than the registration to prove a refresh.
	time.Sleep(2 * time.Millisecond)
	return fedRequest(http.MethodPost, "/heartbeats", rec)
}

func thePeersLastSeenAtIsUpdated() error {
	rec := fedFollowerRecord()
	var got time.Time
	for _, p := range fedSnapshot().Peers {
		if p.ID == rec.ID {
			got = p.LastSeenAt
		}
	}
	if got.IsZero() {
		return fmt.Errorf("peer %q is absent from the registry", rec.ID)
	}
	if !got.After(fedScenario.lastSeen) {
		return fmt.Errorf("last_seen_at was not refreshed: %s (was %s)", got, fedScenario.lastSeen)
	}
	return nil
}

func theFederationServerHasRegisteredPeers(n int) error {
	if _, err := fedEnsureIntroducer(); err != nil {
		return err
	}
	for i := 1; i <= n; i++ {
		fedScenario.registry.Upsert(fedPeerRecord(fmt.Sprintf("peer-%03d", i)), "seed")
	}
	return nil
}

func fedPeerRecord(id string) federation.InstanceRecord {
	return federation.InstanceRecord{
		ID:          id,
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		Port:        fedBasePort + 2,
		Role:        federation.RoleFollower,
		PID:         os.Getpid(),
		StartedAt:   time.Now().UTC(),
	}
}

func theArrayLengthIs(key string, n int) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(httpState.lastBody, &payload); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}
	raw, ok := payload[key]
	if !ok {
		return fmt.Errorf("response body has no %q field (body: %s)", key, httpState.lastBody)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("decode %q as array: %w", key, err)
	}
	if len(items) != n {
		return fmt.Errorf("expected %q length %d, got %d (body: %s)", key, n, len(items), httpState.lastBody)
	}
	return nil
}

// ── age-out ──────────────────────────────────────────────────────────────────

func theRegistryHeartbeatTimeoutIs(raw string) error {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("parse heartbeat timeout %q: %w", raw, err)
	}
	if fedScenario.registry != nil {
		return errors.New("heartbeat_timeout must be set before the introducer is created")
	}
	fedScenario.heartbeatTimeout = d
	return nil
}

func theFollowerSendsNoHeartbeatForMilliseconds(ms int) error {
	if _, err := fedEnsureIntroducer(); err != nil {
		return err
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return nil
}

func theFollowerIsRemovedFromRegistry() error {
	rec := fedFollowerRecord()
	return fedWaitFor(func() bool { return !fedPeerIDs()[rec.ID] })
}

func theRemovalReasonIs(reason string) error {
	rec := fedFollowerRecord()
	for _, ev := range fedEvents() {
		if ev.Type == federation.EventRemoved && ev.Record.ID == rec.ID && ev.Reason == reason {
			return nil
		}
	}
	return fmt.Errorf("no EventRemoved for %q with reason %q (events: %+v)", rec.ID, reason, fedEvents())
}

func theRemovalIsReflectedInMembership() error {
	rec := fedFollowerRecord()
	if err := fedRequest(http.MethodGet, "/membership", nil); err != nil {
		return err
	}
	if err := fedExpectOK(); err != nil {
		return err
	}
	if strings.Contains(string(httpState.lastBody), rec.ID) {
		return fmt.Errorf("removed peer %q is still in /membership: %s", rec.ID, httpState.lastBody)
	}
	return nil
}

func theFollowerSendsHeartbeatAtMilliseconds(ms int) error {
	if _, err := fedEnsureIntroducer(); err != nil {
		return err
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	if err := fedRequest(http.MethodPost, "/heartbeats", fedFollowerRecord()); err != nil {
		return err
	}
	return fedExpectOK()
}

func theFollowerRemainsInRegistryAfterMilliseconds(ms int) error {
	rec := fedFollowerRecord()
	time.Sleep(time.Duration(ms) * time.Millisecond)
	if !fedPeerIDs()[rec.ID] {
		return fmt.Errorf("peer %q was aged out despite heartbeating inside the timeout", rec.ID)
	}
	return nil
}

// ── peer cap ─────────────────────────────────────────────────────────────────

func registryAtMaxPeers(n int) error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	self := federation.Identity{
		ID:          "introducer-self",
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		StateDir:    dir,
		Port:        fedBasePort,
		Role:        federation.RoleIntroducer,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	reg, err := federation.NewRegistry(self, federation.RegistryOptions{
		StateDir: dir,
		MaxPeers: n,
		OnChange: fedAppendEvent,
	})
	if err != nil {
		return err
	}
	fedScenario.registry = reg
	fedScenario.maxPeers = n
	for i := 1; i <= n; i++ {
		reg.Upsert(fedPeerRecord(fmt.Sprintf("peer-%03d", i)), "seed")
	}
	fedScenario.peerBaseline = len(fedSnapshot().Peers)
	fedScenario.eventBaseline = len(fedEvents())
	return nil
}

func upsertCalledWithNewPeerRecord() error {
	if fedScenario.registry == nil {
		if err := fedEnsureBareRegistry(); err != nil {
			return err
		}
	}
	fedScenario.lastErr = nil // Registry.Upsert reports refusals through the event, not an error
	fedScenario.registry.Upsert(fedPeerRecord("peer-overflow"), "introduction")
	return nil
}

func registryPeerCountDoesNotIncrease() error {
	if got := len(fedSnapshot().Peers); got != fedScenario.peerBaseline {
		return fmt.Errorf("expected peer count to stay at %d, got %d", fedScenario.peerBaseline, got)
	}
	return nil
}

func theUpsertReportsNoError() error {
	if fedScenario.lastErr != nil {
		return fmt.Errorf("expected no error, got: %w", fedScenario.lastErr)
	}
	return nil
}

func upsertCalledWithOwnID() error {
	if fedScenario.registry == nil {
		if err := fedEnsureBareRegistry(); err != nil {
			return err
		}
	}
	fedScenario.peerBaseline = len(fedSnapshot().Peers)
	fedScenario.eventBaseline = len(fedEvents())
	fedScenario.registry.Upsert(fedScenario.registry.Self(), "heartbeat")
	return nil
}

func registryPeerCountDoesNotChange() error {
	if got := len(fedSnapshot().Peers); got != fedScenario.peerBaseline {
		return fmt.Errorf("expected peer count to stay at %d, got %d", fedScenario.peerBaseline, got)
	}
	return nil
}

func noEventIsFired() error {
	if got := len(fedEvents()); got != fedScenario.eventBaseline {
		return fmt.Errorf("expected no new events, got %d (baseline %d): %+v",
			got, fedScenario.eventBaseline, fedEvents())
	}
	return nil
}

// ── follower client ──────────────────────────────────────────────────────────

func aFollowerClientConfiguredWithIntroducerURL() error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	return fedNewFollower(5*time.Second, 5*time.Second)
}

func startIsCalled() error {
	fedStartClient()
	rec := fedFollowerRecord()
	err := fedWaitFor(func() bool { return fedPeerIDs()[rec.ID] })
	if err == nil {
		fedScenario.introducedAt = time.Now()
	}
	return err
}

func postIntroductionsSentBeforeFirstInterval() error {
	if fedScenario.introducedAt.IsZero() {
		return errors.New("the follower never appeared in the introducer's registry")
	}
	elapsed := fedScenario.introducedAt.Sub(fedScenario.introStartedAt)
	if elapsed >= fedScenario.announceInterval {
		return fmt.Errorf("introduction took %s, not before the first announce_interval (%s)",
			elapsed, fedScenario.announceInterval)
	}
	return nil
}

func aFollowerClientNotYetIntroduced() error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	if err := fedStartStubIntroducer(); err != nil {
		return err
	}
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	self := federation.Identity{
		ID:          "follower-1",
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		StateDir:    filepath.Join(dir, "follower"),
		Port:        fedBasePort + 1,
		Role:        federation.RoleFollower,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	reg, err := federation.NewRegistry(self, federation.RegistryOptions{StateDir: self.StateDir})
	if err != nil {
		return err
	}
	client, err := federation.NewClient(federation.ClientOptions{
		IntroducerURL:     fedStubURL(),
		Registry:          reg,
		Logger:            fedLogger(),
		AnnounceInterval:  time.Second,
		HeartbeatInterval: 50 * time.Millisecond,
	})
	if err != nil {
		return err
	}
	fedScenario.client = client
	fedScenario.followerRegistry = reg
	fedScenario.announceInterval = time.Second
	fedScenario.heartbeatEvery = 50 * time.Millisecond
	return nil
}

func theHeartbeatIntervalElapses() error {
	startedHere := fedScenario.clientCancel == nil
	fedStartClient()
	time.Sleep(250 * time.Millisecond)
	if startedHere {
		fedStopClient()
	}
	return nil
}

func noPostToHeartbeatsSent() error {
	if got := fedScenario.stubHeartbeats.Load(); got != 0 {
		return fmt.Errorf("expected no /heartbeats POST before a successful introduction, got %d", got)
	}
	if fedScenario.stubIntroductions.Load() == 0 {
		return errors.New("the client never attempted an introduction, so the heartbeat silence proves nothing")
	}
	return nil
}

func theIntroducerResponseContainsPeerRecords(n int) error {
	if _, err := fedEnsureIntroducer(); err != nil {
		return err
	}
	for i := 1; i <= n; i++ {
		fedScenario.registry.Upsert(fedPeerRecord(fmt.Sprintf("peer-%03d", i)), "seed")
	}
	return fedNewFollower(5*time.Second, 5*time.Second)
}

func theFollowerProcessesIntroductionResponse() error {
	fedStartClient()
	return fedWaitFor(func() bool {
		if fedScenario.followerRegistry == nil {
			return false
		}
		ids := map[string]bool{}
		for _, p := range fedScenario.followerRegistry.Snapshot().Peers {
			ids[p.ID] = true
		}
		return ids["peer-001"] && ids["peer-002"]
	})
}

func bothPeersAreUpserted() error {
	if fedScenario.followerRegistry == nil {
		return errors.New("no follower registry")
	}
	ids := map[string]bool{}
	for _, p := range fedScenario.followerRegistry.Snapshot().Peers {
		ids[p.ID] = true
	}
	for _, want := range []string{"peer-001", "peer-002"} {
		if !ids[want] {
			return fmt.Errorf("peer %q missing from the follower registry (has %v)", want, ids)
		}
	}
	return nil
}

func theFollowerRegistrySnapshotSaved() error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "follower", "federation", "registry.json")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("follower registry snapshot %q: %w", path, err)
	}
	return nil
}

func theIntroducerURLIsUnreachable() error {
	if _, err := fedStateDir(); err != nil {
		return err
	}
	// Reserve a port and release it, so the URL is well-formed but dead.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	deadURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	self := federation.Identity{
		ID:          "follower-1",
		ServiceName: "ocd-smoke-alarm",
		Hostname:    "localhost",
		StateDir:    filepath.Join(dir, "follower"),
		Port:        fedBasePort + 1,
		Role:        federation.RoleFollower,
		PID:         os.Getpid(),
		CreatedAt:   time.Now().UTC(),
	}
	reg, err := federation.NewRegistry(self, federation.RegistryOptions{StateDir: self.StateDir})
	if err != nil {
		return err
	}
	client, err := federation.NewClient(federation.ClientOptions{
		IntroducerURL:     deadURL,
		Registry:          reg,
		Logger:            fedLogger(),
		AnnounceInterval:  50 * time.Millisecond,
		HeartbeatInterval: time.Second,
	})
	if err != nil {
		return err
	}
	fedScenario.client = client
	fedScenario.followerRegistry = reg
	fedScenario.announceInterval = 50 * time.Millisecond
	return nil
}

func theFollowerAttemptsToSendIntroduction() error {
	fedStartClient()
	time.Sleep(250 * time.Millisecond)
	fedStopClient()
	return nil
}

func theClientRetriesOnNextInterval() error {
	if got := strings.Count(fedScenario.logs.String(), "federation introduction failed"); got < 2 {
		return fmt.Errorf("expected repeated introduction warnings, got %d (log: %s)", got, fedScenario.logs.String())
	}
	return nil
}

func theFollowerIntroducedAndIntroducerUnreachable() error {
	if _, err := fedEnsureIntroducer(); err != nil {
		return err
	}
	if err := fedNewFollower(5*time.Second, 40*time.Millisecond); err != nil {
		return err
	}
	fedStartClient()
	rec := fedFollowerRecord()
	if err := fedWaitFor(func() bool { return fedPeerIDs()[rec.ID] }); err != nil {
		return fmt.Errorf("follower never introduced itself: %w", err)
	}
	fedStopServer()
	return nil
}

func theFederationLogContains(message string) error {
	if !strings.Contains(fedScenario.logs.String(), message) {
		return fmt.Errorf("federation log does not contain %q\nlog: %s", message, fedScenario.logs.String())
	}
	return nil
}

// ── registry snapshot ────────────────────────────────────────────────────────

func saveSnapshotIsCalled() error {
	if err := fedEnsureBareRegistry(); err != nil {
		return err
	}
	fedScenario.lastErr = fedScenario.registry.SaveSnapshot()
	return fedScenario.lastErr
}

func theFileExists(path string) error {
	abs := resolveTestFilePath(path)
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("file %q does not exist: %w", abs, err)
	}
	return nil
}

// resolveTestFilePath resolves a path for file-existence assertions.
// "<state_dir>" is substituted with the scenario's federation state directory;
// otherwise relative paths resolve against ksState.storeBaseDir (known-state
// scenarios) or the project root.
func resolveTestFilePath(path string) string {
	if fedScenario.stateDir != "" {
		path = strings.ReplaceAll(path, "<state_dir>", fedScenario.stateDir)
	}
	if filepath.IsAbs(path) {
		return path
	}
	if ksState.storeBaseDir != "" {
		return filepath.Join(ksState.storeBaseDir, path)
	}
	return filepath.Join(projectRoot, path)
}

func noTemporaryFileRemains() error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "federation", "registry.json.tmp-*"))
	if err != nil {
		return err
	}
	if len(matches) > 0 {
		return fmt.Errorf("temporary snapshot files remain: %v", matches)
	}
	return nil
}

func theSnapshotJSONContains(key string) error {
	dir, err := fedStateDir()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, "federation", "registry.json"))
	if err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	if !strings.Contains(string(data), `"`+key+`"`) {
		return fmt.Errorf("snapshot JSON has no %q field: %s", key, data)
	}
	return nil
}

func theRegistryIsAtVersion(n int) error {
	if err := fedEnsureBareRegistry(); err != nil {
		return err
	}
	for i := 1; fedSnapshot().Version < uint64(n); i++ {
		if i > n+1 {
			return fmt.Errorf("could not reach registry version %d (at %d)", n, fedSnapshot().Version)
		}
		fedScenario.registry.Upsert(fedPeerRecord(fmt.Sprintf("version-peer-%d", i)), "seed")
	}
	if got := fedSnapshot().Version; got != uint64(n) {
		return fmt.Errorf("expected registry version %d, got %d", n, got)
	}
	return nil
}

func aPeerIsUpserted() error {
	if fedScenario.registry == nil {
		if err := fedEnsureBareRegistry(); err != nil {
			return err
		}
	}
	fedScenario.registry.Upsert(fedPeerRecord("version-peer-extra"), "seed")
	return nil
}

func theRegistryVersionIsPlus(base, delta int) error {
	if got := fedSnapshot().Version; got != uint64(base+delta) {
		return fmt.Errorf("expected registry version %d, got %d", base+delta, got)
	}
	return nil
}

// ── registry events ──────────────────────────────────────────────────────────

func theRegistryHasNoPeerWithID(id string) error {
	if err := fedEnsureBareRegistry(); err != nil {
		return err
	}
	if fedPeerIDs()[id] {
		return fmt.Errorf("registry unexpectedly already contains %q", id)
	}
	fedResetEvents()
	fedScenario.eventBaseline = 0
	return nil
}

func upsertCalledWithID(id string) error {
	if fedScenario.registry == nil {
		if err := fedEnsureBareRegistry(); err != nil {
			return err
		}
	}
	fedScenario.registry.Upsert(fedPeerRecord(id), "introduction")
	return nil
}

func theOnChangeCallbackReceivesEvent(eventType string) error {
	for _, ev := range fedEvents() {
		if string(ev.Type) == eventType {
			return nil
		}
	}
	return fmt.Errorf("no %q event recorded (events: %+v)", eventType, fedEvents())
}

func theRegistryAlreadyHasPeerWithID(id string) error {
	if err := fedEnsureBareRegistry(); err != nil {
		return err
	}
	if !fedPeerIDs()[id] {
		fedScenario.registry.Upsert(fedPeerRecord(id), "introduction")
	}
	fedResetEvents()
	return nil
}

func upsertCalledAgainSameID() error {
	if fedScenario.registry == nil {
		return errors.New("no registry")
	}
	fedScenario.registry.Upsert(fedPeerRecord("peer-001"), "heartbeat")
	return nil
}

func theRegistryHasPeerWithID(id string) error {
	if err := fedEnsureBareRegistry(); err != nil {
		return err
	}
	if !fedPeerIDs()[id] {
		fedScenario.registry.Upsert(fedPeerRecord(id), "introduction")
	}
	fedResetEvents()
	fedScenario.lastRemoved = id
	return nil
}

func removeCalledWithThatID() error {
	if fedScenario.registry == nil {
		return errors.New("no registry")
	}
	id := "peer-001"
	if fedScenario.lastRemoved != "" {
		id = fedScenario.lastRemoved
	}
	fedScenario.registry.Remove(id, "test")
	return nil
}
