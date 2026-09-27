package replication

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestReplicationCatchupConcurrentWrites verifies that writes committed during
// the replay-to-live handoff transition are seamlessly captured without drop or gap.
func TestReplicationCatchupConcurrentWrites(t *testing.T) {
	tempDir := t.TempDir()

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
	leaderEngine.SetCommitObserver(leaderRepServer)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen for leader: %v", err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	// Write 50 pre-existing records on leader
	for i := uint64(1); i <= 50; i++ {
		val := []byte(fmt.Sprintf("initial-%d", i))
		if err := leaderEngine.Put(i, val); err != nil {
			t.Fatalf("leader pre-write failed: %v", err)
		}
	}

	// Create follower node
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

	follower := NewFollowerClient("concurrent-replica", followerEngine, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	var followerErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := follower.SyncLoop(ctx); err != nil && !errors.Is(err, context.Canceled) {
			followerErr = err
		}
	}()

	// Concurrently write records 51 to 100 on leader while follower is catching up
	for i := uint64(51); i <= 100; i++ {
		val := []byte(fmt.Sprintf("concurrent-%d", i))
		if err := leaderEngine.Put(i, val); err != nil {
			t.Fatalf("leader concurrent put failed: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Wait for follower to catch up to LSN 100
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if follower.LastAppliedLSN() >= 100 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	wg.Wait()

	if followerErr != nil {
		t.Fatalf("follower sync loop failed: %v", followerErr)
	}

	if follower.LastAppliedLSN() < 100 {
		t.Fatalf("expected follower to apply at least 100 LSNs, got %d", follower.LastAppliedLSN())
	}

	// Validate all 100 records are accurately stored on follower
	for i := uint64(1); i <= 100; i++ {
		val, found, err := followerEngine.Get(i)
		if err != nil || !found {
			t.Fatalf("follower missing key %d (err: %v)", i, err)
		}
		var expected []byte
		if i <= 50 {
			expected = []byte(fmt.Sprintf("initial-%d", i))
		} else {
			expected = []byte(fmt.Sprintf("concurrent-%d", i))
		}
		if !bytes.Equal(val, expected) {
			t.Fatalf("key %d mismatch: expected %s, got %s", i, expected, val)
		}
	}
}
