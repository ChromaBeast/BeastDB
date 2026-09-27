package wal

import (
	"bytes"
	"testing"
)

func TestBatchPayloadEncodeDecode(t *testing.T) {
	ops := []BatchOp{
		{
			Type:  OpPut,
			Key:   []byte("key-1"),
			Value: []byte("val-1"),
		},
		{
			Type:  OpDelete,
			Key:   []byte("key-2"),
			Value: nil,
		},
		{
			Type:  OpPut,
			Key:   []byte("key-3"),
			Value: []byte("val-3-long-content"),
		},
	}

	payload, err := EncodeBatchPayload(ops)
	if err != nil {
		t.Fatalf("failed to encode batch payload: %v", err)
	}

	decoded, err := DecodeBatchPayload(payload)
	if err != nil {
		t.Fatalf("failed to decode batch payload: %v", err)
	}

	if len(decoded) != len(ops) {
		t.Fatalf("expected %d ops, got %d", len(ops), len(decoded))
	}

	for i := range ops {
		if decoded[i].Type != ops[i].Type {
			t.Fatalf("op %d type mismatch: got %d, want %d", i, decoded[i].Type, ops[i].Type)
		}
		if !bytes.Equal(decoded[i].Key, ops[i].Key) {
			t.Fatalf("op %d key mismatch: got %s, want %s", i, decoded[i].Key, ops[i].Key)
		}
		if !bytes.Equal(decoded[i].Value, ops[i].Value) {
			t.Fatalf("op %d value mismatch: got %s, want %s", i, decoded[i].Value, ops[i].Value)
		}
	}
}

func TestBatchPayloadCorrupted(t *testing.T) {
	ops := []BatchOp{
		{Type: OpPut, Key: []byte("k"), Value: []byte("v")},
	}
	payload, err := EncodeBatchPayload(ops)
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// Truncate payload
	truncated := payload[:len(payload)-2]
	_, err = DecodeBatchPayload(truncated)
	if err != ErrInvalidBatchPayload {
		t.Fatalf("expected ErrInvalidBatchPayload, got %v", err)
	}
}
