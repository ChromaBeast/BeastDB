# ⚡ BeastDB

A high-performance, distributed, crash-resilient database engine built from scratch in **pure Go** — no frameworks, no ORMs, no shortcuts. Every byte of the storage engine, replication layer, and index was implemented by hand, from first principles.

[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Zero Allocs](https://img.shields.io/badge/Hot%20Paths-0%20allocs%2Fop-brightgreen)](ROADMAP.md)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Modularity](https://img.shields.io/badge/Code%20Limit-%3C%20200%20LoC-orange)](GEMINI.md)
[![Tests](https://img.shields.io/badge/Tests-8%2F8%20packages%20passing-success)](/)

---

## 🏛️ What's Inside

BeastDB is a 16-week systems engineering deep-dive, building every major database component layer by layer:

| Layer | What Was Built |
|---|---|
| **Memory** | Zero-copy string↔byte conversions, cache-line-padded structs, FNV-1a hasher, open-addressing hash map, generic binary heap, ring buffer |
| **Network** | Custom binary TCP frame protocol (10-byte header + CRC32), zero-allocation RESP2 parser & writer |
| **Cache** | 64-shard partitioned `RWMutex` map, O(1) LRU eviction list, dual passive+active TTL heap, `sync.Pool` buffer recycling |
| **Storage** | 4KB hardware-aligned Slotted Pages (stable `RID`s, O(1) tombstone delete, defrag), `fsync`-safe Disk Manager, Clock-Sweep Buffer Pool Manager with per-frame atomic pin counts |
| **Durability** | 23-byte binary WAL frames with IEEE CRC32 torn-write detection, crash recovery scanner, WAL rotation/checkpointing |
| **Indexing** | On-disk B+ Tree — binary search routing, 50/50 leaf splits, left+right sibling borrow/merge on underflow, streaming `Cursor` with hand-over-hand page pinning and epoch-based concurrent modification detection |
| **API** | Protobuf schema + gRPC service (unary + server-streaming), HTTP/2 multiplexing |
| **Replication** | Leader-Follower WAL streaming — two-phase catch-up replay + live pub-sub broadcaster, batched ACK coalescing, follower crash recovery |
| **Operations** | Multi-stage Docker build (CGO_ENABLED=0, static binary), 3-node Docker Compose cluster, graceful 10-second SIGTERM shutdown |

---

## 📊 Performance Benchmarks

Benchmarked on **Intel Core i5-12450HX**, **Go 1.26**, Windows/amd64 (`go test -bench=. -benchmem ./...`):

| Component | Benchmark | Latency | Throughput | Allocs |
|---|---|---|---|---|
| **Core DSA** | Vector Push | **1.37 ns/op** | ~729M ops/sec | **0 B · 0 allocs** |
| **Core DSA** | Ring Buffer Push/Pop | **1.73 ns/op** | ~578M ops/sec | **0 B · 0 allocs** |
| **Core DSA** | Open-Address Hash Get | **8.45 ns/op** | ~118M ops/sec | **0 B · 0 allocs** |
| **Core DSA** | Zero-Copy `[]byte→string` | **0.30 ns/op** | ~3.3B ops/sec | **0 B · 0 allocs** |
| **Storage** | Slotted Page Tuple Read | **8.01 ns/op** | ~125M reads/sec | **0 B · 0 allocs** |
| **Index** | B+ Tree Point Query | **180 ns/op** | ~5.5M lookups/sec | **0 B · 0 allocs** |
| **Index** | Streaming Cursor Scan (101 keys) | **1208 ns/op** | ~12 ns/key | **64 B · 1 alloc** |
| **Cache** | Sharded Concurrent Get | **51.1 ns/op** | ~19.5M ops/sec | **21 B · 1 alloc** |
| **Cache** | Set/Get Roundtrip | **92.8 ns/op** | ~10.7M ops/sec | **11 B · 1 alloc** |
| **Durability** | WAL Append + fsync | **2.35 µs/op** | ~425K writes/sec | **64 B · 1 alloc** |
| **Network** | TCP Frame Encode | **30.4 ns/op** | ~33M frames/sec | **48 B · 1 alloc** |
| **API** | gRPC End-to-End Get | **129 µs/op** | ~7.7K req/sec | 9KB · 152 allocs |

---

## 🏗️ Architecture

```mermaid
flowchart TD
    Client["Client\n(gRPC / Protobuf)"] --> GS["GRPCServer\nHTTP/2 · Streaming RPC"]

    GS --> E["Engine\nRWMutex coordinator"]

    E --> WAL["WAL\nCRC32 torn-write guard\nAppend-only · fsync"]
    E --> BPM["BufferPoolManager\nClock-Sweep eviction\nRWMutex fast-path · sync.Pool"]
    E --> BT["B+ Tree\nBinary search · epoch guard\nLeft+right sibling rebalance"]

    BPM --> DM["DiskManager\n4KB aligned pages · fsync"]
    BT --> BPM

    BT --> CUR["Cursor\nHand-over-hand pinning\nEpoch concurrency guard"]

    subgraph Replication
        L["LeaderServer\nTwo-phase WAL stream"] --> BC["Broadcaster\nPub-sub fan-out"]
        BC --> F["FollowerClient\nCatch-up + live sync\nBatched ACK · 100ms coalesce"]
    end

    E --> L
    F --> E
```

> 📐 Full component spec: [docs/architecture.md](docs/architecture.md)

---

## 🗺️ 16-Week Roadmap

| Phase | Milestone | Focus Areas | Status |
|---|---|---|---|
| **Phase 1** | [Core DSA & Memory](docs/phase1_dsa.md) | Struct padding, Vector, Ring Buffer, Heap, FNV-1a Hash Map | ✅ |
| **Phase 2** | [Network & Cache](docs/phase2_net_cache.md) | TCP framing, RESP2 parser, Sharded locks, LRU+TTL Cache | ✅ |
| **Phase 3** | [Storage & Indexing](docs/phase3_storage.md) | WAL (CRC32), 4KB Slotted Pages, Disk Manager, B+ Tree + Cursor | ✅ |
| **Phase 4** | [Scaling & Production](docs/phase4_scaling.md) | gRPC/Protobuf, Leader-Follower Replication, Docker Cluster | ✅ |
## 🚀 Quickstart

### 3-Node Cluster with Docker Compose
```bash
git clone https://github.com/ChromaBeast/BeastDB.git
cd BeastDB

# Launch 1 Primary (port 50051) + 2 Replicas (50052, 50053)
docker compose up -d
```

### Run Tests & Chaos Validation
```bash
# All unit + chaos tests (torn-write injection, 20-goroutine stress)
go test -v ./...

# Performance benchmarks with memory profiles
go test -bench=. -benchmem ./...
```

### Build Static Binary
```bash
CGO_ENABLED=0 GOOS=linux go build -o beastdb ./cmd/server
```

---

## 👤 Author

**ChromaBeast** · [GitHub @ChromaBeast](https://github.com/ChromaBeast)
