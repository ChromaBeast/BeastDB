package dsa

import (
	"bytes"
	"testing"
)

func TestStringToBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"short ascii", "hello world"},
		{"database key", "user:profile:10092"},
		{"binary safe data", "hello\x00\xff\xfe"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := StringToBytes(tc.input)
			if tc.input == "" {
				if b != nil {
					t.Fatalf("expected nil for empty string, got len=%d", len(b))
				}
				return
			}
			if string(b) != tc.input {
				t.Fatalf("expected %q, got %q", tc.input, string(b))
			}
		})
	}
}

func TestBytesToString(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty slice", nil},
		{"zero length slice", []byte{}},
		{"short ascii", []byte("hello database")},
		{"binary sequence", []byte{0x01, 0x02, 0x03, 0xff}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := BytesToString(tc.input)
			if len(tc.input) == 0 {
				if s != "" {
					t.Fatalf("expected empty string, got %q", s)
				}
				return
			}
			if !bytes.Equal([]byte(s), tc.input) {
				t.Fatalf("expected %v, got %v", tc.input, []byte(s))
			}
		})
	}
}

func TestCloneBytes(t *testing.T) {
	src := []byte("persistent-data")
	clone := CloneBytes(src)

	if !bytes.Equal(src, clone) {
		t.Fatalf("expected clone to match src")
	}

	clone[0] = 'X'
	if src[0] == 'X' {
		t.Fatalf("modifying clone mutated original source byte slice")
	}
}

func BenchmarkStandardStringToBytes(b *testing.B) {
	s := "benchmarking-database-key-allocation"
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = []byte(s)
	}
}

func BenchmarkZeroCopyStringToBytes(b *testing.B) {
	s := "benchmarking-database-key-allocation"
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = StringToBytes(s)
	}
}

func BenchmarkStandardBytesToString(b *testing.B) {
	bs := []byte("benchmarking-database-key-allocation")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = string(bs)
	}
}

func BenchmarkZeroCopyBytesToString(b *testing.B) {
	bs := []byte("benchmarking-database-key-allocation")
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = BytesToString(bs)
	}
}
