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

func filled(n int, b byte) []byte { return bytes.Repeat([]byte{b}, n) }

// Large values and multi-megabyte batches must replicate in catch-up and live modes.
func TestLargeValuesReplicate(t *testing.T) {
	dir := t.TempDir()
	leader, err := api.NewEngine(filepath.Join(dir, "l.bin"), filepath.Join(dir, "l.wal"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer leader.Close()
	rep := NewLeaderServer(leader.WALPath())
	srv := grpc.NewServer(api.MessageSizeServerOptions()...)
	rep.RegisterService(srv)
	leader.SetCommitObserver(rep)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	catchUp := filled(1500<<10, 'c')
	if err := leader.Put(1, catchUp); err != nil {
		t.Fatal(err)
	}

	follower, err := api.NewEngine(filepath.Join(dir, "f.bin"), filepath.Join(dir, "f.wal"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer follower.Close()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), api.MessageSizeDialOption())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = NewFollowerClient("big", follower, conn).SyncLoop(ctx) }()

	live := filled(api.MaxValueSize, 'l')
	batch := []api.BatchOperation{
		{Type: api.BatchOpPut, Key: 3, Value: filled(api.MaxValueSize, 'x')},
		{Type: api.BatchOpPut, Key: 4, Value: filled(api.MaxValueSize, 'y')},
		{Type: api.BatchOpPut, Key: 5, Value: filled(api.MaxValueSize, 'z')},
	}
	time.Sleep(100 * time.Millisecond)
	if err := leader.Put(2, live); err != nil {
		t.Fatal(err)
	}
	if err := leader.BatchWrite(batch); err != nil {
		t.Fatal(err)
	}

	want := map[uint64][]byte{1: catchUp, 2: live, 3: batch[0].Value, 4: batch[1].Value, 5: batch[2].Value}
	deadline := time.Now().Add(5 * time.Second)
	for k, v := range want {
		for {
			got, found, err := follower.Get(k)
			if err == nil && found && bytes.Equal(got, v) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("follower missing key %d: found=%v len=%d err=%v", k, found, len(got), err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
