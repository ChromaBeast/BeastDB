package replication

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockGapReplicationServer struct {
	beastv1.UnimplementedReplicationServiceServer
}

func (m *mockGapReplicationServer) StreamWAL(req *beastv1.StreamWALRequest, stream grpc.ServerStreamingServer[beastv1.WALRecordMessage]) error {
	// Send LSN 1
	k1 := []byte{1, 0, 0, 0, 0, 0, 0, 0}
	if err := stream.Send(&beastv1.WALRecordMessage{Lsn: 1, OpType: 1, Key: k1, Value: []byte("val1")}); err != nil {
		return err
	}
	// Intentionally skip LSN 2 and send LSN 3 (gap)
	k3 := []byte{3, 0, 0, 0, 0, 0, 0, 0}
	return stream.Send(&beastv1.WALRecordMessage{Lsn: 3, OpType: 1, Key: k3, Value: []byte("val3")})
}

func TestFollowerDetectsAndRejectsGap(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "follower.bin")
	walPath := filepath.Join(tempDir, "follower.wal")

	engine, err := api.NewEngine(dbPath, walPath, 10)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	beastv1.RegisterReplicationServiceServer(grpcServer, &mockGapReplicationServer{})
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to dial mock leader: %v", err)
	}
	defer conn.Close()

	follower := NewFollowerClient("test-replica", engine, conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = follower.SyncLoop(ctx)
	if err == nil {
		t.Fatalf("expected replication gap error, but got nil")
	}

	if !errors.Is(err, ErrReplicationGap) {
		t.Fatalf("expected ErrReplicationGap, got: %v", err)
	}
}
