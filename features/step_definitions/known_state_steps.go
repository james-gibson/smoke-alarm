package stepdefinitions

// known_state_steps.go — step definitions for features/known-state.feature
//
// RESOLVED (2026-03-26): all steps implemented against internal/knownstate/store.go.
//
// Steps owned here (must not be re-registered in other domain files):
//   "a known-state store is initialised with path {string}"
//   "a status value {string}"
//   "I call IsHealthy on the status"
//   "IsHealthy returns {word}"
//   "I call IsFailure on the status"
//   "IsFailure returns {word}"
//   "sustain_success is set to {int}"
//   "{int} consecutive healthy probe results are recorded for target {string}"
//   "{int} healthy probe result is recorded for target {string}"
//   "{int} degraded probe result is recorded for target {string}"
//   "{int} degraded probe results are recorded for target {string}"
//   "{int} consecutive healthy probe results have been recorded for target {string}"
//   "{int} degraded probe results have been recorded for target {string}"
//   "the target {string} ever_healthy is {word}"
//   "the target {string} last_healthy_at is set"
//   "the target {string} success_streak is {int}"
//   "the target {string} consecutive_failures is {int}"
//   "the target {string} current_status is {string}"
//   "the store has target {string} with ever_healthy {word}"
//   "a degraded probe result has been recorded for target {string}"
//   "a healthy probe result has been recorded for target {string}"
//   "a {string} probe result is recorded for target {string}"
//   "the update result is_regression is {word}"
//   "the update result became_healthy is {word}"
//   "the update result became_unhealthy is {word}"
//   "the store has no entry for target {string}"
//   "the update result had_previous is {word}"
//   "a probe result is recorded for target {string}"
//   "a probe result with no status is recorded for target {string}"
//   "an error is returned containing {string}"
//   "I call Get for target {string}"
//   "the returned state ever_healthy matches the stored value"
//   "the returned found flag is false"
//   "I call Save explicitly"
//   "no {string} file remains"
//   "the store is initialised with auto_persist {word}"
//   "a snapshot file exists at {string} with target {string} marked ever_healthy"
//   "no file exists at {string}"
//   "the store loads from disk"
//   "the target map is empty"
//   "I call Snapshot and mutate the returned map"
//   "the store's internal target {string} is unchanged"
//   "I call Reset with delete_file {word}"
//   "the file {string} exists with an empty targets map"
//   "the file {string} does not exist"
//
// Steps owned elsewhere but used here:
//   "the ocd-smoke-alarm binary is installed"  → common_steps.go
//   "no error is returned"                      → federation_steps.go
//   "the file {string} exists"                  → federation_steps.go

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/knownstate"
)

// ksState holds per-scenario state for known-state step definitions.
// It is also read by federation_steps.go (same package) for theFileExists
// and noErrorIsReturned which are registered there.
var ksState struct {
	store        *knownstate.Store
	storeBaseDir string // temp dir; relative paths in file assertions resolve here
	storePath    string // absolute path to backing JSON file

	sustainN    int  // WithSustainSuccess option
	autoPersist bool // WithAutoPersist option

	lastResult knownstate.UpdateResult
	lastErr    error

	// predicate steps
	lastStatusVal knownstate.Status
	lastBoolVal   bool

	// Get step
	getState knownstate.TargetState
	getFound bool

	// Snapshot mutation test
	snapshotBeforeMutation knownstate.Snapshot
}

func resetKSState() {
	if ksState.storeBaseDir != "" {
		os.RemoveAll(ksState.storeBaseDir)
	}
	ksState.store = nil
	ksState.storeBaseDir = ""
	ksState.storePath = ""
	ksState.sustainN = 1
	ksState.autoPersist = true
	ksState.lastResult = knownstate.UpdateResult{}
	ksState.lastErr = nil
	ksState.lastStatusVal = ""
	ksState.lastBoolVal = false
	ksState.getState = knownstate.TargetState{}
	ksState.getFound = false
	ksState.snapshotBeforeMutation = knownstate.Snapshot{}
}

func InitializeKnownStateSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) { resetKSState() })

	// ── setup ──────────────────────────────────────────────────────────────
	ctx.Step(`^a known-state store is initialised with path "([^"]*)"$`, aKnownStateStoreIsInitialisedWithPath)

	// ── status predicates ──────────────────────────────────────────────────
	ctx.Step(`^a status value "([^"]*)"$`, aStatusValue)
	ctx.Step(`^I call IsHealthy on the status$`, iCallIsHealthyOnTheStatus)
	ctx.Step(`^IsHealthy returns (\w+)$`, isHealthyReturns)
	ctx.Step(`^I call IsFailure on the status$`, iCallIsFailureOnTheStatus)
	ctx.Step(`^IsFailure returns (\w+)$`, isFailureReturnsKS)

	// ── sustain-success gate ───────────────────────────────────────────────
	ctx.Step(`^sustain_success is set to (\d+)$`, sustainSuccessIsSetTo)
	ctx.Step(`^(\d+) healthy probe result is recorded for target "([^"]*)"$`, nHealthyProbeResultsRecorded)
	ctx.Step(`^(\d+) consecutive healthy probe results are recorded for target "([^"]*)"$`, nConsecutiveHealthyProbeResultsRecorded)
	ctx.Step(`^(\d+) degraded probe result is recorded for target "([^"]*)"$`, nDegradedProbeResultRecorded)
	ctx.Step(`^(\d+) degraded probe results are recorded for target "([^"]*)"$`, nDegradedProbeResultsRecorded)
	ctx.Step(`^(\d+) consecutive healthy probe results have been recorded for target "([^"]*)"$`, nConsecutiveHealthyResultsHaveBeenRecorded)
	ctx.Step(`^(\d+) degraded probe results have been recorded for target "([^"]*)"$`, nDegradedProbeResultsHaveBeenRecorded)
	ctx.Step(`^the target "([^"]*)" ever_healthy is (\w+)$`, theTargetEverHealthyIs)
	ctx.Step(`^the target "([^"]*)" last_healthy_at is set$`, theTargetLastHealthyAtIsSet)
	ctx.Step(`^the target "([^"]*)" success_streak is (\d+)$`, theTargetSuccessStreakIs)

	// ── regression classification ──────────────────────────────────────────
	ctx.Step(`^the store has target "([^"]*)" with ever_healthy (\w+)$`, theStoreHasTargetWithEverHealthy)
	ctx.Step(`^a degraded probe result has been recorded for target "([^"]*)"$`, aDegradedProbeResultHasBeenRecorded)
	ctx.Step(`^a healthy probe result has been recorded for target "([^"]*)"$`, aHealthyProbeResultHasBeenRecorded)
	ctx.Step(`^a healthy probe result is recorded for target "([^"]*)"$`, aHealthyProbeResultRecordedForTarget)
	ctx.Step(`^a degraded probe result is recorded for target "([^"]*)"$`, aDegradedProbeResultRecordedForTarget)
	ctx.Step(`^a "([^"]*)" probe result is recorded for target "([^"]*)"$`, aStatusProbeResultIsRecordedForTarget)
	ctx.Step(`^the update result is_regression is (\w+)$`, theUpdateResultIsRegressionIs)
	ctx.Step(`^the update result became_healthy is (\w+)$`, theUpdateResultBecameHealthyIs)
	ctx.Step(`^the update result became_unhealthy is (\w+)$`, theUpdateResultBecameUnhealthyIs)

	// ── transition fields ──────────────────────────────────────────────────
	ctx.Step(`^the store has no entry for target "([^"]*)"$`, theStoreHasNoEntryForTarget)
	ctx.Step(`^the update result had_previous is (\w+)$`, theUpdateResultHadPreviousIs)

	// ── consecutive failures / streaks ─────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" consecutive_failures is (\d+)$`, theTargetConsecutiveFailuresIs)
	ctx.Step(`^the target "([^"]*)" current_status is "([^"]*)"$`, theTargetCurrentStatusIs)

	// ── empty target ID / empty status ─────────────────────────────────────
	ctx.Step(`^a probe result is recorded for target "([^"]*)"$`, aProbeResultIsRecordedForTarget)
	ctx.Step(`^a probe result with no status is recorded for target "([^"]*)"$`, aProbeResultWithNoStatusIsRecorded)
	ctx.Step(`^an error is returned containing "([^"]*)"$`, anErrorIsReturnedContaining)

	// ── Get ────────────────────────────────────────────────────────────────
	ctx.Step(`^I call Get for target "([^"]*)"$`, iCallGetForTarget)
	ctx.Step(`^the returned state ever_healthy matches the stored value$`, theReturnedStateEverHealthyMatchesStoredValue)
	ctx.Step(`^the returned found flag is false$`, theReturnedFoundFlagIsFalse)

	// ── persistence ────────────────────────────────────────────────────────
	ctx.Step(`^I call Save explicitly$`, iCallSaveExplicitly)
	ctx.Step(`^no "([^"]*)" file remains$`, noFileRemains)
	ctx.Step(`^the store is initialised with auto_persist (\w+)$`, theStoreIsInitialisedWithAutoPersist)
	ctx.Step(`^a snapshot file exists at "([^"]*)" with target "([^"]*)" marked ever_healthy$`, aSnapshotFileExistsWithTargetMarkedEverHealthy)
	ctx.Step(`^no file exists at "([^"]*)"$`, noFileExistsAt)
	ctx.Step(`^the store loads from disk$`, theStoreLoadsFromDisk)
	ctx.Step(`^the target map is empty$`, theTargetMapIsEmpty)
	ctx.Step(`^I call Snapshot and mutate the returned map$`, iCallSnapshotAndMutateReturnedMap)
	ctx.Step(`^the store's internal target "([^"]*)" is unchanged$`, theStoreInternalTargetIsUnchanged)

	// ── reset ──────────────────────────────────────────────────────────────
	ctx.Step(`^I call Reset with delete_file (\w+)$`, iCallResetWithDeleteFile)
	ctx.Step(`^the file "([^"]*)" exists with an empty targets map$`, theFileExistsWithEmptyTargetsMap)
	ctx.Step(`^the file "([^"]*)" does not exist$`, theFileDoesNotExist)
}

// ── store lifecycle ────────────────────────────────────────────────────────

func aKnownStateStoreIsInitialisedWithPath(path string) error {
	var err error
	ksState.storeBaseDir, err = os.MkdirTemp("", "ks-test-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	ksState.storePath = filepath.Join(ksState.storeBaseDir, path)
	ksState.sustainN = 1
	ksState.autoPersist = true
	ksState.store = knownstate.NewStore(ksState.storePath,
		knownstate.WithAutoPersist(true),
		knownstate.WithSustainSuccess(1),
	)
	return nil
}

func rebuildStore() {
	ksState.store = knownstate.NewStore(ksState.storePath,
		knownstate.WithAutoPersist(ksState.autoPersist),
		knownstate.WithSustainSuccess(ksState.sustainN),
	)
}

// ── status predicates ──────────────────────────────────────────────────────

func aStatusValue(status string) error {
	ksState.lastStatusVal = knownstate.Status(status)
	return nil
}

func iCallIsHealthyOnTheStatus() error {
	ksState.lastBoolVal = knownstate.IsHealthy(ksState.lastStatusVal)
	return nil
}

func isHealthyReturns(expected string) error {
	want := expected == "true"
	if ksState.lastBoolVal != want {
		return fmt.Errorf("IsHealthy(%q) = %v, want %v", ksState.lastStatusVal, ksState.lastBoolVal, want)
	}
	return nil
}

func iCallIsFailureOnTheStatus() error {
	ksState.lastBoolVal = knownstate.IsFailure(ksState.lastStatusVal)
	return nil
}

func isFailureReturnsKS(expected string) error {
	want := expected == "true"
	if ksState.lastBoolVal != want {
		return fmt.Errorf("IsFailure(%q) = %v, want %v", ksState.lastStatusVal, ksState.lastBoolVal, want)
	}
	return nil
}

// ── probe result recording ─────────────────────────────────────────────────

func recordN(n int, id string, status knownstate.Status) error {
	for i := 0; i < n; i++ {
		r, err := ksState.store.Update(context.Background(), knownstate.UpdateInput{
			TargetID: id,
			Status:   status,
		})
		if err != nil {
			ksState.lastErr = err
			return err
		}
		ksState.lastResult = r
		ksState.lastErr = nil
	}
	return nil
}

func sustainSuccessIsSetTo(n int) error {
	ksState.sustainN = n
	rebuildStore()
	return nil
}

func nHealthyProbeResultsRecorded(n int, id string) error {
	return recordN(n, id, knownstate.StatusHealthy)
}

func nConsecutiveHealthyProbeResultsRecorded(n int, id string) error {
	return recordN(n, id, knownstate.StatusHealthy)
}

func nDegradedProbeResultRecorded(n int, id string) error {
	return recordN(n, id, knownstate.StatusDegraded)
}

func nDegradedProbeResultsRecorded(n int, id string) error {
	return recordN(n, id, knownstate.StatusDegraded)
}

func nConsecutiveHealthyResultsHaveBeenRecorded(n int, id string) error {
	return recordN(n, id, knownstate.StatusHealthy)
}

func nDegradedProbeResultsHaveBeenRecorded(n int, id string) error {
	return recordN(n, id, knownstate.StatusDegraded)
}

func aDegradedProbeResultHasBeenRecorded(id string) error {
	return recordN(1, id, knownstate.StatusDegraded)
}

func aHealthyProbeResultHasBeenRecorded(id string) error {
	return recordN(1, id, knownstate.StatusHealthy)
}

// "When" variants — update ksState and capture lastResult for transition assertions.
func aHealthyProbeResultRecordedForTarget(id string) error {
	r, err := ksState.store.Update(context.Background(), knownstate.UpdateInput{
		TargetID: id,
		Status:   knownstate.StatusHealthy,
	})
	ksState.lastResult = r
	ksState.lastErr = err
	return nil
}

func aDegradedProbeResultRecordedForTarget(id string) error {
	r, err := ksState.store.Update(context.Background(), knownstate.UpdateInput{
		TargetID: id,
		Status:   knownstate.StatusDegraded,
	})
	ksState.lastResult = r
	ksState.lastErr = err
	return nil
}

func aStatusProbeResultIsRecordedForTarget(status, id string) error {
	r, err := ksState.store.Update(context.Background(), knownstate.UpdateInput{
		TargetID: id,
		Status:   knownstate.Status(status),
	})
	ksState.lastResult = r
	ksState.lastErr = err
	return nil // don't propagate; let assertion step check ksState.lastErr
}

func aProbeResultIsRecordedForTarget(id string) error {
	// Used for the "empty target ID" test — id may be "".
	r, err := ksState.store.Update(context.Background(), knownstate.UpdateInput{
		TargetID: id,
		Status:   knownstate.StatusHealthy,
	})
	ksState.lastResult = r
	ksState.lastErr = err
	return nil
}

func aProbeResultWithNoStatusIsRecorded(id string) error {
	r, err := ksState.store.Update(context.Background(), knownstate.UpdateInput{
		TargetID: id,
		// Status intentionally empty — store normalises to unknown.
	})
	ksState.lastResult = r
	ksState.lastErr = err
	return nil
}

// ── store state setup ──────────────────────────────────────────────────────

func theStoreHasTargetWithEverHealthy(id, everHealthy string) error {
	if everHealthy == "true" {
		// sustainN defaults to 1 so one healthy result sets EverHealthy.
		return recordN(1, id, knownstate.StatusHealthy)
	}
	// Target exists but EverHealthy stays false.
	return recordN(1, id, knownstate.StatusDegraded)
}

func theStoreHasNoEntryForTarget(_ string) error {
	// Default state: no entry in store. No-op.
	return nil
}

// ── target state assertions ────────────────────────────────────────────────

func getTargetKS(id string) (knownstate.TargetState, error) {
	ts, ok := ksState.store.Get(id)
	if !ok {
		return knownstate.TargetState{}, fmt.Errorf("target %q not found in store", id)
	}
	return ts, nil
}

func theTargetEverHealthyIs(id, expected string) error {
	want := expected == "true"
	ts, ok := ksState.store.Get(id)
	if !ok {
		// Non-existent target has EverHealthy=false (zero value).
		if !want {
			return nil
		}
		return fmt.Errorf("target %q not found in store (ever_healthy = false, want true)", id)
	}
	if ts.EverHealthy != want {
		return fmt.Errorf("target %q ever_healthy = %v, want %v", id, ts.EverHealthy, want)
	}
	return nil
}

func theTargetLastHealthyAtIsSet(id string) error {
	ts, err := getTargetKS(id)
	if err != nil {
		return err
	}
	if ts.LastHealthyAt.IsZero() {
		return fmt.Errorf("target %q last_healthy_at is not set", id)
	}
	return nil
}

func theTargetSuccessStreakIs(id string, n int) error {
	ts, err := getTargetKS(id)
	if err != nil {
		return err
	}
	if ts.SuccessStreak != n {
		return fmt.Errorf("target %q success_streak = %d, want %d", id, ts.SuccessStreak, n)
	}
	return nil
}

func theTargetConsecutiveFailuresIs(id string, n int) error {
	ts, ok := ksState.store.Get(id)
	if !ok {
		if n == 0 {
			return nil // no entry ⇒ 0 failures
		}
		return fmt.Errorf("target %q not found in store", id)
	}
	if ts.ConsecutiveFailures != n {
		return fmt.Errorf("target %q consecutive_failures = %d, want %d", id, ts.ConsecutiveFailures, n)
	}
	return nil
}

func theTargetCurrentStatusIs(id, status string) error {
	ts, err := getTargetKS(id)
	if err != nil {
		return err
	}
	if string(ts.CurrentStatus) != status {
		return fmt.Errorf("target %q current_status = %q, want %q", id, ts.CurrentStatus, status)
	}
	return nil
}

// ── update result assertions ───────────────────────────────────────────────

func theUpdateResultIsRegressionIs(expected string) error {
	got, want := ksState.lastResult.IsRegression, expected == "true"
	if got != want {
		return fmt.Errorf("update result is_regression = %v, want %v", got, want)
	}
	return nil
}

func theUpdateResultBecameHealthyIs(expected string) error {
	got, want := ksState.lastResult.BecameHealthy, expected == "true"
	if got != want {
		return fmt.Errorf("update result became_healthy = %v, want %v", got, want)
	}
	return nil
}

func theUpdateResultBecameUnhealthyIs(expected string) error {
	got, want := ksState.lastResult.BecameUnhealthy, expected == "true"
	if got != want {
		return fmt.Errorf("update result became_unhealthy = %v, want %v", got, want)
	}
	return nil
}

func theUpdateResultHadPreviousIs(expected string) error {
	got, want := ksState.lastResult.HadPrevious, expected == "true"
	if got != want {
		return fmt.Errorf("update result had_previous = %v, want %v", got, want)
	}
	return nil
}

// ── error assertions ───────────────────────────────────────────────────────

func anErrorIsReturnedContaining(substr string) error {
	if ksState.lastErr == nil {
		return fmt.Errorf("expected an error containing %q, got nil", substr)
	}
	if !strings.Contains(ksState.lastErr.Error(), substr) {
		return fmt.Errorf("error %q does not contain %q", ksState.lastErr.Error(), substr)
	}
	return nil
}

// ── Get ────────────────────────────────────────────────────────────────────

func iCallGetForTarget(id string) error {
	ksState.getState, ksState.getFound = ksState.store.Get(id)
	return nil
}

func theReturnedStateEverHealthyMatchesStoredValue() error {
	ts, ok := ksState.store.Get(ksState.getState.TargetID)
	if !ok {
		return fmt.Errorf("target not found in store after Get")
	}
	if ksState.getState.EverHealthy != ts.EverHealthy {
		return fmt.Errorf("returned ever_healthy %v does not match store value %v",
			ksState.getState.EverHealthy, ts.EverHealthy)
	}
	return nil
}

func theReturnedFoundFlagIsFalse() error {
	if ksState.getFound {
		return fmt.Errorf("expected found=false but got true")
	}
	return nil
}

// ── persistence ────────────────────────────────────────────────────────────

func iCallSaveExplicitly() error {
	ksState.lastErr = ksState.store.Save(context.Background())
	return ksState.lastErr
}

func noFileRemains(path string) error {
	abs := resolveTestFilePath(path)
	if _, err := os.Stat(abs); err == nil {
		return fmt.Errorf("file %q still exists but should be absent", abs)
	}
	return nil
}

func theStoreIsInitialisedWithAutoPersist(v string) error {
	ksState.autoPersist = v == "true"
	rebuildStore()
	return nil
}

func aSnapshotFileExistsWithTargetMarkedEverHealthy(path, id string) error {
	abs := resolveTestFilePath(path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	snap := knownstate.Snapshot{
		SchemaVersion: 1,
		Targets: map[string]knownstate.TargetState{
			id: {TargetID: id, EverHealthy: true, CurrentStatus: knownstate.StatusHealthy},
		},
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(abs, b, 0o600)
}

func noFileExistsAt(path string) error {
	abs := resolveTestFilePath(path)
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %q: %w", abs, err)
	}
	return nil
}

func theStoreLoadsFromDisk() error {
	ksState.lastErr = ksState.store.Load(context.Background())
	return nil
}

func theTargetMapIsEmpty() error {
	snap := ksState.store.Snapshot()
	if len(snap.Targets) != 0 {
		return fmt.Errorf("expected empty target map, got %d entries", len(snap.Targets))
	}
	return nil
}

func iCallSnapshotAndMutateReturnedMap() error {
	ksState.snapshotBeforeMutation = ksState.store.Snapshot()
	// Get another copy and mutate it to prove isolation.
	snap := ksState.store.Snapshot()
	for k := range snap.Targets {
		ts := snap.Targets[k]
		ts.EverHealthy = !ts.EverHealthy
		snap.Targets[k] = ts
	}
	snap.Targets["__phantom__"] = knownstate.TargetState{TargetID: "__phantom__"}
	return nil
}

func theStoreInternalTargetIsUnchanged(id string) error {
	ts, ok := ksState.store.Get(id)
	if !ok {
		return fmt.Errorf("target %q not found in store", id)
	}
	before, ok := ksState.snapshotBeforeMutation.Targets[id]
	if !ok {
		return fmt.Errorf("target %q was not in pre-mutation snapshot", id)
	}
	if ts.EverHealthy != before.EverHealthy {
		return fmt.Errorf("store's internal ever_healthy changed: %v → %v", before.EverHealthy, ts.EverHealthy)
	}
	if _, phantom := ksState.store.Get("__phantom__"); phantom {
		return fmt.Errorf("phantom entry was inserted via Snapshot mutation (store is not copy-on-read)")
	}
	return nil
}

// ── reset ──────────────────────────────────────────────────────────────────

func iCallResetWithDeleteFile(deleteFile string) error {
	del := deleteFile == "true"
	err := ksState.store.Reset(context.Background(), del)
	ksState.lastErr = err
	return err
}

func theFileExistsWithEmptyTargetsMap(path string) error {
	abs := resolveTestFilePath(path)
	b, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("file %q not readable: %w", abs, err)
	}
	var snap knownstate.Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return fmt.Errorf("file %q is not valid JSON: %w", abs, err)
	}
	if len(snap.Targets) != 0 {
		return fmt.Errorf("file %q has %d targets, want 0", abs, len(snap.Targets))
	}
	return nil
}

func theFileDoesNotExist(path string) error {
	abs := resolveTestFilePath(path)
	if _, err := os.Stat(abs); err == nil {
		return fmt.Errorf("file %q exists but should be deleted", abs)
	}
	return nil
}
