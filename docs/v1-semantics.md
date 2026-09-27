# BeastDB V1 Semantics & Platform Matrix

**Status:** Normative specification for BeastDB V1.

---

## 1. Data Model & Key Space

- **Key Format:** Unsigned 64-bit integer (`uint64`).
- **Value Format:** Opaque byte slice (`[]byte`).
- **Maximum Record Size:** 4,000 bytes per record (bounded by 4KB slotted page capacity). Payloads exceeding this limit return `ErrRecordTooLarge`.
- **Key Partitioning Convention:** Applications may optionally pack higher-order bits for table/collection prefixing (e.g., bits 56–63 for partition ID), but the engine treats keys as scalar `uint64` values.

---

## 2. Operations & Execution Contracts

### `Put(key uint64, value []byte)`
* **Durability Guarantee:** Synchronous write-ahead log (`WAL`) persistence. A write is acknowledged only after `file.Sync()` (`fdatasync`) completes successfully on disk.
* **Overwrite Semantics:** If `key` exists, the B+ Tree record pointer (`RID`) is atomically updated to the new slotted page location.
* **Atomicity:** Single-key atomic mutation.

### `Delete(key uint64)`
* **Durability Guarantee:** Logged to WAL and synced before returning.
* **Missing Key Behavior:** Deleting a non-existent key is an idempotent no-op returning `nil`.
* **Atomicity:** The slot is marked dead in the slotted page and the key entry is purged from the B+ Tree.

### `BatchWrite(puts []PutOp, deletes []DeleteOp)`
* **Durability Guarantee:** The entire batch is serialized into a single `OpBatch` WAL frame and synced in a single I/O operation.
* **Atomicity:** All-or-nothing. On crash recovery, uncommitted or partial batches are rolled back; fully synced batches are replayed completely.
* **Limits:** Recommended batch limit of 1,000 operations or 2 MB aggregate payload.

### `Get(key uint64) ([]byte, bool, error)`
* **Consistency:** Read-your-own-writes on the leader.
* **Return Values:** Returns `(value, true, nil)` if found, or `(nil, false, nil)` if missing. Returns a non-nil error only upon hardware/storage failure.

### `Scan(startKey, endKey uint64) (*Cursor, error)`
* **Ordering:** Strictly ascending numerical order along the B+ Tree leaf chain.
* **Bounds:** Inclusive `[startKey, endKey]`.
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

## 4. Supported Platform & Deployment Matrix

| Component | Supported Version / Target | Notes |
|---|---|---|
| **Go Runtime** | Go 1.22.x, 1.23.x, 1.24.x | Required for building engine and CLI binaries |
| **Node / Bun** | Bun 1.1+, Node.js 20 LTS | Required for Studio build and web assets |
| **Linux** | Ubuntu 22.04+, Debian 12+, RHEL 9+ | Recommended for primary production deployments |
| **Windows** | Windows 10/11, Windows Server 2019+ | Fully supported; test suite validated |
| **macOS** | macOS 13+ (Ventura, Sonoma, Sequoia) | Supported for local development |
| **Filesystems** | ext4, xfs, NTFS, APFS | Must support `fsync`/`fdatasync`. Network shares (NFS/SMB) discouraged |
