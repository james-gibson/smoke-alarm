package stepdefinitions

// federation_claim_step_test.go — the spawn/parse half of the inter-process
// slot-lock scenario. The child side lives in federation_claim_helper_test.go.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func fedRunClaimHelper(stateDir string, base, maxPort int) (fedClaimResult, error) {
	cmd := exec.Command(os.Args[0], "-test.run=TestFederationClaimHelper", "-test.timeout=30s")
	cmd.Env = append(os.Environ(),
		"FED_CLAIM_HELPER=1",
		"FED_CLAIM_BASE="+strconv.Itoa(base),
		"FED_CLAIM_MAX="+strconv.Itoa(maxPort),
		"FED_CLAIM_STATE_DIR="+stateDir,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()

	for _, line := range strings.Split(out.String(), "\n") {
		idx := strings.Index(line, fedClaimResultPrefix)
		if idx < 0 {
			continue
		}
		var result fedClaimResult
		if err := json.Unmarshal([]byte(line[idx+len(fedClaimResultPrefix):]), &result); err != nil {
			return fedClaimResult{}, fmt.Errorf("decode helper result %q: %w", line, err)
		}
		return result, nil
	}

	if runErr != nil {
		return fedClaimResult{}, fmt.Errorf("helper process failed: %w\noutput: %s", runErr, out.String())
	}
	return fedClaimResult{}, fmt.Errorf("helper process produced no result\noutput: %s", out.String())
}
