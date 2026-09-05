package dsa

import (
	"testing"
)

func TestBinaryCodecLE(t *testing.T) {
	buf := make([]byte, 16)

	PutUint16LE(buf, 0, 0x1234)
	if val := GetUint16LE(buf, 0); val != 0x1234 {
		t.Fatalf("expected 0x1234, got 0x%x", val)
	}

	PutUint32LE(buf, 2, 0xDEADBEEF)
	if val := GetUint32LE(buf, 2); val != 0xDEADBEEF {
		t.Fatalf("expected 0xDEADBEEF, got 0x%x", val)
	}

	PutUint64LE(buf, 6, 0x0123456789ABCDEF)
	if val := GetUint64LE(buf, 6); val != 0x0123456789ABCDEF {
		t.Fatalf("expected 0x0123456789ABCDEF, got 0x%x", val)
	}
}

func TestBinaryCodecBE(t *testing.T) {
	buf := make([]byte, 4)

	PutUint32BE(buf, 0, 0xAABBCCDD)
	if val := GetUint32BE(buf, 0); val != 0xAABBCCDD {
		t.Fatalf("expected 0xAABBCCDD, got 0x%x", val)
	}
	// Verify big endian byte order
	if buf[0] != 0xAA || buf[1] != 0xBB || buf[2] != 0xCC || buf[3] != 0xDD {
		t.Fatalf("unexpected raw byte order: %x", buf)
	}
}

func BenchmarkBinaryCodec(b *testing.B) {
	buf := make([]byte, 16)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		PutUint32LE(buf, 0, uint32(i))
		_ = GetUint32LE(buf, 0)
	}
}
