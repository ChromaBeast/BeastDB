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

## Work packages

| WP | Scope | Status |
|---|---|---|
| WP1 | **WAL group commit** (`internal/wal`, `internal/api/engine_commit.go`): concurrent writers share one fsync with unchanged durability. Reported ~15x on a concurrent-write benchmark; a single sequential writer is unchanged. Includes a **WAL barrier** in the buffer pool so a data page never reaches disk ahead of its log record. | Merged |
| WP2 | **gRPC role authorization** (`internal/api/grpc_auth.go`, token store): deny-by-default per-method roles `viewer`/`writer`/`admin`/`replication`, optional token expiry. | Merged. No handler test yet for `expires_in_seconds`; Studio has no expiry field. |
| WP3 | **`beastbench`** (`cmd/beastbench`): YCSB-style workloads A–F over gRPC, latency histogram, JSON output, self-contained smoke mode, [benchmarking guide](benchmarking.md). | Merged. No competitor comparison has been run. |
| WP4 | **Prometheus `/metrics`**: request counts and latency, WAL size and LSN, checkpoint age, buffer pool, disk free. Never anonymous; accepts an admin session or a metrics/static bearer token. See the [operator guide](runbooks/operator-guide.md). | Merged |

## Known limits of what shipped

- Readers and replication are still blocked for the duration of each group's fsync (the engine keeps one global lock). Lifting this needs MVCC or latch coupling.
- Evicted dirty pages go to a non-durable spill file until the next checkpoint, so the spill file grows with the number of distinct pages dirtied since then. Auto-checkpoint bounds it along with the WAL.
- `LeaderServer.StreamWAL` replays the WAL file from disk and can briefly read a record that is appended but not yet fsynced.
- `TestEngineSystemWorkloadBenchmark` has a fixed 1000 ops/s floor and is timing-sensitive on a loaded machine.
- Checkpoint LSN/age metrics only appear after the first checkpoint since process start; the buffer pool hit ratio is cumulative.

## Next wave (not started)

Encryption at rest, string keys + namespaces, MVCC/latch-coupling to remove the global lock, SDKs (Go/TS/Python), PITR via WAL archive, CDC feed, tamper-evident audit log, Studio expiry field and token-API handler tests.

## Studio dashboard design

A Firestore/Supabase-style console for non-experts is designed in Figma; the implementation in `studio/` has not been changed. See [Studio design system](studio.md#design-system-and-planned-screens).
