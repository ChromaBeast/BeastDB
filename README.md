# ⚡ BeastDB

A high-performance, crash-resilient database engine built from first principles in pure Go, bridging systems-level data structures and production engineering.

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Zero Allocs](https://img.shields.io/badge/Hot%20Paths-0%20allocs%2Fop-brightgreen)](ROADMAP.md)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Modularity](https://img.shields.io/badge/Code%20Limit-%3C%20200%20LoC-orange)](GEMINI.md)

---

## 🏛️ Architectural Highlights

- **Mechanical Sympathy:** Flat contiguous memory layouts designed around CPU L1/L2 cache lines (64 bytes), eliminating pointer chasing and garbage collector scanning pressure.
- **Zero-Allocation Hot Paths:** Memory conversions (`unsafe.Slice`, `unsafe.String`), bitwise power-of-two modulo masking, and arena buffer reuse.
- **Protocol Framing:** 10-byte fixed binary header (`0xDB` Magic Byte, OpCode, Payload Length, IEEE CRC32 Checksum) solving TCP stream packet fragmentation.
- **Modular by Design:** Every single source file and document strictly adheres to a **`< 200 Lines of Code`** limit for maximum readability and clean separation of concerns.

---

## 📊 Benchmark Highlights

Benchmarked on `Intel Core i5-12450HX` (`go test -bench="." -benchmem ./...`):

| Operation | Implementation | Latency | Heap Allocations |
|---|---|---|---|
| **String to Bytes** | `dsa.StringToBytes` (Zero-Copy) | **0.31 ns/op** | **0 B/op (0 allocs)** |
| **Bytes to String** | `dsa.BytesToString` (Zero-Copy) | **0.30 ns/op** | **0 B/op (0 allocs)** |
| **Hash Table Lookup** | `dsa.HashTable.Get` (Open-Addressing) | **8.48 ns/op** | **0 B/op (0 allocs)** |
| **Ring Buffer Push/Pop** | `dsa.RingBuffer` (Bitwise Mask) | **1.68 ns/op** | **0 B/op (0 allocs)** |
| **Vector Append** | `dsa.Vector` (1.5x Expansion) | **1.30 ns/op** | **0 B/op (0 allocs)** |
| **Binary Codec** | `dsa.PutUint32LE / GetUint32LE` | **0.12 ns/op** | **0 B/op (0 allocs)** |

---

## 🗺️ 16-Week Roadmap Overview

| Phase | Milestone | Focus Areas | Status |
|---|---|---|---|
| **Phase 1** | [Core DSA & Memory](docs/phase1_dsa.md) | Struct padding, Vector, Ring Buffer, Heap, FNV-1a Open Map | ✅ Completed |
| **Phase 2** | [Network & Cache](docs/phase2_net_cache.md) | TCP framing, RESP parser, Sharded locks, LRU/TTL Cache | 🔄 In Progress |
| **Phase 3** | [Storage & Indexing](docs/phase3_storage.md) | WAL (CRC32), 4KB Slotted-Page Disk Manager, On-Disk B+ Tree | 📅 Planned |
| **Phase 4** | [Scaling & Production](docs/phase4_scaling.md) | gRPC / CDC Streaming, Mobile LSN Delta Sync, Distroless Docker | 📅 Planned |

*See [ROADMAP.md](ROADMAP.md) for the complete 16-week execution syllabus.*

---

## 🚀 Quickstart

### Prerequisites
- Go 1.26 or higher

### Build & Run Daemon
```bash
# Clone the repository
git clone https://github.com/ChromaBeast/BeastDB.git
cd BeastDB

# Start the BeastDB server
go run ./cmd/server
```

### Run Benchmarks & Tests
```bash
# Run all unit tests
go test -v ./...

# Run memory allocation benchmarks
go test -bench="." -benchmem ./...
```

---

## 📜 Engineering Principles

1. **Zero-Allocation Hot Paths:** Hot query, hashing, and parsing paths must not trigger unnecessary heap allocations; prefer byte slices, arenas, and sync pools.
2. **Mechanical Sympathy:** Align structs to avoid false sharing and padding waste, favor flat memory layouts over pointer chasing, and ensure crash durability via WAL.
3. **Strict Modularity:** Every source file remains readable and `< 200 LoC`.

---

## 👤 Author

- **ChromaBeast** ([@ChromaBeast](https://github.com/ChromaBeast))
