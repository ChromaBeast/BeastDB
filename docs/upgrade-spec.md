# BeastDB upgrade spec (security, usability, performance)

Source: review of 2026-11-12. Complements [ROADMAP.md](../ROADMAP.md) and [v1-semantics.md](v1-semantics.md). Claims here are goals, not shipped features.

## Findings that drive the work

| Area | Finding | Evidence |
|---|---|---|
| Perf | WAL fsyncs inside its mutex on every write; ~4 ms/write | `internal/wal/wal.go` Write |
| Perf | One global `sync.RWMutex`; scans hold it for the cursor lifetime | `internal/api/engine_ops.go` Scan |
| Perf | Benchmarks are in-process, 1 MB pool, 2k records; no competitor comparison | `docs/benchmarks.md` |
| Sec | gRPC auth checks token validity only; roles ignored | `internal/api/grpc_auth.go` |
| Sec | No encryption at rest (data, WAL, backups) | repo-wide search |
| Sec | Static token has no expiry/rotation | `grpc_auth.go` |
| UX | uint64 keys only, no namespaces, one Python example, no /metrics | proto, `examples/` |

## Work packages (parallel, disjoint files)

1. **WP1 WAL group commit** (`internal/wal`, write path in `internal/api`): batch concurrent writers into one fsync, same durability. Target: >=10x concurrent write throughput. Must keep crash/fault tests green.
2. **WP2 gRPC role authorization** (`internal/api/grpc_auth*.go`, token store): per-method roles (viewer = Get/Scan; admin/writer = mutations; replication role for StreamWAL/Ack), token expiry. Deny by default.
3. **WP3 YCSB-style harness** (new `cmd/beastbench`): workloads A-F over gRPC, reports throughput and p50/p95/p99, reproducible flags, doc update.
4. **WP4 Prometheus /metrics** (`internal/web/handler`): request/error/latency, WAL size and sync latency, dirty pages, checkpoint age, LSN.

## Next wave (not started)

Encryption at rest, string keys + namespaces, MVCC/latch-coupling to remove the global lock, SDKs (Go/TS/Python), PITR via WAL archive, CDC feed, tamper-evident audit log.

## Studio dashboard design

Figma design for a Firestore/Supabase-style console: project overview, collections/table editor, SQL-free query bar, API keys and roles, metrics, backups. Aimed at non-experts. Tracked in Figma; implementation in `studio/`.
