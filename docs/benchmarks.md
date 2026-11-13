# BeastDB V1 System Performance Benchmarks

This report documents reproducible system benchmarks, latency profiles, resource utilization, and recovery measurements for BeastDB V1.

---

## 1. Test Harness & Workload Profile

Benchmarks are executed via `go test -v ./internal/api -run TestEngineSystemWorkloadBenchmark` using randomized reference seeds to ensure 100% deterministic reproducibility.

### Workload Characteristics
- **Dataset Seed:** 2,000 pre-populated JSON document records (`~80-120` bytes each).
- **Total Operations:** 10,000 operations per benchmark suite.
- **Request Distribution:**
  - **80% Point Reads (`Get`):** Random uniform key lookups against the B+ tree and slotted page directory.
  - **15% Point Writes (`Put`):** In-place updates and newly allocated records with synchronous WAL append.
  - **5% Range Scans (`ScanRecords`):** 10-key sequential range scans across slotted data pages.
- **Hardware Profile:**
  - Architecture: x86_64 / Windows & Linux
  - Page Size: 4,096 bytes (4KB slotted pages)
  - Buffer Pool Size: 256 frames (1MB memory pool)
  - Storage: Direct local NVMe / SSD

---

## 2. Measured Results & Latency Distribution

| Metric | Measured Value | Target SLA |
|---|---|---|
| **Sustained Throughput** | **17,789 ops/sec** | > 5,000 ops/sec |
| **p50 Latency** | **< 1 µs** | < 100 µs |
| **p95 Latency** | **540.6 µs** | < 2,000 µs |
| **p99 Latency** | **1,000 µs (1.0ms)** | < 5,000 µs |
| **Buffer Cache Hit Ratio**| **100%** (97/97 pages) | > 95% |
| **WAL Growth (10k ops)** | **376,050 bytes** | < 1 MB |

---

## 3. Microbenchmarks (Component Breakdown)

Measured on 12th Gen Intel(R) Core(TM) i5-12450HX:
- **Point Read with Cache (`BenchmarkEnginePointRead`):**
  - **298.7 ns/op** | 48 B/op | 1 alloc/op
  - Direct B+ tree index lookup and slotted page record extraction. Hot-path reads bypass disk I/O entirely when cached in the buffer pool, delivering sub-microsecond retrieval.
- **Point Read without Cache (`BenchmarkEnginePointReadWithoutCache`):**
  - **543.4 ns/op** | 391 B/op | 0 allocs/op
  - Measured across 2,500 records with a constrained 2-frame buffer pool to force frequent clock-sweep page eviction and disk re-reads. Zero heap allocations on the eviction and lookup path.
- **Point Write (`BenchmarkEnginePointWrite`):**
  - **4.02 ms/op** (with synchronous disk sync) | 600 B/op | 5 allocs/op
  - CRC32-checked WAL append, atomic LSN increment, B+ tree key insertion, and slotted page tuple allocation. Writes achieve predictable durability without unbounded heap allocation.

---

### Concurrent writes and group commit

Writers now share `fsync`s (group commit, see [architecture](architecture.md)). On one Apple-silicon laptop the engine benchmark `BenchmarkEngineConcurrentWrite` (96 concurrent writer goroutines) measured about 7.8 ms/op before and 0.5 ms/op after; `BenchmarkEnginePointWrite` (one sequential writer) stays `fsync`-bound at about 9 ms/op. These are single-machine, in-process numbers and not a comparison with other databases. For end-to-end gRPC workloads use `beastbench` and follow the [benchmarking guide](benchmarking.md).

---

## 4. Crash Recovery & Disaster Recovery RTO

Measurements taken using `cmd/beastctl/backup_drill_test.go`:
- **Clean-Host Extraction & Verification Time:** `~14.8 ms` (reading archive, parsing manifest, verifying SHA-256 digests).
- **WAL Crash Recovery & Checkpoint Replay:** `< 25 ms` (replaying uncommitted and checkpointed log records).
- **Total Measured Recovery Time Objective (RTO):** `< 40 ms` to fully initialized, query-ready state.
- **Measured Data Loss (RPO):** `0 records` across clean checkpoint and recovery drills.


---

## 5. Encryption at rest overhead

Measured on Apple M6 (12 logical CPUs), Go 1.27.2, darwin/arm64, a local external SSD, on 13 November 2026 with the build that introduced encryption ([ADR 0002](adr/0002-encryption-at-rest.md)). Each pair is the same operation with encryption off and on, in the same process settings. These are single-machine numbers with run-to-run noise as noted; they say nothing about other databases.

| Measurement | Off | On | Reading |
|---|---|---|---|
| Seal one 4 KB page (`BenchmarkPageSeal`) | n/a | ~0.72-0.78 us, 0 allocs | AES-256-GCM, about 5.2-5.7 GB/s |
| Open one 4 KB page (`BenchmarkPageOpen`) | n/a | ~0.54-0.59 us, 0 allocs | about 7.0-7.6 GB/s |
| Point write, fsync per commit (`BenchmarkEncryptionPut`, 3 runs) | 2.30-2.37 ms | 2.31-2.32 ms | no measurable difference: the fsync dominates |
| Point read, pool of 32 frames over 60,000 records (`BenchmarkEncryptionGetMiss`, 6 runs) | 1.8-2.4 us | 3.0-3.7 us | about +1.1 us per read that misses the pool; a hit costs nothing extra |
| Checkpoint of 500 records (`BenchmarkEncryptionCheckpoint`, 3 runs) | 13.2-18.6 ms | 16.6-18.7 ms | within run-to-run spread (each run varied by more than the difference) |
| `beastbench --self-contained` workload C, 100,000 reads, 8 threads, 64-frame pool, 3 runs each | 83,600-107,300 ops/s, p50 70-87 us | 94,400-103,500 ops/s, p50 72-77 us | the two ranges overlap; loopback gRPC and machine noise exceed the cryptographic cost |
| `beastbench` workload A (50/50) and load phase, 2 runs each | load 415-1,230 ops/s, run 1,044-1,059 ops/s | load 578-1,086 ops/s, run 1,189-1,217 ops/s | fsync-bound and very noisy on this disk; no overhead is visible |

How to reproduce: `go test ./internal/api -run xxx -bench Encryption -benchtime 2s -count 3`, `go test ./internal/storage -run xxx -bench 'Page(Seal|Open)'`, and `beastbench --self-contained [--encrypt] --pool-size 64 --workload C --records 20000 --ops 100000 --threads 8 --value-size 256`.

Space: an encrypted page holds 4,064 bytes of slotted-page data instead of 4,096 (0.8% less), and the WAL adds 40 bytes per record plus a 16-byte header.
