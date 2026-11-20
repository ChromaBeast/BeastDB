package replication

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/wal"
)

func TestRejoinFencedNodeThatIsAPrefix(t *testing.T) {
	fastFencing(t)
	ldir := t.TempDir()
	leader := startTNode(t, "old", ldir, RoleLeader, "")
	fol := startTNode(t, "new", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	_ = leader.engine.Put(1, []byte("a"))
	caughtUp(t, f, leader)
	if _, err := fol.cluster.Promote(context.Background(), PromoteOptions{}); err != nil {
		t.Fatal(err)
	}
	if leader.cluster.Role() != RoleFenced {
		t.Fatal("setup: old leader should be fenced")
	}
	_ = fol.engine.Put(2, []byte("after"))

	res, err := leader.cluster.Rejoin(context.Background(), fol.addr)
	if err != nil {
		t.Fatalf("Rejoin: %v", err)
	}
	if res.Epoch != 2 || leader.cluster.Role() != RoleFollower {
		t.Fatalf("result %+v, role %s", res, leader.cluster.Role())
	}
	if !waitUntil(t, func() bool { return leader.hasKey(2, "after") }) {
		t.Fatal("rejoined node did not replicate the new leader's writes")
	}
	if err := leader.engine.Put(3, []byte("x")); !errors.Is(err, api.ErrReadOnlyReplica) {
		t.Fatalf("rejoined follower accepted a write: %v", err)
	}
	if !waitUntil(t, func() bool { return leader.cluster.state.Get().EpochAt(leader.engine.CurrentLSN()) == 2 }) {
		t.Fatal("follower did not log the new epoch")
	}

	// It stays a follower across a restart, and rejoin is single-use.
	leader.stop()
	back := startTNode(t, "old", ldir, RoleFollower, "")
	if back.cluster.Role() != RoleFollower || back.cluster.Epoch() != 2 {
		t.Fatalf("after restart: %s at epoch %d", back.cluster.Role(), back.cluster.Epoch())
	}
	if _, err := back.cluster.Rejoin(context.Background(), fol.addr); !errors.Is(err, ErrNotFenced) {
		t.Fatalf("second Rejoin err = %v, want ErrNotFenced", err)
	}
}

// The old leader took a write the new leader never saw (and ends at the same
// LSN, so only the epoch markers can tell). Rejoin must refuse, say so, and
// leave the data and the fence alone.
func TestRejoinRefusedWhenNodeHoldsUnreplicatedWrites(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "old", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "new", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	_ = leader.engine.Put(1, []byte("a"))
	caughtUp(t, f, leader)

	leader.partition()
	if _, err := fol.cluster.Promote(context.Background(), PromoteOptions{AcceptDataLoss: true, FenceTimeout: 300 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if err := leader.engine.Put(2, []byte("orphan")); err != nil {
		t.Fatal(err)
	}
	leader.heal()
	if !waitUntil(t, func() bool { return leader.cluster.Role() == RoleFenced }) {
		t.Fatal("setup: old leader not fenced")
	}

	_, err := leader.cluster.Rejoin(context.Background(), fol.addr)
	var refusal *RejoinRefusal
	if !errors.As(err, &refusal) || !errors.Is(err, ErrRejoinRefused) {
		t.Fatalf("Rejoin err = %v, want a RejoinRefusal", err)
	}
	if refusal.UnreplicatedRecords != 1 || !strings.Contains(refusal.Reason, "never on the leader") && !strings.Contains(refusal.Reason, "never had") {
		t.Fatalf("refusal does not say what is lost: %+v", refusal)
	}
	if leader.cluster.Role() != RoleFenced || !leader.hasKey(2, "orphan") {
		t.Fatal("a refused rejoin must leave the node fenced with its data intact")
	}
}

func TestRejoinRefusesNonLeaderTargetAndWrongRole(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "old", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "new", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	caughtUp(t, f, leader)

	if _, err := fol.cluster.Rejoin(context.Background(), leader.addr); !errors.Is(err, ErrNotFenced) {
		t.Fatalf("follower Rejoin err = %v, want ErrNotFenced", err)
	}
	if _, err := fol.cluster.Promote(context.Background(), PromoteOptions{}); err != nil {
		t.Fatal(err)
	}
	// Pointing the fenced node at itself (it is fenced, not the leader).
	if _, err := leader.cluster.Rejoin(context.Background(), leader.addr); !errors.Is(err, ErrRejoinRefused) {
		t.Fatalf("Rejoin to a fenced node err = %v, want a refusal", err)
	}
}

func TestCheckPrefix(t *testing.T) {
	marks := func(m ...wal.EpochMark) State { return State{EpochStarts: m} }
	m := func(l, e uint64) wal.EpochMark { return wal.EpochMark{LSN: l, Epoch: e} }
	remote := []wal.EpochMark{m(1, 1), m(11, 2), m(30, 3)}
	cases := []struct {
		name   string
		st     State
		mine   uint64
		last   uint64
		lost   uint64 // 0: accepted
		reject bool
	}{
		{"empty log", State{}, 0, 40, 0, false},
		{"prefix inside epoch 1", marks(m(1, 1)), 10, 40, 0, false},
		{"prefix, node led epoch 2 and stopped early", marks(m(1, 1), m(11, 2)), 20, 40, 0, false},
		{"wrote past the end of epoch 1", marks(m(1, 1)), 14, 40, 4, true},
		{"epoch the leader never had", marks(m(1, 1), m(11, 5)), 20, 40, 10, true},
		{"same epoch, different start", marks(m(1, 1), m(12, 2)), 20, 40, 9, true},
		{"ahead of the leader", marks(m(1, 1), m(11, 2), m(30, 3)), 45, 40, 5, true},
		{"predates markers", State{}, 7, 40, 0, true},
	}
	for _, c := range cases {
		got := checkPrefix(c.st, c.mine, remote, c.last)
		if (got != nil) != c.reject {
			t.Errorf("%s: refusal = %v, reject expected %t", c.name, got, c.reject)
			continue
		}
		if got != nil && got.UnreplicatedRecords != c.lost {
			t.Errorf("%s: unreplicated = %d, want %d (%s)", c.name, got.UnreplicatedRecords, c.lost, got.Reason)
		}
	}
}
