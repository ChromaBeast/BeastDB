package replication

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/wal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestLeaderFollowerReplication(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Setup Leader Node
	leaderDB := filepath.Join(tempDir, "leader.bin")
	leaderWAL := filepath.Join(tempDir, "leader.wal")
	leaderEngine, err := api.NewEngine(leaderDB, leaderWAL, 50)
	if err != nil {
		t.Fatalf("failed to create leader engine: %v", err)
	}
	defer leaderEngine.Close()

	leaderRepServer := NewLeaderServer(leaderEngine.WALPath())
	grpcServer := grpc.NewServer()
	leaderRepServer.RegisterService(grpcServer)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for leader: %v", err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	// Insert 3 initial keys on the Leader before follower starts
	initialKeys := []uint64{10, 20, 30}
	for _, k := range initialKeys {
		if err := leaderEngine.Put(k, []byte("val-initial")); err != nil {
			t.Fatalf("leader put failed: %v", err)
		}
	}

	// 2. Setup Follower Node
	followerDB := filepath.Join(tempDir, "follower.bin")
	followerWAL := filepath.Join(tempDir, "follower.wal")
	followerEngine, err := api.NewEngine(followerDB, followerWAL, 50)
	if err != nil {
		t.Fatalf("failed to create follower engine: %v", err)
	}
	defer followerEngine.Close()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("follower failed to dial leader: %v", err)
	}
	defer conn.Close()

	follower := NewFollowerClient("replica-node-1", followerEngine, conn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = follower.SyncLoop(ctx)
	}()

	// Wait for catch-up replication of initial keys
	time.Sleep(100 * time.Millisecond)

	for _, k := range initialKeys {
		val, found, err := followerEngine.Get(k)
		if err != nil || !found || !bytes.Equal(val, []byte("val-initial")) {
			t.Fatalf("follower missing replayed key %d: found=%v, err=%v", k, found, err)
		}
	}

	// 3. Test Real-time streaming replication
	liveKey := uint64(999)
	liveVal := []byte("val-live-replicated")
	if err := leaderEngine.Put(liveKey, liveVal); err != nil {
		t.Fatalf("leader live put failed: %v", err)
	}

	var liveKeyBytes [8]byte
	binary.LittleEndian.PutUint64(liveKeyBytes[:], liveKey)
	leaderRepServer.Broadcast(leaderEngine.CurrentLSN(), wal.OpPut, liveKeyBytes[:], liveVal)

	time.Sleep(100 * time.Millisecond)

	val, found, err := followerEngine.Get(liveKey)
	if err != nil || !found || !bytes.Equal(val, liveVal) {
		t.Fatalf("follower failed to receive live replicated key 999: found=%v, err=%v", found, err)
	}

	// 4. Test range scan on read-replica
	cursor, err := followerEngine.Scan(10, 30)
	if err != nil {
		t.Fatalf("follower scan failed: %v", err)
	}
	defer cursor.Close()

	var scanned []uint64
	for {
		k, _, ok, _ := cursor.Next()
		if !ok {
			break
		}
		scanned = append(scanned, k)
	}
	if len(scanned) != 3 || scanned[0] != 10 || scanned[2] != 30 {
		t.Fatalf("unexpected follower scan results: %+v", scanned)
	}

	// 5. Test Delete replication
	_ = leaderEngine.Delete(20)
	var delKeyBytes [8]byte
	binary.LittleEndian.PutUint64(delKeyBytes[:], 20)
	leaderRepServer.Broadcast(leaderEngine.CurrentLSN(), wal.OpDelete, delKeyBytes[:], nil)

	time.Sleep(100 * time.Millisecond)

	_, found20, _ := followerEngine.Get(20)
	if found20 {
		t.Fatalf("expected deleted key 20 to be gone from replica")
	}

	// Verify Leader tracked the follower's Ack
	ackLSN := leaderRepServer.GetReplicaAck("replica-node-1")
	if ackLSN == 0 {
		t.Fatalf("expected non-zero replica ACK LSN on leader")
	}
}
