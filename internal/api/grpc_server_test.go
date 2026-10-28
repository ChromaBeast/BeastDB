package api

import (
	"bytes"
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func setupTestGRPCServer(t testing.TB) (beastv1.BeastDBServiceClient, func()) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "grpc_test.bin")
	walPath := filepath.Join(tempDir, "grpc_test.wal")

	engine, err := NewEngine(dbPath, walPath, 50)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	server := NewGRPCServer(engine)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on local port: %v", err)
	}

	go func() {
		_ = server.Serve(lis)
	}()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("failed to connect to grpc server: %v", err)
	}

	client := beastv1.NewBeastDBServiceClient(conn)

	cleanup := func() {
		_ = conn.Close()
		server.Stop()
		_ = engine.Close()
	}

	return client, cleanup
}

func TestGRPCServerOperations(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Put keys
	keys := []uint64{100, 200, 300, 400}
	for _, k := range keys {
		val := []byte("payload-for-key")
		putResp, err := client.Put(ctx, &beastv1.PutRequest{Key: k, Value: val})
		if err != nil || !putResp.Success {
			t.Fatalf("Put failed for key %d: %v", k, err)
		}
	}

	// 2. Get existing key
	getResp, err := client.Get(ctx, &beastv1.GetRequest{Key: 200})
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if !getResp.Found || !bytes.Equal(getResp.Value, []byte("payload-for-key")) {
		t.Fatalf("unexpected Get response: %+v", getResp)
	}

	// 3. Get non-existent key
	missingResp, err := client.Get(ctx, &beastv1.GetRequest{Key: 9999})
	if err != nil {
		t.Fatalf("Get missing error: %v", err)
	}
	if missingResp.Found {
		t.Fatalf("expected missing key to not be found")
	}

	// 4. Server-streaming range scan: [150, 350] -> should return keys 200 and 300
	stream, err := client.Scan(ctx, &beastv1.ScanRequest{StartKey: 150, EndKey: 350})
	if err != nil {
		t.Fatalf("Scan initiation failed: %v", err)
	}

	var scannedKeys []uint64
	for {
		item, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Scan recv error: %v", err)
		}
		scannedKeys = append(scannedKeys, item.Key)
	}

	if len(scannedKeys) != 2 || scannedKeys[0] != 200 || scannedKeys[1] != 300 {
		t.Fatalf("unexpected scanned keys: %+v", scannedKeys)
	}

	// 5. Delete key 200
	delResp, err := client.Delete(ctx, &beastv1.DeleteRequest{Key: 200})
	if err != nil || !delResp.Success {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify key 200 is gone
	delGet, err := client.Get(ctx, &beastv1.GetRequest{Key: 200})
	if err != nil || delGet.Found {
		t.Fatalf("deleted key still found")
	}
}

func BenchmarkGRPCGet(b *testing.B) {
	client, cleanup := setupTestGRPCServer(b)
	defer cleanup()

	ctx := context.Background()
	_, _ = client.Put(ctx, &beastv1.PutRequest{Key: 42, Value: []byte("benchmark-value")})

	req := &beastv1.GetRequest{Key: 42}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.Get(ctx, req)
	}
}

// Values above gRPC's 4 MiB default, batched, must round-trip with the message-size options.
func TestGRPCLargeValuesWithMessageSizeOptions(t *testing.T) {
	dir := t.TempDir()
	engine, err := NewEngine(filepath.Join(dir, "big.bin"), filepath.Join(dir, "big.wal"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	server := NewGRPCServer(engine, MessageSizeServerOptions()...)
	lis, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() { _ = server.Serve(lis) }()
	defer server.Stop()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), MessageSizeDialOption())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := beastv1.NewBeastDBServiceClient(conn)
	ctx := context.Background()

	var ops []*beastv1.BatchOperation
	for k := uint64(1); k <= 3; k++ {
		ops = append(ops, &beastv1.BatchOperation{OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT, Key: k, Value: bytes.Repeat([]byte{byte(k)}, MaxValueSize)})
	}
	if _, err := client.BatchWrite(ctx, &beastv1.BatchWriteRequest{Operations: ops}); err != nil {
		t.Fatalf("6 MiB batch: %v", err)
	}
	got, err := client.Get(ctx, &beastv1.GetRequest{Key: 2})
	if err != nil || !bytes.Equal(got.Value, ops[1].Value) {
		t.Fatalf("get large value: %v", err)
	}
	stream, err := client.Scan(ctx, &beastv1.ScanRequest{StartKey: 1, EndKey: 3})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for {
		kv, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil || len(kv.Value) != MaxValueSize {
			t.Fatalf("scan large value: %v", err)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("scan returned %d values", n)
	}
}
