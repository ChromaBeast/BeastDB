# BeastDB roadmap plan: from V1 gates to a general-purpose database

**Status:** proposed, 28 October 2026. Builds on [ROADMAP.md](../ROADMAP.md) (V1 gates M0–M6, first iteration done). Nothing below is shipped until it has tests and evidence on the release commit.

**Principle:** BeastDB is domain-agnostic. Every item below is a general primitive or operational capability. Application schemas, partition names and key layouts belong to client projects (`partitions.json`, `-partition-config`), never to the engine.

## Where BeastDB stands

Done: B+ tree storage, WAL with CRC and recovery, atomic batch writes, checkpoints, leader–follower WAL streaming, token auth, TLS/mTLS, health and readiness probes, backup/restore CLI with a measured drill, Studio, CI, benchmarks.

Gaps that real clients keep hitting, all generic:

| Gap | Why it matters |
|---|---|
| No conditional or compare-and-swap writes | Read-modify-write races (counters, token rotation, uniqueness, "create if absent") cannot be made safe across more than one writer process. Clients fall back to process-local locks, which limits them to one writer. |
| Batches cannot carry preconditions | Multi-key invariants (move value A→B, swap an index entry) can't be enforced atomically. |
| 64-bit keys only, caller hashes identifiers | Hashing a string ID to fewer bits risks silent collisions. Clients need either wider keys or a safe pattern. |
| Scans are unbounded streams without resume tokens | Large ranges risk memory and lock pressure; clients cap results ad hoc. |
| No expiry (TTL) | Sessions, caches and job records must be swept by hand. |
| No change feed | Clients poll to learn about writes; replication already has a WAL stream that could be exposed. |
| Single role model | One shared token per service, no read-only or scoped tokens. |
| Backups are manual | No schedule, retention or off-host copy. |
| No official client libraries | Each application re-implements dialing, retries, auth metadata and key helpers. |
| Failover is manual | Documented, but operator-driven. |

## Phases

Order is dependency-based. Each phase ends with a runnable demo, tests and docs.

### Phase A — Safe concurrent writes (highest value)

1. **Conditional write primitive.** Add `expected` preconditions to `Put`/`Delete`: *absent*, *present*, or *value equals / version equals*. Return gRPC `ABORTED` (the idiomatic code for retryable concurrency conflicts). Implemented in the engine under the existing write lock, logged as normal WAL records, replicated unchanged.
2. **Conditional `BatchWrite`.** A batch carries a list of preconditions evaluated before any operation applies; all pass or nothing is written.
3. **Per-record version (optional).** A monotonically increasing version returned by `Get`/`Put`, usable as a precondition (optimistic concurrency without comparing whole values).
4. **Tests.** Model-based test against a reference map with concurrent writers; crash and replay tests prove preconditions are not re-evaluated on recovery or replication; race detector clean.
5. **API compatibility.** New protobuf fields only (additive); old clients keep working. Document in [v1-semantics.md](v1-semantics.md).

**Status: implemented (conditional Put/Delete/BatchWrite; optional per-record versions deferred: `VALUE_EQUALS` covers the same need without an on-disk format change).**

**Exit:** two independent processes can increment a counter, rotate a token or claim a unique name with no lost updates, shown by a stress test.

### Phase B — Scan and key ergonomics

1. **Large values:** values up to 2 MiB, chained across pages with CRC checks, and 8 MiB batches are **implemented** ([ADR 0001](adr/0001-large-values.md)).
2. **Paginated scans:** `limit` on `ScanRequest` and resume by last key + 1 are **implemented**. A server-enforced maximum page size and reverse/prefix scans remain.
3. **Reverse and prefix scans** over the uint64 range, with documented bounds semantics.
4. **Optional wide-key mode design.** Options to evaluate and decide with a written ADR: (a) keep uint64 and ship a documented collision-safe pattern plus a client helper, or (b) add a variable-length key space. Decision gated on benchmarks and on-disk-format migration cost.
5. **Key-layout helper library** (client side, optional convention, not required by the engine).

**Exit:** a client can page through a 10M-key range with bounded memory on both sides.

### Phase C — Lifecycle features

1. **TTL.** Per-record expiry timestamp checked on read and removed by a background sweeper; expiry is deterministic and replicated.
2. **Change feed.** Expose the committed WAL (or a filtered key range) as a server stream with resume position, reusing replication's contiguous-LSN guarantees. Documented delivery semantics: at-least-once, ordered per feed.
3. **Bulk import** with idempotent upserts and progress reporting (extends `beastctl import`).

**Exit:** an expiring record disappears on schedule on leader and follower; a consumer resumes a feed after restart without gaps.

### Phase D — Security and multi-tenancy primitives

1. **Scoped tokens.** Roles (read-only, read-write, admin) and optional key-range scopes per token, enforced in gRPC, HTTP, Studio and CLI.
2. **Token lifecycle:** create, rotate, revoke, audit log of administrative actions; denied-action tests.
3. **Rate limits** on login and expensive scans.
4. **Verified TLS defaults** in the reference Compose file and certificate-rotation runbook.

**Exit:** a service holding a read-only, range-scoped token provably cannot write or read outside its range.

### Phase E — Operations

1. **Metrics endpoint** (Prometheus-compatible) covering request/error/latency, WAL size and sync latency, dirty/pinned pages, checkpoint age, free disk, leader/follower LSNs and lag.
2. **Scheduled backups** with retention, checksum verification and optional off-host copy; restore drill runs in CI.
3. **Promotion tooling:** `beastctl promote`/`fence` with LSN checks, measured RPO/RTO. Automatic failover only after consensus research (out of V1).
4. **Integrity check command** that validates pages, WAL and index/data consistency, failing closed with diagnostics.

**Exit:** an operator is alerted to a lagging replica and completes a rehearsed promotion within the stated RTO.

### Phase F — Developer experience

1. **Official clients:** Go, TypeScript and Python packages with connection management, retry/backoff, auth, TLS, conditional-write helpers, pagination iterators and an in-memory test double that matches server semantics.
2. **`beastctl`:** stable exit codes, JSON output, timeouts, safe restore prompts.
3. **Studio:** finish pagination and filters everywhere, honest loaded-record counts, keyboard and screen-reader support.
4. **One end-to-end tutorial** from install to upgrade, tested in CI.

**Exit:** a new developer completes the tutorial without reading source.

### Phase G — Release validation (V1 → V1.x)

Mixed read/write/scan benchmarks at several data sizes, soak, disk-full, partition and crash-loop campaigns, upgrade/rollback tests, signed artifacts with SBOM, evidence index per gate. Features are frozen before the tag.

## How this improves BeastDB

| Change | Improvement |
|---|---|
| Conditional writes and batches | Removes the biggest correctness limit: multiple writer processes become safe. Counters, uniqueness, token rotation and index maintenance stop depending on application-side locks. |
| Pagination and scan limits | Bounds memory and lock time, so a wide scan can no longer degrade other clients. |
| Key ergonomics decision | Makes silent key collisions a documented, tested concern instead of each client's private risk. |
| TTL | Sessions, caches and job records clean themselves; storage no longer grows from forgotten records. |
| Change feed | Clients react to writes without polling, which supports notifications, cache invalidation and downstream indexing. |
| Scoped tokens and audit | Least-privilege per service; a leaked read-only token can't modify data. |
| Metrics, scheduled backups, integrity check | Failures are detected and recoverable before users notice; recovery time and data-loss window are measured. |
| Official clients | One tested implementation of dialing, retries and semantics replaces per-application copies, cutting integration bugs. |
| Evidence-gated release | Claims in the README match measured behavior, so users can trust the guarantees. |

## Non-goals for V1/V1.x

SQL, joins, multi-key interactive transactions, automatic failover or consensus, multi-leader writes, hosted service. Conditional batches cover single-node atomic multi-key updates only.

## Suggested sequencing and risk

1. Phase A first: it unblocks safe multi-writer deployments and is small relative to its value.
2. Phases B and E can proceed in parallel after A (separate files and subsystems); C and D follow.
3. Phase F runs alongside, updating clients as each primitive lands.
4. Highest risks: on-disk format changes (versions, TTL, wide keys) need migration and rollback tests; WAL record changes need replication compatibility tests. Mitigation: ADR per format change, feature flags, mixed-version tests before release.
