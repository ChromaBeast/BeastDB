package api

import (
	"math/rand"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// BenchmarkReport captures measurable performance and latency distribution.
type BenchmarkReport struct {
	TotalOps    int
	Duration    time.Duration
	OpsPerSec   float64
	P50Latency  time.Duration
	P95Latency  time.Duration
	P99Latency  time.Duration
	WALBytes    int64
	TotalPages  uint64
	CachedPages int
}

func runWorkloadBenchmark(t *testing.T, numOps int, poolSize int) BenchmarkReport {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "bench.bin")
	walPath := filepath.Join(dir, "bench.wal")

	engine, err := NewEngine(dbPath, walPath, poolSize)
	if err != nil {
		t.Fatalf("failed to init engine: %v", err)
	}
	defer engine.Close()

	// Pre-populate with initial dataset
	const initialKeys = 2000
	payload := []byte(`{"id":1001,"title":"The Matrix","year":1999,"rating":8.7,"status":"completed"}`)
	for i := uint64(1); i <= initialKeys; i++ {
		if err := engine.Put(i, payload); err != nil {
			t.Fatalf("populate failed: %v", err)
		}
	}

	latencies := make([]time.Duration, numOps)
	rng := rand.New(rand.NewSource(42))

	start := time.Now()
	for i := 0; i < numOps; i++ {
		opStart := time.Now()
		roll := rng.Intn(100)

		if roll < 80 {
			// 80% Get (Point read)
			key := uint64(rng.Intn(initialKeys) + 1)
			_, _, _ = engine.Get(key)
		} else if roll < 95 {
			// 15% Put (Point write/update)
			key := uint64(rng.Intn(initialKeys*2) + 1)
			_ = engine.Put(key, payload)
		} else {
			// 5% Scan (Range scan)
			startKey := uint64(rng.Intn(initialKeys) + 1)
			_, _ = engine.ScanRecords(startKey, 10)
		}
		latencies[i] = time.Since(opStart)
	}
	totalDuration := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	metrics := engine.StorageMetrics()

	return BenchmarkReport{
		TotalOps:    numOps,
		Duration:    totalDuration,
		OpsPerSec:   float64(numOps) / totalDuration.Seconds(),
		P50Latency:  latencies[numOps*50/100],
		P95Latency:  latencies[numOps*95/100],
		P99Latency:  latencies[numOps*99/100],
		WALBytes:    metrics.WALSizeBytes,
		TotalPages:  metrics.TotalPages,
		CachedPages: metrics.CachedPages,
	}
}

func TestEngineSystemWorkloadBenchmark(t *testing.T) {
	report := runWorkloadBenchmark(t, 10000, 256)

	t.Logf("=== BeastDB Mixed Workload Benchmark (10,000 Ops: 80%% Read / 15%% Write / 5%% Scan) ===")
	t.Logf("Total Time:     %v", report.Duration)
	t.Logf("Throughput:     %.2f ops/sec", report.OpsPerSec)
	t.Logf("p50 Latency:    %v", report.P50Latency)
	t.Logf("p95 Latency:    %v", report.P95Latency)
	t.Logf("p99 Latency:    %v", report.P99Latency)
	t.Logf("Total Pages:    %d", report.TotalPages)
	t.Logf("Cached Pages:   %d", report.CachedPages)
	t.Logf("WAL Size:       %d bytes", report.WALBytes)

	if report.OpsPerSec < 1000 {
		t.Errorf("throughput unexpectedly low: %.2f ops/sec", report.OpsPerSec)
	}
}

func BenchmarkEnginePointRead(b *testing.B) {
	dir := b.TempDir()
	engine, err := NewEngine(filepath.Join(dir, "bench.bin"), filepath.Join(dir, "bench.wal"), 256)
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	payload := []byte(`{"name":"benchmark-record","val":12345}`)
	for i := uint64(1); i <= 1000; i++ {
		_ = engine.Put(i, payload)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := uint64((i % 1000) + 1)
		_, _, _ = engine.Get(key)
	}
}

func BenchmarkEnginePointWrite(b *testing.B) {
	dir := b.TempDir()
	engine, err := NewEngine(filepath.Join(dir, "bench.bin"), filepath.Join(dir, "bench.wal"), 256)
	if err != nil {
		b.Fatal(err)
	}
	defer engine.Close()

	payload := []byte(`{"name":"benchmark-record","val":12345}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.Put(uint64(i+1), payload)
	}
}
