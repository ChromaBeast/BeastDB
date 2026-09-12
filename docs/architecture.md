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
1. **WAL Append:** Before modifying any page in memory, a log record is written to the WAL and synced (`fdatasync`).
2. **Buffer Pool Pin:** The targeted 4KB disk page is fetched into a memory frame and pinned.
3. **Slotted Page Update:** The tuple is inserted into the slotted page, updating the slot array and free space pointer.
4. **Index Update:** If indexed, the B+ tree is traversed; keys and record pointers are updated, splitting nodes if page capacity is exceeded.
5. **Frame Unpin:** The page frame is marked dirty and unpinned, allowing the background clock-sweep evictor to flush it later.

### Read Execution Flow
1. **Cache Check:** Point queries check the sharded in-memory cache first for sub-microsecond responses.
2. **Index Search:** If missed, the B+ tree is traversed from the root page down to the leaf page.
3. **Tuple Fetch:** The tuple offset from the leaf slot is retrieved from the slotted disk page via the Buffer Pool.

---

## 3. Storage Invariants
- **Write-Ahead Invariant:** Log records must hit disk before dirty database pages containing those modifications are written.
- **Page Isolation:** Every 4KB page maintains its own self-describing header, slot array, and CRC checksum.
- **Pin Safety:** A page frame with `pin_count > 0` cannot be evicted from the buffer pool under any circumstances.
