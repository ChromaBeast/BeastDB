<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/beastdb-preview-dark.png" />
    <img src="docs/assets/beastdb-preview-light.png" alt="BeastDB" width="440" />
  </picture>
</p>

<h3 align="center">High-Performance, Crash-Resilient Distributed Database — Built from Scratch in Pure Go</h3>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go" alt="Go Version" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License" /></a>
  <a href="ROADMAP.md"><img src="https://img.shields.io/badge/Hot%20Paths-0%20allocs%2Fop-brightgreen" alt="Zero Allocs" /></a>
  <img src="https://img.shields.io/badge/Tests-Passing-success" alt="Tests" />
  <img src="https://img.shields.io/badge/Studio-Embedded-blueviolet" alt="Studio" />
</p>

---

BeastDB is a purpose-built OLTP database engine designed for **read-heavy, structured workloads**. Every layer — from memory allocation to on-disk page layout — is implemented from first principles in Go with zero external dependencies beyond gRPC.

It ships as a **single static binary** that includes a full gRPC service, WAL-backed durability, leader-follower replication, and an embedded Next.js web console. No sidecars, no JVM, no Node.js runtime in production.

---

## Why BeastDB?

| Concern | How BeastDB addresses it |
|---|---|
| **Predictable latency** | B+ Tree with `O(log N)` point reads at 180 ns, 0 allocs — no compaction spikes |
| **Crash safety** | 23-byte ARIES WAL frames with CRC32 torn-write detection; full replay on boot |
| **Operational simplicity** | Single binary, `go run ./cmd/server -dev` for local dev, `docker compose up` for prod |
| **Developer ergonomics** | Embedded Studio web UI, `beastctl` CLI, partition-aware key builder, fixture seeding |
| **Replication** | Leader-follower WAL streaming — two-phase catch-up + live pub-sub, batched ACK coalescing |

---

## Quickstart

### Local dev (ephemeral, zero disk residue)

```bash
go run ./cmd/server -dev
```

Starts the gRPC server on `:50051`, opens Studio at `http://localhost:8088` with credentials `admin / admin`, and purges all data on `Ctrl+C`. No leftover files.

### Production / persistent

```bash
go run ./cmd/server \
  -role leader \
  -port 50051 \
  -data-dir ./data \
  -web-addr 0.0.0.0:8088 \
  -admin-password your_password \
  -partition-config ./cmd/server/partitions.json
```

### 3-node cluster (Docker)
```bash
# Leader (:50051, Studio :8088) + Follower-1 (:50052) + Follower-2 (:50053)
BEASTDB_ADMIN_PASSWORD=secret docker compose up -d --build
```
---

## `beastctl` CLI

Standalone binary for scripting, CI pipelines, and data migration — no browser required.

```bash
go build -o beastctl ./cmd/beastctl

beastctl ping   --addr 127.0.0.1:50051                        # Health check + latency
beastctl export --out backup.json --partition 0x01             # Snapshot by partition
beastctl import --file backup.json                             # Restore from snapshot
beastctl seed   --file examples/fixtures/seed.json             # Load fixtures with partition stats
beastctl get    --key 72057594037927936                        # Inspect a single record
beastctl put    --key 72057594037927936 --value '{"name":"x"}' # Write a record
beastctl delete --key 72057594037927936                        # Tombstone a record
```

---

## BeastDB Studio

An administrative console embedded directly in the binary — zero Node.js in production.

- **Firebase-Style 3-Pane Explorer** — Partition rail → Document list → Full field inspector
- **Partition-Aware Key Builder** — Pick a partition, enter domain/item seeds → auto-computes `(prefix << 56) | FNV28(domain) << 28) | FNV28(item)`. No manual bit-shifting.
- **64-Bit Key Bit-Slicer** — Decomposes any `uint64` key into prefix / domain / item segments with hex + binary view
- **Cascading User Deletion** — Atomic wipe across all partitions for a given user hash
- **Automatic Secret Redaction** — `passwordHash`, `salt`, `token`, `apiKey` stripped at parse time
- **Live Partition Donut Chart** — Ground-truth partition counts via key-only B+ Tree leaf scan (0 tuple I/O)

Provide a `partitions.json` sidecar to name and color-code your collections — no recompile needed.

---

## Engine Architecture

```mermaid
flowchart TD
    Client["Client\n(gRPC / Protobuf)"] --> GS["GRPCServer :50051\nHTTP/2 · Streaming RPC"]
    Browser["Admin\n(Browser)"] --> WS["Studio :8088\nEmbedded Next.js · Argon2id"]

    GS --> E["Engine\nRWMutex coordinator"]
    WS --> E

    E --> WAL["WAL\nCRC32 torn-write guard\nAppend-only · fsync"]
    E --> BPM["BufferPoolManager\nClock-Sweep eviction\nsync.Pool fast-path"]
    E --> BT["B+ Tree\nBinary search · epoch guard\nLeaf split / sibling rebalance"]

    BPM --> DM["DiskManager\n4KB aligned pages · fsync"]
    BT --> BPM
    BT --> CUR["Cursor\nHand-over-hand pinning"]

    subgraph Replication
        L["LeaderServer\nTwo-phase WAL stream"] --> BC["Broadcaster\nPub-sub fan-out"]
        BC --> F["FollowerClient\nCatch-up + live sync"]
    end

    E --> L
    F --> E
```

---

## What's Under the Hood

| Layer | Implementation |
|---|---|
| **Memory** | Zero-copy `[]byte↔string` via `unsafe`, cache-line-padded structs, FNV-1a hasher, open-addressing hash map, binary heap, ring buffer |
| **Network** | 10-byte binary TCP frame (`0xDB` magic + CRC32), zero-alloc RESP2 parser & writer |
| **Cache** | 64-shard partitioned `RWMutex` map, O(1) LRU eviction list, dual passive+active TTL heap, `sync.Pool` buffer recycling |
| **Storage** | 4KB hardware-aligned Slotted Pages (stable `RID`s, O(1) tombstone delete), `fsync`-safe Disk Manager, Clock-Sweep Buffer Pool |
| **Durability** | 23-byte binary WAL frames, IEEE CRC32 torn-write detection, ARIES crash recovery scanner, checkpointing |
| **Indexing** | On-disk B+ Tree — binary search routing, 50/50 leaf splits, borrow/merge rebalancing, streaming cursor with epoch detection |
| **API** | Protobuf schema + gRPC service — `Get`, `Put`, `Delete`, server-streaming `Scan` |
| **Replication** | Leader-follower physical WAL streaming — two-phase catch-up + live broadcaster, batched ACK coalescing |
| **Tooling** | `-dev` ephemeral emulator, `beastctl` CLI (export/import/seed/ping/crud), embedded Studio console |

---

## Benchmarks

Measured on **Intel Core i5-12450HX**, Go 1.26, Windows/amd64:

```
go test -bench=. -benchmem ./...
```

| Component | Operation | Latency | Allocs |
|---|---|---|---|
| Core DSA | Zero-copy `[]byte→string` | **0.30 ns/op** | 0 B · 0 |
| Core DSA | Vector Push | **1.37 ns/op** | 0 B · 0 |
| Core DSA | Open-Address Hash Get | **8.45 ns/op** | 0 B · 0 |
| Storage | Slotted Page Tuple Read | **8.01 ns/op** | 0 B · 0 |
| Index | B+ Tree Point Query | **180 ns/op** | 0 B · 0 |
| Index | Streaming Cursor (101 keys) | **1208 ns/op** (~12 ns/key) | 64 B · 1 |
| Cache | Sharded Concurrent Get | **51.1 ns/op** | 21 B · 1 |
| Durability | WAL Append + fsync | **2.35 µs/op** | 64 B · 1 |
| Network | TCP Frame Encode | **30.4 ns/op** | 48 B · 1 |
| API | gRPC End-to-End Get | **129 µs/op** | ~9 KB · 152 |

---

## B+ Tree vs LSM-Tree

BeastDB is optimized for **read-heavy OLTP** — deterministic point reads and ordered range scans. It is not designed for write-heavy append workloads.

| Dimension | BeastDB (B+ Tree) | RocksDB (LSM-Tree) |
|---|---|---|
| Point Read | `O(log N)` direct jump — **180 ns, 0 allocs** | MemTable + Bloom + multi-SSTable lookup |
| Range Scan | Sequential leaf traversal — **12 ns/key** | Multi-way merge-sort across sorted runs |
| Tail Latency | **Predictable** — no compaction spikes | Variable — subject to compaction write stalls |
| WAL Role | **Crash durability** (ARIES recovery for pages) | Primary ingest buffer (replays to MemTable) |

---
## Tests & Benchmarks
```bash
go test -count=1 ./...           # All packages including chaos and crash recovery
go test -bench=. -benchmem ./... # Reproducible system and zero-alloc benchmarks
```
---
## Client Integration & Contract
BeastDB publishes a Protobuf contract ([`api/proto/beastdb.proto`](api/proto/beastdb.proto)). See [`docs/clients.md`](docs/clients.md) for Go, Python, TypeScript, and Rust integration guides.
```go
conn, _ := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
client := beastv1.NewBeastDBServiceClient(conn)
client.Put(ctx, &beastv1.PutRequest{Key: 72057594037927936, Value: []byte(`{"name":"beast"}`)})
```

---

## 👤 Author

**Sheersh Jaiswal** · [@ChromaBeast](https://github.com/ChromaBeast)
