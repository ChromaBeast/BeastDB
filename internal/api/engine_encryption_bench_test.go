package api

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ChromaBeast/beastdb/internal/crypt"
)

// Run: go test ./internal/api -run xxx -bench Encryption -benchtime 3s
// Each pair measures the same operation with and without encryption at rest on
// this machine; there is no cross-database comparison here.
func benchEngine(b *testing.B, encrypted bool, pool int) *Engine {
	b.Helper()
	dir := b.TempDir()
	var opts Options
	if encrypted {
		opts.KeyFile = filepath.Join(dir, "k")
		if err := crypt.GenerateKeyFile(opts.KeyFile); err != nil {
			b.Fatal(err)
		}
	}
	e, err := NewEngineWithOptions(filepath.Join(dir, "b.bin"), filepath.Join(dir, "b.wal"), pool, opts)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = e.Close() })
	return e
}

func BenchmarkEncryptionPut(b *testing.B) {
	for _, enc := range []bool{false, true} {
		b.Run(fmt.Sprintf("encrypted=%v", enc), func(b *testing.B) {
			e := benchEngine(b, enc, 256)
			val := make([]byte, 100)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := e.Put(uint64(i+1), val); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEncryptionGetMiss reads from a dataset far larger than the pool, so
// most reads load a page from disk (one decrypt each).
func BenchmarkEncryptionGetMiss(b *testing.B) {
	const records = 60000
	for _, enc := range []bool{false, true} {
		b.Run(fmt.Sprintf("encrypted=%v", enc), func(b *testing.B) {
			e := benchEngine(b, enc, 32)
			val := make([]byte, 100)
			ops := make([]BatchOperation, 0, 500)
			for i := 1; i <= records; i++ {
				ops = append(ops, BatchOperation{Type: BatchOpPut, Key: uint64(i), Value: val})
				if len(ops) == 500 {
					if err := e.BatchWrite(ops); err != nil {
						b.Fatal(err)
					}
					ops = ops[:0]
				}
			}
			if err := e.Checkpoint(); err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			x := uint64(1)
			for i := 0; i < b.N; i++ {
				x = x*6364136223846793005 + 1442695040888963407
				if _, _, err := e.Get(x>>33%records + 1); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEncryptionCheckpoint measures one checkpoint of ~20000 dirty pages' worth of data.
func BenchmarkEncryptionCheckpoint(b *testing.B) {
	for _, enc := range []bool{false, true} {
		b.Run(fmt.Sprintf("encrypted=%v", enc), func(b *testing.B) {
			e := benchEngine(b, enc, 4096)
			val := make([]byte, 400)
			key := uint64(1)
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				ops := make([]BatchOperation, 0, 500)
				for j := 0; j < 500; j++ {
					ops = append(ops, BatchOperation{Type: BatchOpPut, Key: key, Value: val})
					key++
				}
				if err := e.BatchWrite(ops); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				if err := e.Checkpoint(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
