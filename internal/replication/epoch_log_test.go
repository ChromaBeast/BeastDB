package replication

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/wal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestLeaderLogsEpochOnceAndReplicatesItAtTheSameLSN(t *testing.T) {
	ldir := t.TempDir()
	leader := startTNode(t, "leader", ldir, RoleLeader, "")
	marks, err := leader.engine.EpochMarks()
	if err != nil || len(marks) != 1 || marks[0].Epoch != 1 || marks[0].LSN != 1 {
		t.Fatalf("marks = %+v, err = %v; want one marker for epoch 1 at LSN 1", marks, err)
	}
	if err := leader.engine.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}

	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	caughtUp(t, f, leader)
	if got := fol.engine.CurrentLSN(); got != leader.engine.CurrentLSN() {
		t.Fatalf("follower LSN %d != leader %d: the marker drifted the logs apart", got, leader.engine.CurrentLSN())
	}
	if !waitUntil(t, func() bool { return fol.cluster.state.Get().EpochAt(2) == 1 }) {
		t.Fatalf("follower history lacks the marker: %+v", fol.cluster.state.Get().EpochStarts)
	}

	// A restart does not log the same epoch again.
	lsn := leader.engine.CurrentLSN()
	leader.stop()
	back := startTNode(t, "leader", ldir, RoleLeader, "")
	if back.engine.CurrentLSN() != lsn {
		t.Fatalf("restart moved the LSN from %d to %d", lsn, back.engine.CurrentLSN())
	}
}

// A node whose log predates epochs gets its marker after the old records;
// those stay at epoch 0 (unknown).
func TestUpgradedLogKeepsLegacyRecordsAtEpochZero(t *testing.T) {
	dir := t.TempDir()
	eng, err := api.NewEngine(filepath.Join(dir, "beast.bin"), filepath.Join(dir, "beast.wal"), 50)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 3; i++ {
		_ = eng.Put(i, []byte("v"))
	}
	_ = eng.Close()

	n := startTNode(t, "n", dir, RoleLeader, "")
	s := n.cluster.state.Get()
	if n.engine.CurrentLSN() != 4 || s.EpochAt(3) != 0 || s.EpochAt(4) != 1 {
		t.Fatalf("LSN %d, history %+v: want legacy records 1-3 at epoch 0 and the marker at LSN 4", n.engine.CurrentLSN(), s.EpochStarts)
	}
}

// The persisted history is only a cache of the WAL's markers: lose it and the
// next start rebuilds it without writing anything.
func TestEpochHistoryRebuiltFromWAL(t *testing.T) {
	dir := t.TempDir()
	n := startTNode(t, "n", dir, RoleLeader, "")
	for i := uint64(1); i <= 3; i++ {
		_ = n.engine.Put(i, []byte("v"))
	}
	n.stop()
	st, _ := LoadStateStore(dir)
	cur := st.Get()
	cur.EpochStarts = nil
	if err := writeAtomic(st.path, cur); err != nil {
		t.Fatal(err)
	}
	back := startTNode(t, "n", dir, RoleLeader, "")
	s := back.cluster.state.Get()
	if s.EpochAt(1) != 1 || s.EpochAt(4) != 1 {
		t.Fatalf("history not rebuilt from the WAL: %+v", s.EpochStarts)
	}
	if back.engine.CurrentLSN() != 4 {
		t.Fatalf("rebuilding history wrote records: LSN %d", back.engine.CurrentLSN())
	}
}

func TestPromotionLogsNewEpochAndCrashBeforeMarkerIsRepaired(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	fdir := t.TempDir()
	fol := startTNode(t, "r1", fdir, RoleFollower, "")
	f := fol.follow(leader.addr)
	_ = leader.engine.Put(1, []byte("a"))
	caughtUp(t, f, leader)

	res, err := fol.cluster.Promote(context.Background(), PromoteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := fol.cluster.state.Get()
	if s.EpochAt(res.AppliedLSN) != 2 || s.EpochAt(res.AppliedLSN-1) != 1 {
		t.Fatalf("epochs around promotion LSN %d: %+v", res.AppliedLSN, s.EpochStarts)
	}

	// Crash right after the epoch was persisted but before the marker: a node
	// restarted as leader at epoch 3 logs the marker then.
	cur := fol.cluster.state.Get()
	next := cur
	next.Epoch, next.ServedEpoch = 3, 3
	if err := fol.cluster.state.Set(next); err != nil {
		t.Fatal(err)
	}
	fol.stop()
	back := startTNode(t, "r1", fdir, RoleLeader, "")
	if got := back.cluster.state.Get().EpochAt(back.engine.CurrentLSN()); got != 3 {
		t.Fatalf("tail epoch after restart = %d, want 3", got)
	}
}

// The case the RPC epoch cannot catch: the old leader and the new leader end at
// the same LSN with different records. The epoch markers tell them apart.
func TestEqualLSNDivergenceIsRefused(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	_ = leader.engine.Put(1, []byte("a"))
	caughtUp(t, f, leader)

	leader.partition()
	if _, err := fol.cluster.Promote(context.Background(), PromoteOptions{AcceptDataLoss: true, FenceTimeout: 300 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	if err := leader.engine.Put(2, []byte("diverged")); err != nil { // old leader, LSN 3
		t.Fatal(err)
	}
	if leader.engine.CurrentLSN() != fol.engine.CurrentLSN() {
		t.Fatalf("setup: LSNs differ (%d vs %d); the test needs equal LSNs", leader.engine.CurrentLSN(), fol.engine.CurrentLSN())
	}

	conn, err := grpc.NewClient(fol.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	oldLog := leader.cluster.state.Get()
	last := leader.engine.CurrentLSN()
	stream, err := beastv1.NewReplicationServiceClient(conn).StreamWAL(ctx, &beastv1.StreamWALRequest{
		ReplicaId: "leader", FromLsn: last + 1, Epoch: 2, LastLogEpoch: oldLog.EpochAt(last)})
	if err == nil {
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.FailedPrecondition || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("StreamWAL err = %v, want FailedPrecondition mentioning divergence", err)
	}

	// A follower that really is a prefix is accepted.
	stream, err = beastv1.NewReplicationServiceClient(conn).StreamWAL(ctx, &beastv1.StreamWALRequest{
		ReplicaId: "ok", FromLsn: 3, Epoch: 2, LastLogEpoch: oldLog.EpochAt(2)})
	if err != nil {
		t.Fatal(err)
	}
	if m, err := stream.Recv(); err != nil || m.Lsn != 3 {
		t.Fatalf("prefix follower got %v, %v; want record 3", m, err)
	}
}

// A frame is judged against the follower's own log, not only the epoch the
// stream reports.
func TestFollowerRejectsStaleFramesByItsOwnLog(t *testing.T) {
	st, _ := LoadStateStore(t.TempDir())
	if err := st.MergeEpochMarks([]wal.EpochMark{{LSN: 1, Epoch: 1}, {LSN: 4, Epoch: 2}}); err != nil {
		t.Fatal(err)
	}
	f := &FollowerClient{state: st, lastAppliedLSN: 6}

	rec := func(epoch uint64, op byte, val []byte) *beastv1.WALRecordMessage {
		return &beastv1.WALRecordMessage{Lsn: 7, OpType: uint32(op), Epoch: epoch, Value: val}
	}
	if err := f.checkFrame(rec(2, wal.OpPut, nil)); err != nil {
		t.Fatalf("current-epoch frame refused: %v", err)
	}
	if err := f.checkFrame(rec(3, wal.OpEpoch, wal.EncodeEpoch(3))); err != nil {
		t.Fatalf("newer epoch marker refused: %v", err)
	}
	for name, m := range map[string]*beastv1.WALRecordMessage{
		"data from epoch 1":      rec(1, wal.OpPut, nil),
		"marker below the log":   rec(2, wal.OpEpoch, wal.EncodeEpoch(1)),
		"marker above its frame": rec(2, wal.OpEpoch, wal.EncodeEpoch(5)),
	} {
		if err := f.checkFrame(m); err == nil {
			t.Errorf("%s was accepted", name)
		} else if name == "data from epoch 1" && !errors.Is(err, ErrStaleLeader) {
			t.Errorf("%s: err = %v, want ErrStaleLeader", name, err)
		}
	}
}
