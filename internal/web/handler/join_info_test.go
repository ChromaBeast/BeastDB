package handler

import (
	"strings"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/replication"
)

func TestBuildJoinInfo(t *testing.T) {
	t.Run("follower and fenced nodes offer nothing", func(t *testing.T) {
		for _, r := range []replication.Role{replication.RoleFollower, replication.RoleFenced} {
			j := buildJoinInfo(RuntimeSettings{GRPCAddr: "0.0.0.0:50051"}, r, "h:8080")
			if j.Available || j.Command != "" || !strings.Contains(j.Reason, "leader") {
				t.Errorf("%s: %+v", r, j)
			}
		}
	})
	t.Run("wildcard bind uses the host Studio was reached on", func(t *testing.T) {
		j := buildJoinInfo(RuntimeSettings{GRPCAddr: "0.0.0.0:50051"}, replication.RoleLeader, "db1.internal:8080")
		if !j.Available || j.LeaderAddr != "db1.internal:50051" || !strings.Contains(j.Command, "-leader-addr db1.internal:50051") {
			t.Fatalf("%+v", j)
		}
		if !strings.Contains(j.Command, "-role follower") || strings.Contains(j.Command, "-tls") {
			t.Fatalf("command: %s", j.Command)
		}
		if len(j.Notes) == 0 || !strings.Contains(j.Notes[0], "all interfaces") {
			t.Fatalf("notes: %v", j.Notes)
		}
	})
	t.Run("loopback leader says only local followers can join", func(t *testing.T) {
		j := buildJoinInfo(RuntimeSettings{GRPCAddr: "127.0.0.1:50051"}, replication.RoleLeader, "localhost:8080")
		if j.LeaderAddr != "127.0.0.1:50051" || !strings.Contains(strings.Join(j.Notes, " "), "loopback") || !strings.Contains(j.Command, "-port 50052") {
			t.Fatalf("%+v", j)
		}
	})
	t.Run("TLS and token settings change the command and notes", func(t *testing.T) {
		j := buildJoinInfo(RuntimeSettings{GRPCAddr: "10.0.0.5:50051", TLSEnabled: true, MutualTLS: true, APITokenConfigured: true}, replication.RoleLeader, "x")
		all := strings.Join(j.Notes, " ")
		if !strings.Contains(j.Command, "-tls-cert") || !strings.Contains(j.Command, "-tls-ca") || !strings.Contains(all, "mutual TLS") || !strings.Contains(j.Command, "-leader-token") || !strings.Contains(all, "bearer token") {
			t.Fatalf("%s\n%v", j.Command, j.Notes)
		}
		if j.LeaderAddr != "10.0.0.5:50051" {
			t.Fatal(j.LeaderAddr)
		}
	})
}
