package stepdefinitions

// federation_claim_helper_test.go — the second process in the
// "concurrent ClaimSlot calls from different processes do not double-claim a
// port" scenario.
//
// The slot lock is an O_EXCL lock file keyed on PID, so proving it needs a real
// second process: goroutines in one process cannot race it honestly. The step
// re-executes this test binary with -test.run=TestFederationClaimHelper and a
// FED_CLAIM_HELPER env marker; without the marker the test is a no-op so normal
// runs of the package are unaffected.

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/federation"
)

func TestFederationClaimHelper(t *testing.T) {
	if os.Getenv("FED_CLAIM_HELPER") != "1" {
		return
	}
	base, err := strconv.Atoi(os.Getenv("FED_CLAIM_BASE"))
	if err != nil {
		fmt.Printf("%s%v\n", fedClaimResultPrefix, map[string]string{"err": "bad FED_CLAIM_BASE"})
		return
	}
	maxPort, err := strconv.Atoi(os.Getenv("FED_CLAIM_MAX"))
	if err != nil {
		fmt.Printf("%s%v\n", fedClaimResultPrefix, map[string]string{"err": "bad FED_CLAIM_MAX"})
		return
	}

	result := fedClaimResult{}
	claim, claimErr := federation.ClaimSlot(federation.SlotOptions{
		BasePort:    base,
		MaxPort:     maxPort,
		StateDir:    os.Getenv("FED_CLAIM_STATE_DIR"),
		ServiceName: "ocd-smoke-alarm",
	})
	if claimErr != nil {
		result.Err = claimErr.Error()
	} else {
		result.Port = claim.Identity.Port
		result.Role = string(claim.Identity.Role)
		result.ID = claim.Identity.ID
	}

	data, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		fmt.Printf("%s%v\n", fedClaimResultPrefix, map[string]string{"err": marshalErr.Error()})
		return
	}
	fmt.Printf("%s%s\n", fedClaimResultPrefix, data)

	if claim != nil {
		// Hold the slot so the other process observes it as occupied.
		time.Sleep(300 * time.Millisecond)
		_ = claim.Close()
	}
}
