package replication

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/api"
	"github.com/ChromaBeast/beastdb/internal/crypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// An encrypted leader replays its encrypted WAL for a follower that is behind
// (catch-up) and keeps streaming live commits.
func TestEncryptedLeaderCatchUpAndLive(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "k")
	if err := crypt.GenerateKeyFile(key); err != nil {
		t.Fatal(err)
	}
	leader, err := api.NewEngineWithOptions(filepath.Join(dir, "l.bin"), filepath.Join(dir, "l.wal"), 50, api.Options{KeyFile: key})
	if err != nil {
		t.Fatal(err)
	}
	defer leader.Close()
	rep := NewLeaderServer(leader.WALPath())
	rep.SetWALCipher(leader.WALCipher())
	srv := grpc.NewServer()
	rep.RegisterService(srv)
	leader.SetCommitObserver(rep)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	for _, k := range []uint64{1, 2, 3} {
		if err := leader.Put(k, []byte("before-follower")); err != nil {
			t.Fatal(err)
		}
	}
	follower, err := api.NewEngine(filepath.Join(dir, "f.bin"), filepath.Join(dir, "f.wal"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer follower.Close()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = NewFollowerClient("r1", follower, conn).SyncLoop(ctx) }()

	for _, k := range []uint64{1, 2, 3} {
		if !waitUntil(t, func() bool { v, ok, _ := follower.Get(k); return ok && bytes.Equal(v, []byte("before-follower")) }) {
			t.Fatalf("follower missed catch-up key %d", k)
		}
	}
	if err := leader.Put(99, []byte("live")); err != nil {
		t.Fatal(err)
	}
	if !waitUntil(t, func() bool { v, ok, _ := follower.Get(99); return ok && bytes.Equal(v, []byte("live")) }) {
		t.Fatal("follower missed live key")
	}
}
