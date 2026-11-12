# BeastDB V1 Semantics & Platform Matrix

**Status:** Normative specification for BeastDB V1.

---

## 1. Data Model & Key Space

- **Key Format:** Unsigned 64-bit integer (`uint64`).
- **Value Format:** Opaque byte slice (`[]byte`).
- **Maximum Value Size:** 2 MiB (`MaxValueSize`). Values up to 4,000 bytes (`MaxRecordSize`) are stored inline in one slotted-page tuple; larger ones are chained across pages and checked with a CRC32 on read ([ADR 0001](adr/0001-large-values.md)). Larger payloads return `ErrRecordTooLarge`.
- **Batch Size:** the values in one `BatchWrite` may total at most 8 MiB (`MaxBatchBytes`, `ErrBatchTooLarge`). gRPC clients sending large values must raise the 4 MiB default message limit (`api.MessageSizeDialOption`).
- **Key Partitioning Convention:** Applications may optionally pack higher-order bits for table/collection prefixing (e.g., bits 56–63 for partition ID), but the engine treats keys as scalar `uint64` values.

---

## 2. Operations & Execution Contracts

### `Put(key uint64, value []byte)`
* **Durability Guarantee:** Synchronous write-ahead log (`WAL`) persistence. A write is acknowledged only after an `fsync` covering its WAL record completes successfully on disk.
* **Group commit:** Concurrent writers are queued and share one `fsync` per group. The durability guarantee is unchanged (no write is acknowledged before its record is durable, and readers and replication never observe a write before it is durable), but a group of N writers costs one `fsync` instead of N. A single sequential writer sees no speedup.
* **Failed fsync:** If the group `fsync` fails, the engine halts (fail-stop): every write in the group returns the error, later writes are rejected until restart, and the failure is sticky because retrying `fsync` after an error can report false success.
* **Overwrite Semantics:** If `key` exists, the B+ Tree record pointer (`RID`) is atomically updated to the new slotted page location.
* **Atomicity:** Single-key atomic mutation.

### `Delete(key uint64)`
* **Durability Guarantee:** Logged to WAL and synced before returning.
* **Missing Key Behavior:** Deleting a non-existent key is an idempotent no-op returning `nil`.
* **Atomicity:** The slot is marked dead in the slotted page and the key entry is purged from the B+ Tree.

### `BatchWrite(puts []PutOp, deletes []DeleteOp)`
* **Durability Guarantee:** The entire batch is serialized into a single `OpBatch` WAL frame and synced in a single I/O operation.
* **Atomicity:** All-or-nothing. On crash recovery, uncommitted or partial batches are rolled back; fully synced batches are replayed completely.
* **Limits:** Values in one batch may total at most 8 MiB; each value at most 2 MiB.

### Conditional writes: `Put`/`Delete` with `Condition`, `BatchWrite` with `conditions`
* **Conditions:** `ABSENT` (key must not exist), `PRESENT` (key must exist), `VALUE_EQUALS` (key must exist with exactly the given bytes). Omitted or `NONE` means unconditional.
* **Atomicity:** The check and the write happen under the same exclusive engine lock, so no other writer can interleave. For batches, every condition is evaluated before any operation applies: all hold and the whole batch commits, or none is written.
* **Failure:** gRPC status `ABORTED` ("precondition failed"); the caller should re-read and retry. Read-only replicas still return `FAILED_PRECONDITION`.
* **Durability and replication:** Only the resulting mutation is logged (a normal `OpPut`/`OpDelete`/`OpBatch` frame). Preconditions are never re-evaluated on recovery or on followers.
* **Scope:** Single node, single leader. A compare-and-swap loop (`Get`, then `Put` with `VALUE_EQUALS`) is safe across any number of client processes writing to the same leader.

### `Get(key uint64) ([]byte, bool, error)`
* **Consistency:** Read-your-own-writes on the leader.
* **Return Values:** Returns `(value, true, nil)` if found, or `(nil, false, nil)` if missing. Returns a non-nil error only upon hardware/storage failure.

### `Scan(startKey, endKey uint64) (*Cursor, error)`
* **Ordering:** Strictly ascending numerical order along the B+ Tree leaf chain.
* **Bounds:** Inclusive `[startKey, endKey]`.
* **Pagination (gRPC):** `ScanRequest.limit` caps the records returned (0 = unlimited). To resume, pass last returned key + 1 as `start_key`.
* **Resource Safety:** Cursors hold buffer pool page pins during traversal and **must** be closed via `cursor.Close()`.

---

## 3. Replication & Consistency Model

* **Architecture:** Single-leader with asynchronous streaming read replicas via gRPC (`ReplicationService`).
* **Follower Reads:** Read replicas service read-only queries (`Get`, `Scan`). Replicas strictly reject mutation operations.
* **Replication Guarantees:** 
  - Followers verify strictly contiguous log sequence numbers (`LSN`).
  - Unfillable gaps or buffer overflows trigger an immediate visible disconnect (`ErrReplicationGap` or `ResourceExhausted`), enforcing clean catch-up replay from the leader's disk WAL.
* **Freshness & Lag:** Replicas track `LastAppliedLSN`. Applications requiring bounded staleness may compare the replica's `LastAppliedLSN` against the leader's acknowledged LSN.

---

## 4. Authorization (gRPC)

Authentication is a Bearer token: the static `--api-token` (treated as `admin`) or an API token from the token store. Authorization is **deny by default** per method:

| Role | Allowed methods |
|---|---|
| `viewer` | `Get`, `Scan` |
| `writer` | `Get`, `Scan`, `Put`, `Delete`, `BatchWrite` |
| `admin` | all client methods and `ReplicationService` |
| `replication` | `ReplicationService.StreamWAL`, `ReplicationService.Ack` only |

- A missing, malformed, unknown or expired token returns `UNAUTHENTICATED`. A valid token whose role may not call the method returns `PERMISSION_DENIED`.
- Any gRPC method not in the role map is denied for every role, including `admin` and the static token. Register new methods in `methodRoles` (`internal/api/grpc_auth.go`).
- A stored token with a role outside the four above can call nothing.
- Tokens may carry an optional expiry (`expires_at`); a token is rejected once the current time reaches it. Tokens created before expiry support never expire.
- With no static token and no token store, the interceptor is open (development only).

---

## 5. Supported Platform & Deployment Matrix

| Component | Supported Version / Target | Notes |
|---|---|---|
| **Go Runtime** | Go 1.26.x (pinned in go.mod) | Required for building engine and CLI binaries |
| **Node / Bun** | Bun 1.1+, Node.js 20 LTS | Required for Studio build and web assets |
| **Linux** | Ubuntu 22.04+, Debian 12+, RHEL 9+ | Recommended for primary production deployments |
| **Windows** | Windows 10/11, Windows Server 2019+ | Fully supported; test suite validated |
| **macOS** | macOS 13+ (Ventura, Sonoma, Sequoia) | Supported for local development |
| **Filesystems** | ext4, xfs, NTFS, APFS | Must support `fsync`/`fdatasync`. Network shares (NFS/SMB) discouraged |
