# BeastDB V1 roadmap

**Status:** proposed release plan, 28 September 2026. This succeeds the completed [16-week engine build plan](docs/roadmap-16-week.md). Planned work below is not a shipped feature or guarantee.

## Release promise

V1 is a dependable, self-hosted, read-heavy **uint64-key / byte-value OLTP database** with one writable leader, optional asynchronous read replicas, a gRPC API, CLI, and embedded administrative Studio. A successful acknowledged write must survive a single-node process crash and restart under the documented durability mode. Replica reads may lag; leader loss requires an operator-guided recovery or promotion procedure.

V1 does not promise zero data loss on leader failure, automatic failover, consensus, multi-leader writes, SQL, joins, arbitrary transactions, or a hosted service. The protobuf package name beastdb.v1 is an API namespace, not evidence that this release is complete.

## Current baseline

- On-disk B+ tree, slotted pages, WAL/recovery, checkpoints, snapshot function, gRPC Get/Put/Delete/Scan/BatchWrite, CLI export/import, leader-follower WAL streaming, token auth, and Studio exist.
- Unit, crash-recovery, snapshot, migration, concurrency, and basic replication tests exist. The current [README](README.md) reports component benchmarks; V1 still needs reproducible system-level results.
- Storage metrics are available in Studio, but release-grade health, alerting, restore drills, and compatibility checks are not yet demonstrated.

## V1 release gates

Release only when each **must-pass** gate has evidence on the release commit. Narrow the published V1 promise before release if a gate is deferred. Every milestone ends with a runnable demo, tests, and documentation; unit tests alone do not close a milestone.

| Gate | Required evidence |
|---|---|
| Data integrity | Model-based tests and repeated crash/fault campaigns show no lost acknowledged writes, phantom records, broken scans, or index/data divergence on a single node. State fsync and filesystem assumptions. |
| Backup and recovery | Backup/restore CLI and runbook restore a populated database on a clean host, with verified manifest/checksums and measured recovery time. Restore runs in CI. |
| Replication | Followers catch up exactly across concurrent writes, reconnects, slow consumers, WAL rotation, and bootstrap from an older snapshot. Unfillable gaps fail visibly and require resync. |
| Security | Production startup rejects default credentials and unauthenticated public listeners; gRPC client and replica traffic supports TLS; write paths enforce roles. |
| API compatibility | Documented protobuf and disk-format policy; supported older clients work against V1; upgrade and rollback tests cover the previous supported release. |
| Operability | Health/readiness, metrics, structured errors, alerts, and recovery runbooks work in the reference deployment. |
| Performance | Reproducible mixed workloads report throughput, p50/p95/p99 latency, CPU, memory, disk, WAL growth, restart time, and replica lag at several data sizes. |
| Release quality | CI is green on supported platforms; a fresh user can install, load, back up, restore, and upgrade using versioned docs and packaged artifacts. |

## Milestones

Order is dependency-based, not a calendar promise. Initial planning range for one focused maintainer: **16–24 weeks**, revised after M0. Integrity and recovery work takes priority over UI expansion.

| Milestone | Focus | Exit artifact |
|---|---|---|
| M0 — Contract and baseline | Scope, semantics, CI, workload harness | Specification and reproducible baseline |
| M1 — Single-node correctness | WAL, pages, index, concurrency, crash recovery | Fault-tested engine and invariants report |
| M2 — Recoverability | Backups, restore, retention, upgrades | Restore-tested release candidate |
| M3 — Replication | Gap-free catch-up, resync, promotion | Fault-tested optional replica mode |
| M4 — Secure operations | TLS, auth, health, metrics, deployment | Hardened reference deployment and runbooks |
| M5 — Developer experience | API contract, CLI, Studio, examples | Beta with complete user journey |
| M6 — Release validation | Scale tests, compatibility, docs, packaging | Signed-off V1 release |

### M0 — Contract and baseline

- Define exact semantics for Put, Delete, BatchWrite, Get, and Scan: durability acknowledgment, atomicity, overwrite/delete behavior, scan ordering and bounds, concurrency, cancellation, value/batch limits, and errors. State whether follower reads are supported and how freshness is reported.
- Inventory existing tests and run uncached Go tests, race detector, Studio tests/build, vet/static checks, and clean Docker build in CI. Pin supported Go, Node/Bun, and OS versions.
- Build a reference workload generator. Publish hardware, dataset, request mix, concurrency, cache state, and commands with every performance result. Set measurable thresholds from baseline runs.
- Open tracked issues for each gate, with reproduction, owner, acceptance test, and evidence link. Correct README claims that exceed the measured contract.

**Exit:** A reviewer reproduces the baseline and can state the V1 guarantees in one page.

### M1 — Single-node correctness and durability

- Audit metadata, WAL, page, and close errors. CreateSnapshot, Checkpoint, and Close currently discard some metadata/flush errors; make failures visible and safe. Specify behavior when WAL sync succeeds but page/index application fails.
- Audit Put overwrite, missing-key Delete, duplicate keys, batch replay, record-size limits, B+ tree split/merge, and secondary-index rebuild. Compare randomized operations against a simple reference map after each restart.
- Inject failures around WAL write/sync, page flush, metadata update, checkpoint rename, and process exit. Repeat kill/restart and torn-WAL tests on Windows and Linux.
- Define a corruption policy: detect invalid pages/WAL, fail closed with actionable diagnostics, and provide an integrity-check command. Verify checkpoints retain every record needed for replay.
- Bound scan resources and held locks; test concurrent reads, writes, scans, cancellation, and shutdown for deadlock and starvation.

**Exit:** Published invariants and fault matrix pass repeatedly; acknowledged writes survive the tested single-node crash model.

### M2 — Recoverability and upgrades

- Make online physical snapshots atomic and verifiable: temporary path, file/directory sync where supported, checked metadata errors, format/LSN/checksums, then publication. Keep enough WAL for the declared restore point.
- Add beastctl backup, verify, and restore flows with destination checks and no silent overwrite. Keep logical export/import as a separate portability path.
- Set backup schedule, retention, off-host copy, restore-time goal, and recovery-point goal for the reference deployment. Measure them in drills. Point-in-time recovery is required only if the V1 promise explicitly offers it; otherwise document snapshot-based recovery and its data-loss window.
- Write disk-format migration and rollback rules; test old data files, mixed-version clients, interrupted upgrades, and restore of a pre-upgrade backup.

**Exit:** An operator restores populated data on a clean host with automated verification and measured time/data loss.

### M3 — Replication and controlled recovery

- Repair replay-to-live handoff: the current leader subscribes **after** historical WAL replay, leaving a write gap. Preserve a continuous log position and test writes during catch-up.
- Replace silent broadcaster drops with explicit slow-follower disconnect plus replay, or bounded backpressure. Followers verify contiguous LSNs and reject gaps.
- Make LSNs durable and monotonic across WAL rotation/restart. Define retention and snapshot bootstrap when a follower is too far behind. Replicate atomic batches as one unit rather than separate records sharing an LSN.
- Ensure followers reject ordinary writes; test auth, reconnect, replay idempotence, restart, and data equality at a common LSN.
- Document manual failover: fence old leader, check follower LSN, promote one node, redirect clients, and rejoin old node through resync. State the asynchronous data-loss window.

**Exit:** Fault tests show no silent divergence; rehearsed promotion/rejoin has measured RPO and RTO.

### M4 — Secure operations

- Reject production startup with admin/admin or public gRPC without auth. Keep loopback-only development mode. Remove default credentials from production Compose.
- Support TLS for gRPC clients and replica links; verify peer identity for replication and document certificate rotation. Validate Studio proxy HTTPS settings, cookies, CSRF protections, token lifecycle, and destructive-action audit logs.
- Define viewer/admin permissions across gRPC, HTTP, Studio, and CLI. Test denied actions and rate limits for login and expensive scans.
- Expose liveness, readiness, and metrics for request/error/latency rates, WAL size/sync latency, dirty/pinned pages, disk free space, checkpoint age, follower/leader LSNs, and gap/resync state. Add alerts and runbooks.
- Harden deployment: non-root container, persistent-volume permissions, resource limits, clean SIGTERM shutdown, startup checks, and TLS-enabled Compose example.

**Exit:** Fresh production configuration is secure by default; an operator can detect and handle unhealthy or lagging nodes.

### M5 — Developer and Studio experience

- Stabilize protobuf fields and status codes, validate inputs/limits, publish Go/Python/TypeScript examples, and test examples in CI. Document key layout as an optional convention rather than an engine requirement.
- Make beastctl scripting reliable: stable exit codes, machine-readable output, timeouts, TLS/token config, and safe restore prompts.
- Finish Studio pagination in Table/Gallery, normalize status filtering, label loaded-record counts honestly, and test keyboard/screen-reader access. Keep engine internals available for diagnostics.
- Write one end-to-end tutorial: install → authenticate → write/read/scan → back up → restore → inspect in Studio → upgrade.

**Exit:** A new developer completes the tutorial without reading source; CLI and Studio agree with API results on a dataset larger than one page.

### M6 — Release validation and launch

- Benchmark mixed reads/writes/scans at small, medium, and larger-than-RAM datasets. Publish throughput, tail latency, resource use, restore/failover time, and limits. Compare only equivalent end-to-end workloads.
- Run soak, disk-full, network-partition, slow-follower, crash-loop, and upgrade/rollback campaigns. Record failures and rerun release blockers after fixes.
- Build binaries/images for supported targets with checksums, SBOM, dependency scan, changelog, sample config, and versioned docs. Exercise the exact release-candidate artifacts in a clean environment.
- Freeze features, triage open severity-1/2 defects, and sign off each gate with an evidence index. Publish known limitations and maintenance patch plan.

**Exit:** V1 is tagged only from the tested release commit.

## First iteration

1. Write the [V1 semantics page](docs/v1-semantics.md) and supported deployment/OS matrix.
2. Reproduce metadata error paths and the replication replay/live gap; add targeted regression tests.
3. Add clean CI and a reference-map fault harness; capture the baseline.
4. Run the first clean-host backup/restore drill and record actual RPO/RTO.

## References

- [PostgreSQL WAL archiving and recovery](https://www.postgresql.org/docs/17/continuous-archiving.html) — backup/restore design reference, not a BeastDB feature claim.
- [gRPC authentication and TLS](https://grpc.io/docs/guides/auth/) — transport security guidance.
- [Jepsen consistency models](https://jepsen.io/consistency/models) — language for precise guarantees.
- [OpenTelemetry observability primer](https://opentelemetry.io/docs/concepts/observability-primer/) — metrics, logs, and tracing concepts.
