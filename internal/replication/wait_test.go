package replication

import (
	"testing"
	"time"
)

// replicationDeadline bounds how long a test waits for the follower to apply
// what the leader committed. Each applied write is an fsync on the follower,
// which on a loaded CI runner can take far longer than on a laptop.
const replicationDeadline = 5 * time.Second

// waitUntil polls cond until it holds or the deadline passes, and reports
// whether it held. Use it instead of a fixed sleep before asserting on a follower.
func waitUntil(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(replicationDeadline)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}
