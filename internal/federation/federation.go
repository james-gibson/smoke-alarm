package federation

// federation.go — legacy helpers that survive the retired downstream/upstream
// poller design.
//
// The fleet is a star: one introducer (the process that wins base_port) serves
// membership and aggregates follower status; followers push status to it. There
// is no chain of downstream endpoints, so the Poller/ReportToUpstream pair that
// used to live here had no production caller and was removed (#38). Status
// propagation lives in status.go and client.go.

import (
	"net"
	"strconv"
)

// DetectRank derives the federation rank from a listen address. Rank predates
// slot claiming: the introducer is elected by port, not by rank, but configs
// still carry the field and operators read it in logs, so it is populated for
// round-tripping.
func DetectRank(listenAddr string) int {
	_, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(portStr)
	return port
}
