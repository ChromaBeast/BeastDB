package api

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
)

func TestGRPCServerBatchWrite(t *testing.T) {
	client, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Execute an atomic batch write: put keys 10, 20, 30
	ops := []*beastv1.BatchOperation{
		{
			OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT,
			Key:    10,
			Value:  []byte("val-10"),
		},
		{
			OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT,
			Key:    20,
			Value:  []byte("val-20"),
		},
		{
			OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT,
			Key:    30,
			Value:  []byte("val-30"),
		},
	}

	resp, err := client.BatchWrite(ctx, &beastv1.BatchWriteRequest{Operations: ops})
	if err != nil || !resp.Success || resp.AppliedCount != 3 {
		t.Fatalf("batch write failed: resp=%+v, err=%v", resp, err)
	}

	// 2. Verify all keys were applied
	for _, k := range []uint64{10, 20, 30} {
		getResp, err := client.Get(ctx, &beastv1.GetRequest{Key: k})
		if err != nil || !getResp.Found {
			t.Fatalf("key %d not found after batch write: %v", k, err)
		}
		expected := fmt.Sprintf("val-%d", k)
		if !bytes.Equal(getResp.Value, []byte(expected)) {
			t.Fatalf("key %d value mismatch: got %s, want %s", k, getResp.Value, expected)
		}
	}

	// 3. Execute mixed batch: update 10, delete 20, insert 40
	mixedOps := []*beastv1.BatchOperation{
		{
			OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT,
			Key:    10,
			Value:  []byte("val-10-updated"),
		},
		{
			OpType: beastv1.BatchOpType_BATCH_OP_TYPE_DELETE,
			Key:    20,
		},
		{
			OpType: beastv1.BatchOpType_BATCH_OP_TYPE_PUT,
			Key:    40,
			Value:  []byte("val-40"),
		},
	}

	mixedResp, err := client.BatchWrite(ctx, &beastv1.BatchWriteRequest{Operations: mixedOps})
	if err != nil || !mixedResp.Success || mixedResp.AppliedCount != 3 {
		t.Fatalf("mixed batch write failed: resp=%+v, err=%v", mixedResp, err)
	}

	// 4. Verify results of mixed batch
	get10, _ := client.Get(ctx, &beastv1.GetRequest{Key: 10})
	if string(get10.Value) != "val-10-updated" {
		t.Fatalf("expected updated val-10, got %s", get10.Value)
	}

	get20, _ := client.Get(ctx, &beastv1.GetRequest{Key: 20})
	if get20.Found {
		t.Fatalf("expected key 20 to be deleted")
	}

	get40, _ := client.Get(ctx, &beastv1.GetRequest{Key: 40})
	if !get40.Found || string(get40.Value) != "val-40" {
		t.Fatalf("expected key 40 to be found")
	}
}

func TestEngineBatchWriteCrashRecovery(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "batch_recovery.bin")
	walPath := filepath.Join(tempDir, "batch_recovery.wal")

	engine, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Execute batch write of 50 keys
	var ops []BatchOperation
	for i := uint64(1); i <= 50; i++ {
		ops = append(ops, BatchOperation{
			Type:  BatchOpPut,
			Key:   i,
			Value: []byte(fmt.Sprintf("batch-recovered-%d", i)),
		})
	}

	if err := engine.BatchWrite(ops); err != nil {
		t.Fatalf("batch write failed: %v", err)
	}

	// Abruptly simulate crash without flush
	_ = engine.wal.Sync()
	_ = engine.wal.Close()
	_ = engine.disk.Close()

	// Reopen engine and recover
	recovered, err := NewEngine(dbPath, walPath, 64)
	if err != nil {
		t.Fatalf("failed to reopen engine for recovery: %v", err)
	}
	defer recovered.Close()

	// Verify all 50 batch keys were reconstructed from OpBatch in WAL
	for i := uint64(1); i <= 50; i++ {
		val, found, err := recovered.Get(i)
		if err != nil || !found {
			t.Fatalf("missing recovered batch key %d: %v", i, err)
		}
		expected := fmt.Sprintf("batch-recovered-%d", i)
		if string(val) != expected {
			t.Fatalf("value mismatch for key %d: got %s, want %s", i, val, expected)
		}
	}
}
