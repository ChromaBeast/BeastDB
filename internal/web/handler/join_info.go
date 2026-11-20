package handler

import (
	"net"
	"strings"

	"github.com/ChromaBeast/beastdb/internal/replication"
)

// JoinInfo is what Studio shows for "Add replica". There is no join protocol:
// a follower is a process started with the right flags, and the leader learns
// of it when it opens a WAL stream. So this is only the command, built from
// this node's real settings, plus the caveats that apply to this node.
type JoinInfo struct {
	// Available is true only on a node that serves its WAL (the leader).
	Available bool `json:"available"`
	// Reason says why not, when Available is false.
	Reason string `json:"reason,omitempty"`
	// LeaderAddr is the address the command points followers at.
	LeaderAddr string `json:"leaderAddr,omitempty"`
	// Command is the full beastdb-server invocation, flags continued with a backslash.
	Command string `json:"command,omitempty"`
	// Notes are caveats specific to this node's configuration.
	Notes []string `json:"notes,omitempty"`
}

// SetNodeSettings gives the handler the node's start-up configuration so it can
// describe how to join a follower to it.
func (h *APIHandler) SetNodeSettings(s RuntimeSettings) { h.node = s }

// buildJoinInfo derives the follower command for a node. requestHost is the
// host the operator used to reach Studio (Host header): the best available
// guess at an address a follower can reach, and said so in the notes.
func buildJoinInfo(s RuntimeSettings, role replication.Role, requestHost string) JoinInfo {
	if role != replication.RoleLeader {
		return JoinInfo{Reason: "Followers are added to the leader. This node's role is " + string(role) + "; open Studio on the leader."}
	}
	host, port := splitHostPort(s.GRPCAddr)
	var notes []string
	switch {
	case host == "" || host == "0.0.0.0" || host == "::" || host == "[::]":
		host = hostOnly(requestHost)
		notes = append(notes, "The leader listens on all interfaces; the command uses the host you reached Studio on ("+host+"). Replace it with an address the follower can reach.")
	case host == "127.0.0.1" || host == "localhost" || host == "::1":
		notes = append(notes, "The leader listens on loopback only ("+s.GRPCAddr+"), so only a follower on this machine can connect. Start the leader with -bind-addr set to a reachable address to replicate across machines.")
	}
	if host == "" {
		host = "<leader-host>"
	}
	leader := net.JoinHostPort(host, port)

	lines := []string{
		"beastdb-server -role follower",
		"-leader-addr " + leader,
		"-leader-token <token>",
		"-replica-id <unique-name>",
		"-data-dir <empty-directory>",
		"-port " + otherPort(port),
		"-web-addr 127.0.0.1:8081",
	}
	if s.TLSEnabled {
		lines = append(lines, "-tls-cert <follower-cert.pem> -tls-key <follower-key.pem> -tls-ca <ca.pem>")
		notes = append(notes, "This leader serves gRPC over TLS: the follower needs the CA that signed the leader's certificate"+mutual(s.MutualTLS)+".")
	}
	if s.Encryption.Enabled {
		notes = append(notes, "This node is encrypted at rest. Followers keep their own setting and share no keys; add -encryption-key-file with a different key to encrypt the follower too.")
	}
	notes = append(notes,
		"The leader's gRPC port requires a bearer token. Give the follower one that is valid on this leader: an admin or replication-role token created under Tokens, or the leader's -api-token (a node's own -api-token is used when -leader-token is unset). It can also be set as BEASTDB_LEADER_TOKEN so it stays out of the process list.",
		"Use a data directory that is empty (a new follower copies the leader's WAL from the start) and a replica id no other follower uses.",
		"The follower appears in the table below once it connects. Nothing here starts it for you.")
	return JoinInfo{Available: true, LeaderAddr: leader, Command: strings.Join(lines, " \\\n  "), Notes: notes}
}

func mutual(on bool) string {
	if on {
		return ", and its own certificate (mutual TLS is required)"
	}
	return ""
}

func splitHostPort(addr string) (string, string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "50051"
	}
	return host, port
}

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return strings.Trim(h, "[]")
}

// otherPort suggests a gRPC port for the follower different from the leader's
// when both might share a machine.
func otherPort(leaderPort string) string {
	if leaderPort == "50052" {
		return "50053"
	}
	return "50052"
}
