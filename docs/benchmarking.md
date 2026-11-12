# Benchmarking BeastDB with `beastbench`

`cmd/beastbench` is a reproducible, YCSB-style load generator that drives a
BeastDB server over gRPC. This document explains how to run it and how to run
*equivalent* workloads against Redis, Pebble and Badger so that a comparison is
fair. It deliberately contains no results: numbers depend on your hardware,
settings and dataset, and only numbers you measured yourself are meaningful.

## Running beastbench

```
go build -o beastbench ./cmd/beastbench

# against a running server (flags follow cmd/beastctl: token and TLS)
./beastbench --addr 127.0.0.1:50051 --token "$BEASTDB_API_TOKEN" \
  --workload A --dist zipfian --records 10000000 --ops 5000000 \
  --threads 64 --value-size 1024 --warmup 200000 --seed 42 --json > a.json

# TLS / mTLS
./beastbench --addr db:50051 --tls-ca ca.pem --tls-cert client.pem --tls-key client-key.pem ...

# quick smoke run: in-process engine + gRPC on a temp dir (loopback, no auth)
./beastbench --self-contained --workload B --records 5000 --ops 5000 --threads 4
```

Flags: `--addr --token --tls-ca --tls-cert --tls-key --tls-server-name --conns`
(connection), `--workload A-F`, `--dist zipfian|uniform`, `--zipf-theta`,
`--records --ops --warmup --threads --value-size --max-scan-len --seed`,
`--json`, `--skip-load`, `--load-only`, `--self-contained --pool-size`.
`--token` defaults to `$BEASTDB_API_TOKEN`, the TLS flags to `$BEASTDB_TLS_*`.

### Phases

1. **Load**: inserts `--records` records (keys 1..N, deterministic values of
   `--value-size` bytes) in parallel. Reported separately.
2. **Warmup** (optional): `--warmup` unmeasured operations of the chosen mix.
3. **Run**: `--ops` measured operations split across `--threads` workers.

Each worker has its own RNG seeded from `--seed`, so a given
(seed, threads, ops) produces the same operation and key sequence. Timing and
server behaviour still vary run to run; repeat runs and report the spread.

### Workloads

| Name | Mix | Keys |
|---|---|---|
| A | 50% read, 50% update | zipfian / uniform |
| B | 95% read, 5% update | zipfian / uniform |
| C | 100% read | zipfian / uniform |
| D | 95% read, 5% insert | read-latest (zipfian offset from newest insert) |
| E | 95% scan (length uniform 1..`--max-scan-len`), 5% insert | zipfian / uniform start key |
| F | 50% read, 50% read-modify-write | zipfian / uniform |

Workload F's read-modify-write is `Get` followed by `Put` with a
`VALUE_EQUALS` condition on the value read. A rejected condition (ABORTED) is
counted as a conflict, not an error, and is included in the `rmw` latency.
Latency for `rmw` covers both RPCs.

Notes on semantics:

- Zipfian uses the YCSB generator (theta 0.99 by default). Keys are *not*
  hash-scrambled, so the hottest keys are the lowest ones; this keeps scans
  contiguous but concentrates the hot set in one region of the B+ tree.
- Workloads D/E allocate new keys above the loaded range. A D read may hit a key
  whose insert is still in flight; that is reported as a `miss`.
- Percentiles come from a log-linear histogram (128 sub-buckets per power of
  two, under 1% error); max is exact. Latency is client-observed per call,
  including gRPC and network time. There is no coordinated-omission correction:
  workers are closed-loop, so a stalled server lowers offered load. Say so when
  you report tail latencies.

### Output

Text by default; `--json` emits config, host info (Go version, OS, CPUs,
self-contained flag) and per-phase, per-operation count, errors, misses,
conflicts, ops/s, mean, p50, p95, p99 and max in microseconds. Keep the JSON
with any number you quote.

## Fair comparison against Redis, Pebble and Badger

A comparison is only as honest as its weakest control. Fix all of the following
and record them next to results.

### Environment

- **Same machine, same disk, run sequentially**, never concurrently. Record CPU
  model, core count, RAM, disk model and filesystem, OS and kernel, Go version
  and every system's version/commit.
- Disable CPU frequency scaling surprises (performance governor), keep the
  machine otherwise idle, and use a fresh data directory per run on the same
  device.
- **Dataset larger than RAM.** If the dataset fits in the page cache you are
  benchmarking memory, not the storage engine. Choose `records x (value size +
  per-key overhead)` at least 2-4x physical RAM (or constrain RAM with a
  cgroup/container memory limit and state it). Check with `free`/`vmstat` that
  the page cache did not absorb the working set, and drop caches between
  phases if you intend a cold start (`sync; echo 3 > /proc/sys/vm/drop_caches`).
- State the **cache budgets** explicitly and make them comparable: BeastDB
  `--pool-size` (4 KiB frames), Pebble block cache, Badger block/index cache,
  Redis `maxmemory`. Do not leave one system on defaults while tuning another.
- Same key count, key size (BeastDB keys are `uint64`; use 8-byte keys
  elsewhere, big-endian so ordering matches), value size, and value contents.
  Values from `beastbench` are incompressible-ish pseudo-random bytes; disable
  or equalise compression (Pebble/Badger default compression off or on must
  match; Redis stores raw).

### Durability: the biggest source of unfair numbers

Compare like with like. State what each system guarantees when an
acknowledged write is followed by a power loss.

| System | Per-write durable (fsync on ack) | Relaxed |
|---|---|---|
| BeastDB | Each write is acknowledged only after the WAL is synced (`docs/v1-semantics.md`); benchmark as shipped | no relaxed mode is documented; document any change you make |
| Redis | `appendonly yes`, `appendfsync always` | `appendfsync everysec`, or no AOF (RDB only) |
| Pebble | `pebble.Sync` on every write (`WriteOptions{Sync: true}`) | `pebble.NoSync` |
| Badger | `SyncWrites: true` | `SyncWrites: false` (default) |

Run the per-write-durable configuration for all systems as the primary
comparison, and report any relaxed configuration as a separate, clearly
labelled table. A durable system compared against a non-durable one tells you
nothing. Also note that group commit/batching behaviour differs and is part of
what is being measured when many threads write concurrently.

### Matching the access path

- `beastbench` measures a *networked* client-server round trip over gRPC. Redis
  has a network protocol too (use `redis-benchmark` or a small RESP client,
  no pipelining unless you also pipeline everywhere). Pebble and Badger are
  embedded libraries with no network hop; comparing them to a gRPC call
  mostly measures the RPC. Either (a) wrap them in an equivalent gRPC/TCP
  service and run the same `--threads`, or (b) benchmark BeastDB's engine
  in-process (`go test ./internal/api -bench`) and compare to the embedded
  libraries in-process. Do not mix the two.
- Use the same concurrency (`--threads`) and report results at several
  concurrency levels, not one.
- Same operation mix, key distribution, warmup, run length, and seed-derived
  key sequence. Implement the equivalent of each workload in the other
  system's harness: A/B/C/D as get/put, E as an iterator seeking to the start
  key and reading up to N entries (Redis: a sorted-set or key range scheme and
  say which), F as get then a conditional/transactional write (Redis
  `WATCH`/`MULTI` or a Lua CAS; Pebble/Badger: a transaction or CAS loop).
  Document any place the semantics differ.
- Load the dataset the same way (same ordering and parallelism) and let
  background work finish (compaction in Pebble/Badger, checkpoint in BeastDB,
  `BGSAVE`/AOF rewrite in Redis) before the measured phase, or measure it
  deliberately and say so.

### Existing harnesses

You may use YCSB itself (Redis binding exists; Pebble/Badger need a custom or
community binding) or `redis-benchmark`. If you do, map its parameters to the
above (recordcount, operationcount, fieldcount=1, fieldlength, request
distribution, threadcount) and note that YCSB's latency measurement differs
from `beastbench`'s.

### Reporting

- Publish the exact command lines, config files, versions, hardware and the raw
  JSON/CSV outputs.
- Report several repetitions (at least 5) with median and range, not a single
  best run.
- Report p50/p95/p99/max latency, not only throughput, plus error and conflict
  counts.
- State what you did *not* control. Do not generalise from one workload,
  dataset size or machine; do not claim one system is faster in general.

### Smoke runs are not benchmarks

`--self-contained` runs client and server in one process on loopback, with
whatever disk the temp dir lands on and no auth or TLS. Use it to validate the
harness and catch regressions in your own environment only.
