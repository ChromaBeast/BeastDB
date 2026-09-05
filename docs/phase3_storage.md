# Phase 3: Storage Engine & Indexing (Weeks 8–12)

Implements the persistent on-disk architecture: Write-Ahead Logging (WAL), 4KB slotted-page disk management, buffer pool caching, and an on-disk B+ Tree.

---

## Week 8: Write-Ahead Logging (WAL) & Crash Recovery

### Format & Durability
Every state mutation must be appended to the log before modifying dirty pages in memory.
```
+---------------+---------------+---------------+---------------+---------------+---------------+---------------+
| RecLen (4B)   | LSN (8B)      | Type (1B)     | KeyLen (2B)   | ValLen (4B)   | CRC32 (4B)    | Payload (...) |
+---------------+---------------+---------------+---------------+---------------+---------------+---------------+
```
- **Sync Policies:** Configurable sync modes: `EveryWrite` (`fdatasync`), `Periodic` (e.g. 100ms ticker), or `OSBuffer`.
- **Recovery Scanner:** Sequential reader validating CRC32 checksums, replaying committed mutations to restore state after crash.

---

## Week 9: Disk Paging & Slotted-Page Layout

Fixed 4KB disk blocks (`PageSize = 4096 bytes`) to align with hardware sector boundaries.

### Slotted-Page Memory Layout
```
+-------------------------------------------------------------+
| Page Header: PageID (8B), LSN (8B), Lower (2B), Upper (2B)  |
+-------------------------------------------------------------+
| Slot 0 (Offset 2B, Len 2B) | Slot 1 (Offset 2B, Len 2B) ... | (Grows Down)
+-------------------------------------------------------------+
|                     ... Free Space ...                      |
+-------------------------------------------------------------+
| ... Tuple Record 1 ...                                      |
+-------------------------------------------------------------+
| ... Tuple Record 0 ...                                      | (Grows Up)
+-------------------------------------------------------------+
```
- **Slot Directory:** Grows downwards from header; stores pointers to records.
- **Tuples:** Stored at the end of the page, growing upwards.
- **Compaction:** Defragmentation routine that shifts records to reclaim tombstoned/deleted space.

---

## Week 10: Buffer Pool Manager

Caches active disk pages in memory to eliminate repeated disk I/O.
- **Page Table:** Maps disk `PageID` to memory `FrameID`.
- **Pin Counting:** Tracks active readers/writers. Pinned frames (`pin > 0`) cannot be evicted.
- **Clock Eviction Policy:** Circular clock hand scanning unpinned frames; clears reference bit on first pass, evicts on second.
- **Dirty Flusher:** Writes modified pages to disk before recycling the frame.

---

## Week 11: On-Disk B+ Tree (Search & Insert)

B+ Tree nodes serialized directly inside 4KB slotted pages.
- **Internal Nodes:** Contain routing keys and child `PageID` pointers.
- **Leaf Nodes:** Contain indexed keys, record pointers, and `NextPageID`/`PrevPageID` pointers for sequential scans.
- **Node Splitting:** When node fills up (free space < record size), split keys 50/50, allocate a new page, and promote the middle key to parent.

---

## Week 12: On-Disk B+ Tree (Deletes & Range Scans)

- **Underflow & Merging:** When page fill factor falls below threshold (e.g., 50%), borrow keys from siblings or merge adjacent leaf nodes.
- **Cursor Range Scans:** `Cursor.Seek(startKey)` locates leaf node via root traversal, then iterates sequentially across leaf pages via `NextPageID`.
- **End-to-End Pipeline:** Integrate WAL $\to$ Buffer Pool $\to$ Slotted Pages $\to$ B+ Tree into unified transaction coordinator.

---

## Success Verification
- [ ] Process recovery test: Crash during active load; re-open DB and verify 100% data consistency via WAL replay.
- [ ] Sequential scan matches exact key ordering across 100,000 inserted keys.
- [ ] Zero unpinned frame leaks under heavy concurrent read/write transactions.
- [ ] All source files modularized to `< 200 LoC`.
