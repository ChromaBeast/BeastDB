# BeastDB: 16-Week Custom Database Engine Roadmap

A structured 16-week execution plan bridging systems-level data structures and production engineering while building BeastDB in pure Go from scratch.

---

## Architectural Overview

| Layer | Component | Week | Responsibility | Status |
|---|---|---|---|---|
| **Core DSA**| Systems Primitives | 1–3 | Memory alignment, vector, ring buffer, open hash map | ✅ Completed |
| **Network** | TCP & RESP | 4–5 | 10-byte binary framing, zero-alloc RESP2 parser | ✅ Completed |
| **Cache** | In-Memory Store | 6–7 | Sharded RWMutex locks, LRU eviction, TTL heap sweeps | ✅ Completed |
| **Durability**| WAL & Checkpoints | 8 | Append-only log, CRC32 verification, crash replay | ✅ Completed |
| **Storage**| Buffer Pool & Paging | 9–10 | 4KB slotted pages, clock-sweep frame eviction | ✅ Completed |
| **Index** | On-Disk B+ Tree | 11–12 | Multi-way branching, split/merge, range scans | ✅ Completed |
| **API** | gRPC & Protobuf | 13 | HTTP/2 multiplexing, unary & streaming RPCs | ✅ Completed |
| **Replication**| Distributed Sync | 14 | Leader-Follower physical WAL streaming & ACKs | ✅ Completed |
| **Deployment**| Docker & Cluster | 15 | Multi-stage static build, 3-node compose cluster | ✅ Completed |
| **Verification**| Chaos & Benchmarks | 16 | Torn-write crash recovery, stress testing, benchmarks | ✅ Completed |

---

## 16-Week Phase Summary

### Phase 1: Go Systems Fundamentals & Core DSA (Weeks 1–3) [✅ Completed]
* Zero-copy string/byte conversions via `unsafe.Pointer`.
* Dynamic vector, power-of-two circular ring buffer, min/max heap.
* Open-addressing hash table with linear probing and tombstone tracking.

### Phase 2: Networking & In-Memory Store (Weeks 4–7) [✅ Completed]
* Custom TCP socket server with 10-byte binary frame header (`0xDB`).
* Zero-allocation streaming RESP2 protocol parser and writer.
* 64-shard partitioned RWMutex map and hardware atomic CPU metrics.
* In-memory LRU cache with active Min-Heap and passive TTL expiration.

### Phase 3: Storage Engine & Indexing (Weeks 8–12) [✅ Completed]
* Append-only WAL with CRC32 torn-write detection and crash replayer.
* 4KB Slotted-Page manager with Record Identifiers (`RID`) and defragmentation.
* Buffer Pool Manager with Clock-Sweep eviction and page pinning.
* On-Disk B+ Tree index with binary search routing, 50/50 leaf splits, and underflow rebalancing.
* Streaming B+ Tree `Cursor` range scans via hand-over-hand page pinning.

### Phase 4: Distributed Scaling, Replication & Cloud (Weeks 13–16) [✅ Completed]
* Strongly typed gRPC Protobuf API (`Get`, `Put`, `Delete`, streaming `Scan`).
* Leader-Follower WAL streaming replication with historical replay and live sync.
* Multi-stage static Docker container and 3-node cluster compose topology.
* Chaos fault-injection (torn-write recovery), concurrent stress tests, and benchmarks.
