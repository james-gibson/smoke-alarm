// mock_stubborn_stdio_server is the server in issue #44's worst case: it
// completes a handshake and then refuses to exit when the client closes stdin.
//
// A real server can behave this way (a wedged thread, a blocking shutdown hook,
// a library that never sees EOF). The probe must still tear it down and return,
// so this mock deliberately holds stdout/stderr open for far longer than any
// probe deadline.
//
// If STUBBORN_PID_FILE is set, the pid is written there on startup so a test can
// assert that no process survived teardown.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
}

func main() {
	if path := os.Getenv("STUBBORN_PID_FILE"); path != "" {
		if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "write pid file: %v\n", err)
		}
	}

	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)

	for {
		line, err := in.ReadString('\n')
		if err != nil {
			break
		}
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if jsonErr := json.Unmarshal([]byte(line), &req); jsonErr != nil || len(req.ID) == 0 {
			// Notifications get no reply; garbage is ignored on purpose.
			continue
		}

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  map[string]any{},
		}
		body, marshalErr := json.Marshal(resp)
		if marshalErr != nil {
			continue
		}
		if _, writeErr := out.Write(append(body, '\n')); writeErr != nil {
			return
		}
		if flushErr := out.Flush(); flushErr != nil {
			return
		}
	}

	// stdin is closed: a well-behaved server would exit here. This one does not.
	time.Sleep(10 * time.Minute)
}
