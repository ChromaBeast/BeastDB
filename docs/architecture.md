# BeastDB Architecture Specification

A modular, zero-allocation database architecture built from first principles in Go.

---

## 1. System Block Diagram

```
+-------------------------------------------------------------+
|                     Client Application                      |
|                  (Go SDK / Mobile Client)                   |
+------------------------------+------------------------------+
                               | (RESP / gRPC / Streaming)
+------------------------------v------------------------------+
|                     Network / API Layer                     |
|  - TCP Connection Handler & Non-blocking Listener           |
|  - RESP2 / RESP3 Zero-Allocation Parser                     |
|  - gRPC Query & Mutation Endpoints                          |
+------------------------------+------------------------------+
                               |
+------------------------------v------------------------------+
|                    Engine Coordinator                       |
|  - Sharded Mutex Manager (`sync.RWMutex`)                   |
|  - Atomic Ops Counter & Telemetry Metrics                   |
+--------------------+-------------------+--------------------+
                     |                   |
        (Read / Fast Path)         (Write Path)
                     |                   |
+--------------------v----+      +-------v--------------------+
|     In-Memory Cache     |      |  Write-Ahead Log (WAL)     |
| - Open-Addressing Map   |      | - Append-only Log File     |
| - Doubly Linked LRU     |      | - CRC32 Frame Checksums    |
| - TTL Expiration Timer  |      | - Crash Recovery Scanner   |
+-------------------------+      +-------+--------------------+
                                         |
+----------------------------------------v--------------------+
|                    Storage Engine                           |
|  - Buffer Pool Manager (Frames, Page Table, Clock Evictor)  |
|  - Disk Paging (4KB Slotted-Page Manager, Slot Array)       |
|  - On-Disk B+ Tree Index (Branching, Range Cursors)         |
+-------------------------------------------------------------+
```

---

## 2. Component Layers & Interactions

### Client to Network
1. Clients establish persistent TCP connections or gRPC streaming channels.
2. The network listener accepts connections and passes raw sockets to a bounded worker pool.
3. The RESP parser processes byte streams in-place into command frames without heap allocations.

### Write Execution Flow
1. **Group commit:** Writers enqueue their operation. One committer goroutine drains the queue, takes the engine lock exclusively, and for each queued write checks its condition, appends the record to the WAL (no `fsync` yet) and applies it in memory. It then issues **one** `fsync` for the whole group, notifies replication in LSN order, releases the lock, and only then acknowledges the writers. The lock is held across the `fsync` so readers and replicas never see a write before it is durable.
2. **Buffer Pool Pin:** The targeted 4KB disk page is fetched into a memory frame and pinned.
3. **Slotted Page Update:** The tuple is inserted into the slotted page, updating the slot array and free space pointer.
4. **Index Update:** If indexed, the B+ tree is traversed; keys and record pointers are updated, splitting nodes if page capacity is exceeded.
5. **Frame Unpin:** The page frame is marked dirty and unpinned. If the clock-sweep evictor later needs the frame, the page goes to the spill file; it reaches the data file only at the next checkpoint.

### Read Execution Flow
1. **Cache Check:** Point queries check the sharded in-memory cache first for sub-microsecond responses.
2. **Index Search:** If missed, the B+ tree is traversed from the root page down to the leaf page.
3. **Tuple Fetch:** The tuple offset from the leaf slot is retrieved from the slotted disk page via the Buffer Pool.

---

## 3. Storage Invariants
- **Checkpoint Image:** The data file only ever holds a complete checkpoint image. Dirty pages evicted between checkpoints go to a scratch spill file (`<db>-spill`), which is never fsynced and is discarded on restart, so evictions cost no `fsync`. A checkpoint (`FlushAll`) installs every dirty and spilled page atomically: it writes and fsyncs them all to a journal (`<db>-journal`, framed and CRC-checked), copies them into the data file, fsyncs it, and only then empties the journal. On open, a complete journal is re-applied and an incomplete one discarded. Recovery replays the WAL after the checkpoint LSN onto exactly that image; logical replay is not idempotent against pages that already hold later changes, which is why evicted pages must never reach the data file early.
- **Write-Ahead Invariant:** Log records must hit disk before dirty database pages containing those modifications are written. Because group commit applies a write to its page before the group's `fsync`, the buffer pool runs a **WAL barrier** (`WAL.EnsureDurable`) before a checkpoint writes pages to the data file. If the barrier fails, nothing is written. The barrier costs no extra `fsync` when the WAL is already durable up to the current LSN.
- **Page Isolation:** Every 4KB page maintains its own self-describing header, slot array, and CRC checksum.
- **Pin Safety:** A page frame with `pin_count > 0` cannot be evicted from the buffer pool under any circumstances.
