package net

import (
	"bytes"
	"testing"
)

func TestFrameEncodeDecode(t *testing.T) {
	payload := []byte("SET user:heisenberg Walter_White")
	encoded := EncodeFrame(OpSet, payload)

	if len(encoded) != HeaderSize+len(payload) {
		t.Fatalf("unexpected encoded size: got %d, want %d", len(encoded), HeaderSize+len(payload))
	}

	frame, err := DecodeFrame(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("failed to decode valid frame: %v", err)
	}

	if frame.OpCode != OpSet {
		t.Fatalf("unexpected OpCode: got %d, want %d", frame.OpCode, OpSet)
	}

	if !bytes.Equal(frame.Payload, payload) {
		t.Fatalf("payload mismatch: got %q, want %q", frame.Payload, payload)
	}
}

func TestFrameInvalidMagic(t *testing.T) {
	encoded := EncodeFrame(OpPing, []byte("PING"))
	encoded[0] = 0xAA // Corrupt magic byte

	_, err := DecodeFrame(bytes.NewReader(encoded))
	if err != ErrInvalidMagic {
		t.Fatalf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestFrameChecksumCorruption(t *testing.T) {
	encoded := EncodeFrame(OpGet, []byte("user:101"))
	// Corrupt a byte in the payload (index 12 is inside payload)
	encoded[12] ^= 0xFF

	_, err := DecodeFrame(bytes.NewReader(encoded))
	if err != ErrChecksumMismatch {
		t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	}
}

func BenchmarkFrameEncode(b *testing.B) {
	payload := []byte("GET user:profile:benchmark")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = EncodeFrame(OpGet, payload)
	}
}
