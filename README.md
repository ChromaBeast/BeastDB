# ⚡ BeastDB

A high-performance, distributed, crash-resilient database engine built from first principles in pure Go, bridging systems-level data structures and production cloud engineering.

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Zero Allocs](https://img.shields.io/badge/Hot%20Paths-0%20allocs%2Fop-brightgreen)](ROADMAP.md)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Modularity](https://img.shields.io/badge/Code%20Limit-%3C%20200%20LoC-orange)](GEMINI.md)

---

## 🏛️ Architectural Highlights

- **Mechanical Sympathy:** Flat contiguous memory layouts aligned to 64-byte CPU cache lines, eliminating pointer chasing and GC overhead.
- **Zero-Allocation Hot Paths:** $O(1)$ memory conversions (`unsafe.Slice`, `unsafe.String`), power-of-two bitwise modulo masks, and buffer reuse.
- **Durable Storage Engine:** Write-Ahead Logging (WAL) with IEEE CRC32 torn-write detection, 4KB hardware-aligned Slotted Pages with stable Record IDs (`RID`), and Buffer Pool Manager with Clock-Sweep eviction.
- **On-Disk B+ Tree Index:** Multi-way node fanout, binary search routing, 50/50 page splits, sibling borrowing/merging underflow rebalancing, and streaming Cursor range scans.
- **High-Performance gRPC & Replication:** Protobuf binary API, HTTP/2 multiplexing, streaming scans, and Leader-Follower WAL streaming replication.
- **Modular by Design:** Every single source file strictly adheres to the **`< 200 Lines of Code`** limit for maximum readability and clean separation of concerns.

---

## 📊 Comprehensive Benchmark Suite

Benchmarked on `Intel Core i5-12450HX` (`go test -bench="." -benchmem ./...`):

| Component | Operation | Latency | Throughput | Heap Allocations |
|---|---|---|---|---|
| **Core DSA** | Vector Append | **1.38 ns/op** | ~725M ops/sec | **0 B/op (0 allocs)** |
| **Core DSA** | Ring Buffer Push/Pop | **1.64 ns/op** | ~608M ops/sec | **0 B/op (0 allocs)** |
| **Core DSA** | Hash Table Lookup | **8.39 ns/op** | ~119M ops/sec | **0 B/op (0 allocs)** |
| **Storage** | Slotted Page Tuple Read | **8.18 ns/op** | ~122M reads/sec | **0 B/op (0 allocs)** |
| **Index** | B+ Tree Point Query | **149.0 ns/op** | ~6.7M lookups/sec | **0 B/op (0 allocs)** |
| **Index** | Streaming Cursor Scan (101 keys) | **1064 ns/op** | ~10.5 ns/key | **48 B/op (1 alloc)** |
| **In-Memory** | Sharded Concurrent Cache | **50.14 ns/op** | ~20M ops/sec | **21 B/op (1 alloc)** |
| **Durability** | WAL Append & Sync | **2.35 µs/op** | ~424K writes/sec | **64 B/op (1 alloc)** |

---

## 🗺️ 16-Week Master Roadmap

| Phase | Milestone | Focus Areas | Status |
|---|---|---|---|
| **Phase 1** | [Core DSA & Memory](docs/phase1_dsa.md) | Struct padding, Vector, Ring Buffer, Heap, FNV-1a Open Map | ✅ Completed |
| **Phase 2** | [Network & Cache](docs/phase2_net_cache.md) | TCP framing, RESP parser, Sharded locks, LRU/TTL Cache | ✅ Completed |
| **Phase 3** | [Storage & Indexing](docs/phase3_storage.md) | WAL (CRC32), 4KB Slotted-Page Disk Manager, On-Disk B+ Tree | ✅ Completed |
| **Phase 4** | [Scaling & Production](docs/phase4_scaling.md) | gRPC, Leader-Follower WAL Replication, Docker Cluster | ✅ Completed |

---

## 🚀 Quickstart & Deployment

### Run 3-Node Clustered BeastDB with Docker Compose
```bash
git clone https://github.com/ChromaBeast/BeastDB.git
cd BeastDB

# Launch 1 Primary Leader + 2 Replicas
docker compose up -d
```

### Run Tests & Chaos Validation
```bash
# Run all unit and chaos tests
go test -v ./...

# Run all performance benchmarks
go test -bench="." -benchmem ./...
```

---

## 👤 Author

- **ChromaBeast** ([@ChromaBeast](https://github.com/ChromaBeast))
