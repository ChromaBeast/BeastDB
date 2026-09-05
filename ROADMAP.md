# CustomDB: 16-Week Custom Database Engine Roadmap

A structured 16-week execution plan bridging systems-level data structures and production engineering while building a custom database engine in Go from scratch.

---

## Architectural Overview

| Layer | Component | Week | Responsibility |
|---|---|---|---|
| **Client** | CLI & SDK | 14 | Automatic retries, connection pooling, mobile delta sync |
| **API** | TCP / RESP / gRPC | 4–5, 13 | Binary framing, RESP2/3 tokenizer, RPC streaming |
| **Cache** | In-Memory Store | 6–7 | Sharded RW locks, LRU eviction, TTL timers |
| **Index** | On-Disk B+ Tree | 11–12 | Multi-way branching, split/merge, range scans |
| **Storage**| Buffer Pool & Paging | 9–10 | 4KB slotted pages, clock-sweep frame eviction |
| **Durability**| WAL & Checkpoints | 8 | Append-only log, CRC32 verification, crash replay |
| **Core DSA**| Systems Primitives | 1–3 | Memory alignment, vector, ring buffer, open hash map |

---

## 16-Week Phase Breakdown

### [Phase 1: Go Systems Fundamentals & Core DSA (Weeks 1–3)](docs/phase1_dsa.md)
* **Week 1: Syntax & Memory Layouts:** Struct padding, word alignment, cache line sizing, slice header mechanics, and zero-copy byte slice conversions via `unsafe.Pointer`.
* **Week 2: Linear Data Structures:** Pure Go dynamic vector, power-of-two circular ring buffer, and array-backed binary min/max heap.
* **Week 3: Associative Structures & Profiling:** Open-addressing hash table with Robin Hood hashing or linear probing, automated memory benchmarking (`go test -benchmem`), and `pprof` heap profiles.

### [Phase 2: Networking & In-Memory Store (Weeks 4–7)](docs/phase2_net_cache.md)
* **Week 4: TCP Protocol Engine:** Non-blocking socket listener via `net.Listen`, worker pool dispatcher, and custom binary frame parser (magic byte + CRC32).
* **Week 5: RESP Parser:** Zero-allocation Redis Serialization Protocol parser supporting bulk strings, integers, arrays, and error frames.
* **Week 6: Concurrency Primitives:** Lock striping across hash shards with `sync.RWMutex`, lock-free atomic metrics (`sync/atomic`), and connection throttling.
* **Week 7: In-Memory Caching:** Doubly linked list + hash map LRU eviction, and active background sweep with passive on-read TTL expiration.

### [Phase 3: Storage Engine & Indexing (Weeks 8–12)](docs/phase3_storage.md)
* **Week 8: Write-Ahead Logging (WAL):** Append-only commit log, LSN tracking, CRC32 checksums, fsync policies, and recovery replay scanner.
* **Week 9: Disk Paging Architecture:** Fixed 4KB slotted-page format (page header, slot directory growing down, tuple records growing up), and defragmentation.
* **Week 10: Buffer Pool Manager:** Memory frame pool, page table mapping, pin count reference safety, and clock-sweep eviction of dirty pages.
* **Week 11: On-Disk B+ Tree (Search & Insert):** 4KB node format, root-to-leaf binary search traversal, and 50/50 page split on overflow.
* **Week 12: On-Disk B+ Tree (Deletes & Scans):** Sibling key rebalancing, node merge underflow handling, and leaf-linked cursor range scans.

### [Phase 4: Scaling, Mobile Sync & Deployment (Weeks 13–16)](docs/phase4_scaling.md)
* **Week 13: API & Streaming Layer:** gRPC Protobuf services, change-data-capture streaming, token auth, and token-bucket rate limiting.
* **Week 14: Client SDK & Mobile Sync:** Resilient Go client with exponential backoff and LSN delta synchronization protocol for mobile clients.
* **Week 15: Containerization & Hosting:** Multi-stage distroless Docker image (< 25MB), volume mounts, systemd service unit, and graceful crash handling.
* **Week 16: Profiling & Load Testing:** `k6` / `wrk` load generation targeting 50k+ QPS, `pprof` bottleneck analysis, and Go runtime GC tuning (`GOMEMLIMIT`).

---

## Core Engineering Constraints
1. **Strict File Size Limit:** Every source file and document must remain **< 200 LoC** for high readability and modular separation of concerns.
2. **Zero-Allocation Hot Paths:** Hot query and parsing paths must not trigger unnecessary heap allocations; use byte slices, arenas, and sync pools.
3. **Crash Safety by Design:** Any committed transaction must be recoverable through WAL replaying with verified CRC32 checksums.
