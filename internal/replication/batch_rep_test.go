package replication

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestAtomicBatchReplicationCompleteness(t *testing.T) {
	tempDir := t.TempDir()

	leaderDB := filepath.Join(tempDir, "leader.bin")
	leaderWAL := filepath.Join(tempDir, "leader.wal")
	leader, err := api.NewEngine(leaderDB, leaderWAL, 20)
	if err != nil {
		t.Fatalf("failed to create leader: %v", err)
	}
	defer leader.Close()

	leaderRepServer := NewLeaderServer(leader.WALPath())
	grpcServer := grpc.NewServer()
	leaderRepServer.RegisterService(grpcServer)
	leader.SetCommitObserver(leaderRepServer)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	go func() { _ = grpcServer.Serve(lis) }()
	defer grpcServer.Stop()

	followerDB := filepath.Join(tempDir, "follower.bin")
	followerWAL := filepath.Join(tempDir, "follower.wal")
	follower, err := api.NewEngine(followerDB, followerWAL, 20)
	if err != nil {
		t.Fatalf("failed to create follower: %v", err)
	}
	defer follower.Close()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	followerClient := NewFollowerClient("batch-test-replica", follower, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = followerClient.SyncLoop(ctx)
	}()

	// Execute an atomic batch on the leader containing 4 puts and 1 delete
	batchOps := []api.BatchOperation{
		{Type: api.BatchOpPut, Key: 101, Value: []byte("val-101")},
		{Type: api.BatchOpPut, Key: 102, Value: []byte("val-102")},
		{Type: api.BatchOpPut, Key: 103, Value: []byte("val-103")},
		{Type: api.BatchOpPut, Key: 104, Value: []byte("val-104")},
	}
	if err := leader.BatchWrite(batchOps); err != nil {
		t.Fatalf("batch write failed: %v", err)
	}

	// Wait for follower catch-up
	time.Sleep(150 * time.Millisecond)

	// Verify all 4 operations are present on the follower
	for _, op := range batchOps {
		val, found, err := follower.Get(op.Key)
		if err != nil || !found {
			t.Fatalf("follower missing batch record %d: found=%v, err=%v", op.Key, found, err)
		}
		if !bytes.Equal(val, op.Value) {
			t.Fatalf("batch record %d value mismatch: expected %s, got %s", op.Key, op.Value, val)
		}
	}
}

func TestLSNMonotonicityAcrossCheckpointAndRestart(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "mono.bin")
	walPath := filepath.Join(tempDir, "mono.wal")

	engine, err := api.NewEngine(dbPath, walPath, 20)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	for i := uint64(1); i <= 10; i++ {
		if err := engine.Put(i, []byte("val")); err != nil {
			t.Fatalf("put failed: %v", err)
		}
	}
	lsnBeforeCheckpoint := engine.CurrentLSN()
	if lsnBeforeCheckpoint != 10 {
		t.Fatalf("expected LSN 10, got %d", lsnBeforeCheckpoint)
	}

	// Checkpoint rotates WAL to .bak and creates empty active log
	if err := engine.Checkpoint(); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	_ = engine.Close()

	// Reopen engine from disk: active WAL is empty
	reopened, err := api.NewEngine(dbPath, walPath, 20)
	if err != nil {
		t.Fatalf("reopening engine failed: %v", err)
	}
	defer reopened.Close()

	if reopened.CurrentLSN() < lsnBeforeCheckpoint {
		t.Fatalf("LSN regressed after checkpoint and restart: was %d, reopened at %d",
			lsnBeforeCheckpoint, reopened.CurrentLSN())
	}

	// Next write must have LSN > lsnBeforeCheckpoint
	if err := reopened.Put(11, []byte("val-11")); err != nil {
		t.Fatalf("put 11 failed: %v", err)
	}

	if reopened.CurrentLSN() <= lsnBeforeCheckpoint {
		t.Fatalf("expected next LSN > %d, got %d", lsnBeforeCheckpoint, reopened.CurrentLSN())
	}
}
