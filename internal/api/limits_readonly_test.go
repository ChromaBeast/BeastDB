package api

import (
	"bytes"
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestEngineRecordSizeLimit(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "size_test.bin")
	walPath := filepath.Join(tempDir, "size_test.wal")

	engine, err := NewEngine(dbPath, walPath, 20)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	// Exactly 4000 bytes should succeed
	validPayload := bytes.Repeat([]byte("A"), 4000)
	if err := engine.Put(1, validPayload); err != nil {
		t.Fatalf("expected 4000 byte put to succeed, got: %v", err)
	}

	// 4001 bytes must fail with ErrRecordTooLarge
	tooLargePayload := bytes.Repeat([]byte("B"), 4001)
	err = engine.Put(2, tooLargePayload)
	if !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("expected ErrRecordTooLarge for 4001 bytes, got: %v", err)
	}

	// BatchWrite with oversized record must also fail
	err = engine.BatchWrite([]BatchOperation{
		{Type: BatchOpPut, Key: 3, Value: tooLargePayload},
	})
	if !errors.Is(err, ErrRecordTooLarge) {
		t.Fatalf("expected ErrRecordTooLarge in BatchWrite, got: %v", err)
	}
}

func TestEngineReadOnlyReplicaMode(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "ro_test.bin")
	walPath := filepath.Join(tempDir, "ro_test.wal")

	engine, err := NewEngine(dbPath, walPath, 20)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	engine.SetReadOnly(true)
	if !engine.IsReadOnly() {
		t.Fatalf("expected engine to report readOnly true")
	}

	// Direct Put, Delete, and BatchWrite must fail with ErrReadOnlyReplica
	if err := engine.Put(10, []byte("val")); !errors.Is(err, ErrReadOnlyReplica) {
		t.Fatalf("expected ErrReadOnlyReplica on Put, got: %v", err)
	}
	if err := engine.PutIfAbsent(10, []byte("val")); !errors.Is(err, ErrReadOnlyReplica) {
		t.Fatalf("expected ErrReadOnlyReplica on PutIfAbsent, got: %v", err)
	}
	if err := engine.Delete(10); !errors.Is(err, ErrReadOnlyReplica) {
		t.Fatalf("expected ErrReadOnlyReplica on Delete, got: %v", err)
	}
	if err := engine.BatchWrite([]BatchOperation{{Type: BatchOpPut, Key: 10, Value: []byte("val")}}); !errors.Is(err, ErrReadOnlyReplica) {
		t.Fatalf("expected ErrReadOnlyReplica on BatchWrite, got: %v", err)
	}

	// Replicated writes must succeed even when readOnly is enabled
	kBytes := []byte{10, 0, 0, 0, 0, 0, 0, 0}
	if err := engine.ApplyReplicatedRecord(1, kBytes, []byte("replicated-val")); err != nil {
		t.Fatalf("expected ApplyReplicatedRecord to succeed on replica, got: %v", err)
	}

	val, found, err := engine.Get(10)
	if err != nil || !found || string(val) != "replicated-val" {
		t.Fatalf("expected replicated value, got: %s (found: %v, err: %v)", val, found, err)
	}
}

func TestGRPCReadOnlyAndSizeValidation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "grpc_ro.bin")
	walPath := filepath.Join(tempDir, "grpc_ro.wal")

	engine, err := NewEngine(dbPath, walPath, 20)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer engine.Close()

	engine.SetReadOnly(true)

	grpcServer := NewGRPCServer(engine)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	go func() { _ = grpcServer.Serve(lis) }()
	defer grpcServer.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	client := beastv1.NewBeastDBServiceClient(conn)
	ctx := context.Background()

	// Put on replica should return codes.FailedPrecondition
	_, err = client.Put(ctx, &beastv1.PutRequest{Key: 1, Value: []byte("hello")})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition on replica Put, got: %v", err)
	}

	// Delete on replica should return codes.FailedPrecondition
	_, err = client.Delete(ctx, &beastv1.DeleteRequest{Key: 1})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition on replica Delete, got: %v", err)
	}

	// Disable read-only to test payload size rejection
	engine.SetReadOnly(false)
	_, err = client.Put(ctx, &beastv1.PutRequest{Key: 1, Value: bytes.Repeat([]byte("X"), 4001)})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument on oversized Put, got: %v", err)
	}
}
