package replication

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestPromoteWithReachableLeaderLosesNoAcknowledgedWrite(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)

	// Writers run straight through the promotion. Every write the leader
	// acknowledged must exist on the new leader; writes refused after the
	// fence must not.
	var acked sync.Map
	var stop atomic.Bool
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				key := uint64(w*1_000_000 + i)
				if err := leader.engine.Put(key, []byte("v")); err == nil {
					acked.Store(key, true)
				} else if !errors.Is(err, api.ErrReadOnlyReplica) {
					t.Errorf("unexpected write error: %v", err)
					return
				}
			}
		}(w)
	}
	if !waitUntil(t, func() bool { return f.LastAppliedLSN() > 20 }) {
		t.Fatal("no replication before promotion")
	}

	res, err := fol.cluster.Promote(context.Background(), PromoteOptions{})
	stop.Store(true)
	wg.Wait()
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !res.OldLeaderFenced || res.DataLossPossible || res.Epoch != 2 {
		t.Fatalf("unexpected result %+v", res)
	}

	n := 0
	acked.Range(func(k, _ any) bool {
		n++
		if !fol.hasKey(k.(uint64), "v") {
			t.Errorf("acknowledged key %d missing on the new leader", k)
		}
		return true
	})
	if n == 0 {
		t.Fatal("test wrote nothing")
	}
	if got := fol.engine.CurrentLSN(); got < res.FinalLSN {
		t.Fatalf("new leader at LSN %d, behind the fenced leader's final %d", got, res.FinalLSN)
	}

	// Old leader refuses writes; new leader takes them.
	if err := leader.engine.Put(1, []byte("late")); !errors.Is(err, api.ErrReadOnlyReplica) {
		t.Fatalf("old leader write after fence: err = %v, want ErrReadOnlyReplica", err)
	}
	if err := fol.engine.Put(7, []byte("new-era")); err != nil {
		t.Fatalf("new leader write: %v", err)
	}
	if got := leader.cluster.Status().Role; got != RoleFenced {
		t.Fatalf("old leader role = %s, want fenced", got)
	}
	if got := fol.cluster.Status().Role; got != RoleLeader {
		t.Fatalf("new leader role = %s", got)
	}
	if fol.cluster.ServedEpoch() != 2 {
		t.Fatalf("new leader serves epoch %d, want 2", fol.cluster.ServedEpoch())
	}
}

func TestFenceIsIdempotentAndExclusive(t *testing.T) {
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	if err := leader.engine.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}
	lsn1, err := leader.cluster.fence(2, "r1")
	if err != nil {
		t.Fatal(err)
	}
	lsn2, err := leader.cluster.fence(2, "r1")
	if err != nil || lsn2 != lsn1 {
		t.Fatalf("same candidate retry: (%d, %v), want (%d, nil)", lsn2, err, lsn1)
	}
	// Two followers must never both be promoted at the same epoch.
	if _, err := leader.cluster.fence(2, "r2"); !errors.Is(err, ErrFenceRefused) {
		t.Fatalf("second candidate at same epoch: err = %v, want ErrFenceRefused", err)
	}
	if _, err := leader.cluster.fence(1, "r3"); !errors.Is(err, ErrFenceRefused) {
		t.Fatalf("stale epoch: err = %v, want ErrFenceRefused", err)
	}
}

func TestLeaderStepsDownWhenItSeesHigherEpoch(t *testing.T) {
	for _, via := range []string{"StreamWAL", "Ack"} {
		t.Run(via, func(t *testing.T) {
			dir := t.TempDir()
			leader := startTNode(t, "leader", dir, RoleLeader, "")
			if err := leader.engine.Put(1, []byte("a")); err != nil {
				t.Fatal(err)
			}
			conn, err := grpc.NewClient(leader.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			rc := beastv1.NewReplicationServiceClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if via == "Ack" {
				_, err = rc.Ack(ctx, &beastv1.ReplicationAck{ReplicaId: "peer", AcknowledgedLsn: 1, Epoch: 5})
			} else {
				var s grpc.ServerStreamingClient[beastv1.WALRecordMessage]
				if s, err = rc.StreamWAL(ctx, &beastv1.StreamWALRequest{ReplicaId: "peer", FromLsn: 1, Epoch: 5}); err == nil {
					_, err = s.Recv()
				}
			}
			if status.Code(err) != codes.FailedPrecondition {
				t.Fatalf("%s at higher epoch: err = %v, want FailedPrecondition", via, err)
			}
			if err := leader.engine.Put(2, []byte("b")); !errors.Is(err, api.ErrReadOnlyReplica) {
				t.Fatalf("write after step-down: err = %v, want ErrReadOnlyReplica", err)
			}
			st := leader.cluster.Status()
			if st.Role != RoleFenced || st.Epoch != 5 {
				t.Fatalf("status after step-down: %+v", st)
			}

			// The fence survives a restart: the old leader must not come back writable.
			leader.stop()
			re := startTNode(t, "leader", dir, RoleLeader, "")
			if re.cluster.Role() != RoleFenced {
				t.Fatalf("restarted role = %s, want fenced", re.cluster.Role())
			}
			if err := re.engine.Put(3, []byte("c")); !errors.Is(err, api.ErrReadOnlyReplica) {
				t.Fatalf("write after restart: err = %v, want ErrReadOnlyReplica", err)
			}
		})
	}
}

// staleLeader is a replication server that ignores the epoch in requests, as a
// partitioned old leader that has not yet heard of the promotion would.
type staleLeader struct {
	beastv1.UnimplementedReplicationServiceServer
	epoch uint64
}

func (s *staleLeader) StreamWAL(req *beastv1.StreamWALRequest, stream grpc.ServerStreamingServer[beastv1.WALRecordMessage]) error {
	if err := stream.Send(&beastv1.WALRecordMessage{Lsn: 1, OpType: 1, Key: []byte{1, 0, 0, 0, 0, 0, 0, 0}, Value: []byte("stale"), Epoch: s.epoch}); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

func TestFollowerRejectsStaleEpochLeader(t *testing.T) {
	dir := t.TempDir()
	fol := startTNode(t, "r1", dir, RoleFollower, "")
	if err := fol.cluster.state.Set(State{Epoch: 3, Role: RoleFollower}); err != nil {
		t.Fatal(err)
	}

	srv := grpc.NewServer()
	beastv1.RegisterReplicationServiceServer(srv, &staleLeader{epoch: 2})
	lis, addr := listen(t)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	f := NewFollowerClient("r1", fol.engine, conn)
	f.SetStateStore(fol.cluster.state)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.SyncLoop(ctx); !errors.Is(err, ErrStaleLeader) {
		t.Fatalf("SyncLoop err = %v, want ErrStaleLeader", err)
	}
	if fol.hasKey(1, "stale") || f.LastAppliedLSN() != 0 {
		t.Fatal("record from a stale-epoch leader was applied")
	}
	if fol.cluster.Epoch() != 3 {
		t.Fatalf("epoch moved to %d", fol.cluster.Epoch())
	}
}

func TestFollowerAdoptsAndPersistsNewerEpoch(t *testing.T) {
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	if err := leader.cluster.state.Set(State{Epoch: 4, Role: RoleLeader, ServedEpoch: 4}); err != nil {
		t.Fatal(err)
	}
	if err := leader.engine.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	fol := startTNode(t, "r1", dir, RoleFollower, "")
	fol.follow(leader.addr)
	if !waitUntil(t, func() bool { return fol.hasKey(1, "a") }) {
		t.Fatal("not replicated")
	}
	if fol.cluster.Epoch() != 4 {
		t.Fatalf("follower epoch = %d, want 4", fol.cluster.Epoch())
	}
	fol.stop()
	re, err := LoadStateStore(dir)
	if err != nil || re.Epoch() != 4 {
		t.Fatalf("persisted epoch = %v (%v), want 4", re.Epoch(), err)
	}
}

func TestPromoteRefusedWhenLeaderUnreachableUnlessDataLossAccepted(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	if err := leader.engine.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}
	caughtUp(t, f, leader)

	leader.partition()
	opts := PromoteOptions{FenceTimeout: 300 * time.Millisecond}
	if _, err := fol.cluster.Promote(context.Background(), opts); !errors.Is(err, ErrLeaderUnreachable) {
		t.Fatalf("Promote err = %v, want ErrLeaderUnreachable", err)
	}
	if st := fol.cluster.Status(); st.Role != RoleFollower || st.Epoch != 1 {
		t.Fatalf("refused promotion changed state: %+v", st)
	}
	if err := fol.engine.Put(2, []byte("x")); !errors.Is(err, api.ErrReadOnlyReplica) {
		t.Fatalf("follower accepted a write after refused promotion: %v", err)
	}

	opts.AcceptDataLoss = true
	res, err := fol.cluster.Promote(context.Background(), opts)
	if err != nil {
		t.Fatalf("forced Promote: %v", err)
	}
	if res.OldLeaderFenced || !res.DataLossPossible || res.Epoch != 2 {
		t.Fatalf("unexpected forced result %+v", res)
	}
	if err := fol.engine.Put(2, []byte("x")); err != nil {
		t.Fatalf("new leader write: %v", err)
	}
}

// The old leader is cut off, a follower is force-promoted, and the old leader
// keeps taking writes (the unavoidable split-brain window). When the partition
// heals the new leader fences it, with no operator action.
func TestPartitionHealFencesOldLeader(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	if err := leader.engine.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}
	caughtUp(t, f, leader)

	leader.partition()
	if _, err := fol.cluster.Promote(context.Background(), PromoteOptions{AcceptDataLoss: true, FenceTimeout: 300 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	// Still writable on the far side of the partition: this is the divergence the fence bounds.
	if err := leader.engine.Put(2, []byte("diverged")); err != nil {
		t.Fatalf("partitioned leader should still accept writes until fenced: %v", err)
	}

	leader.heal()
	if !waitUntil(t, func() bool { return leader.cluster.Role() == RoleFenced }) {
		t.Fatal("old leader was not fenced after the partition healed")
	}
	if err := leader.engine.Put(3, []byte("after-heal")); !errors.Is(err, api.ErrReadOnlyReplica) {
		t.Fatalf("old leader write after heal: err = %v, want ErrReadOnlyReplica", err)
	}
	if fol.hasKey(2, "diverged") {
		t.Fatal("new leader must not contain the old leader's partitioned write")
	}
}

// An old leader that crashed (never fenced) and restarts with -role leader at
// its old epoch is fenced as soon as the promoted node reaches it.
func TestOldLeaderRestartedAfterPromotionIsFenced(t *testing.T) {
	fastFencing(t)
	ldir := t.TempDir()
	leader := startTNode(t, "leader", ldir, RoleLeader, "")
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	if err := leader.engine.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}
	caughtUp(t, f, leader)

	addr := leader.addr
	leader.stop() // crash
	if _, err := fol.cluster.Promote(context.Background(), PromoteOptions{AcceptDataLoss: true, FenceTimeout: 300 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}

	back := startTNode(t, "leader", ldir, RoleLeader, addr)
	if !waitUntil(t, func() bool { return back.cluster.Role() == RoleFenced }) {
		t.Fatal("restarted old leader stayed a leader")
	}
	if err := back.engine.Put(9, []byte("x")); !errors.Is(err, api.ErrReadOnlyReplica) {
		t.Fatalf("restarted old leader accepted a write: %v", err)
	}
	// And it now refuses to be restarted as a follower on its stale data.
	back.stop()
	if _, err := NewClusterForTest(t, ldir, RoleFollower); err == nil {
		t.Fatal("fenced node must refuse to start as a follower without a rebuild")
	}
}

func TestPromotedNodeRestartedAsFollowerStaysLeader(t *testing.T) {
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	dir := t.TempDir()
	fol := startTNode(t, "r1", dir, RoleFollower, "")
	f := fol.follow(leader.addr)
	if err := leader.engine.Put(1, []byte("a")); err != nil {
		t.Fatal(err)
	}
	caughtUp(t, f, leader)
	if _, err := fol.cluster.Promote(context.Background(), PromoteOptions{}); err != nil {
		t.Fatal(err)
	}
	fol.stop()

	// The service unit still says -role follower; the persisted epoch wins.
	re := startTNode(t, "r1", dir, RoleFollower, "")
	if re.cluster.Role() != RoleLeader || re.cluster.Epoch() != 2 {
		t.Fatalf("restarted: role %s epoch %d, want leader at 2", re.cluster.Role(), re.cluster.Epoch())
	}
	if !re.hasKey(1, "a") {
		t.Fatal("data lost across restart")
	}
	if err := re.engine.Put(2, []byte("b")); err != nil {
		t.Fatalf("restarted leader should accept writes: %v", err)
	}
}

// Crash after the new epoch was persisted but before writes were opened: the
// node comes back as leader, never as a follower of the old leader.
func TestCrashBetweenPersistAndOpenRestartsAsLeader(t *testing.T) {
	dir := t.TempDir()
	fol := startTNode(t, "r1", dir, RoleFollower, "")
	if err := fol.cluster.state.Set(State{Epoch: 2, Role: RoleLeader, ServedEpoch: 2}); err != nil {
		t.Fatal(err)
	}
	fol.stop() // "crash": engine still read-only, state already says leader

	re := startTNode(t, "r1", dir, RoleFollower, "")
	if re.cluster.Role() != RoleLeader {
		t.Fatalf("role = %s, want leader", re.cluster.Role())
	}
	if err := re.engine.Put(1, []byte("a")); err != nil {
		t.Fatalf("write after recovery: %v", err)
	}
}

// A lagging follower can be promoted when the leader is reachable: the fence
// fixes the leader's final LSN and the follower waits to reach it.
func TestPromoteLaggingFollowerWaitsForFinalLSN(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	// Attached but not syncing yet: the follower is behind from the start.
	conn, err := grpc.NewClient(leader.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	f := NewFollowerClient("r1", fol.engine, conn)
	f.SetStateStore(fol.cluster.state)
	// In production a follower adopts the leader's epoch on first contact.
	if err := fol.cluster.state.Set(State{Epoch: 1, Role: RoleFollower}); err != nil {
		t.Fatal(err)
	}
	fol.cluster.mu.Lock()
	fol.cluster.follower = f
	fol.cluster.mu.Unlock()
	for i := uint64(1); i <= 50; i++ {
		if err := leader.engine.Put(i, []byte(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
	if st := fol.cluster.Status(); st.Promotable || st.PromoteBlockedReason == "" {
		t.Fatalf("a follower that has not heard from the leader must not look promotable: %+v", st)
	}

	// First attempt: leader fenced, but the follower cannot catch up in time.
	_, err = fol.cluster.Promote(context.Background(), PromoteOptions{CatchUpTimeout: 150 * time.Millisecond})
	if !errors.Is(err, ErrCatchUpTimeout) {
		t.Fatalf("Promote err = %v, want ErrCatchUpTimeout", err)
	}
	if st := fol.cluster.Status(); st.Role != RoleFollower {
		t.Fatalf("a failed catch-up must not promote: %+v", st)
	}
	if err := leader.engine.Put(99, []byte("z")); !errors.Is(err, api.ErrReadOnlyReplica) {
		t.Fatalf("leader stays fenced after a failed promotion attempt: %v", err)
	}

	// Retry with sync running: same candidate, so the fence is idempotent.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fol.cluster.RunFollower(ctx, f)
	res, err := fol.cluster.Promote(context.Background(), PromoteOptions{})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !res.OldLeaderFenced || res.AppliedLSN < 50 {
		t.Fatalf("unexpected result %+v", res)
	}
	for i := uint64(1); i <= 50; i++ {
		if !fol.hasKey(i, fmt.Sprint(i)) {
			t.Fatalf("key %d missing after promotion", i)
		}
	}
}

func TestStatusPromotableWhenCaughtUp(t *testing.T) {
	fastFencing(t)
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	f := fol.follow(leader.addr)
	for i := uint64(1); i <= 20; i++ {
		_ = leader.engine.Put(i, []byte("v"))
	}
	if !waitUntil(t, func() bool { return fol.cluster.Status().Promotable }) {
		t.Fatalf("a caught-up follower with a live leader should be promotable: %+v", fol.cluster.Status())
	}
	if st := leader.cluster.Status(); st.Promotable || st.PromoteBlockedReason == "" {
		t.Fatalf("a leader is not promotable and must say why: %+v", st)
	}

	// Silence from the leader makes the follower non-promotable and says so.
	leader.partition()
	if !waitUntil(t, func() bool { return !fol.cluster.Status().Promotable }) {
		t.Fatal("follower still promotable after leader went silent")
	}
	if st := fol.cluster.Status(); st.PromoteBlockedReason == "" {
		t.Fatalf("no reason given: %+v", st)
	}
	_ = f
}

func TestLeaderRefusesFollowerAheadOfIt(t *testing.T) {
	leader := startTNode(t, "leader", t.TempDir(), RoleLeader, "")
	_ = leader.engine.Put(1, []byte("a"))
	conn, err := grpc.NewClient(leader.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := beastv1.NewReplicationServiceClient(conn).StreamWAL(ctx, &beastv1.StreamWALRequest{ReplicaId: "old", FromLsn: 500})
	if err == nil {
		_, err = s.Recv()
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("err = %v, want FailedPrecondition for a follower ahead of the leader", err)
	}
}

func TestInactiveNodeServesNoWAL(t *testing.T) {
	fol := startTNode(t, "r1", t.TempDir(), RoleFollower, "")
	conn, err := grpc.NewClient(fol.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rc := beastv1.NewReplicationServiceClient(conn)
	if _, err := rc.Fence(ctx, &beastv1.FenceRequest{Epoch: 9, CandidateId: "x"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("Fence on a follower: err = %v, want FailedPrecondition", err)
	}
	if fol.cluster.Epoch() != 0 {
		t.Fatal("a follower must not change epoch because it was asked to fence")
	}
}
