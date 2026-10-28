# ADR 0001: Values larger than one page

**Status:** accepted, 28 October 2026.

## Context

Every value lived in one slotted-page tuple, so values were capped at 4,000 bytes. Clients
storing documents, reports, serialized aggregates or index lists had to invent their own
chunking, each with its own atomicity and corruption bugs.

## Decision

- Values up to `MaxRecordSize` (4,000 bytes) are stored inline exactly as before.
- Values up to `MaxValueSize` (2 MiB) are stored as a chain of chunk tuples headed by a
  24-byte manifest tuple (magic, version, length, CRC32, chunk count, first chunk RID). Each
  chunk carries a magic byte and its successor's RID; page 0 (the meta page) ends the chain.
- The B+ tree points at the manifest with bit 15 of the RID's slot ID set. A 4 KB page holds
  at most ~1,000 slots, so inline RIDs never use that bit.
- The WAL still logs the whole value in one frame (`OpPut`) or one batch frame, so recovery
  and replication are unchanged: replay re-chains the value. A batch's values are capped at
  `MaxBatchBytes` (8 MiB) and a WAL frame at 32 MiB.
- Reads verify chunk magic, chain length and CRC and return `ErrCorruptValue` on mismatch.
  Overwrite and delete free the chain only after it validates, so a damaged chain is leaked
  rather than allowed to tombstone tuples owned by other keys.

## Consequences

- No migration: existing files contain no flagged RIDs. A file containing large values,
  opened by an older binary, fails reads of those keys with `ErrSlotOutOfBounds` instead of
  returning manifest bytes as data. Downgrade after writing large values is not supported.
- gRPC's default 4 MiB message limit is raised with `api.MessageSizeServerOptions` and
  `api.MessageSizeDialOption`; the server, followers and `beastctl` use them, and clients
  that send large batches must too.
- A large write holds the engine's write lock while it writes ~260 chunk tuples per MiB.
- Writing this exposed two allocator defects, fixed alongside: inserts never reused
  tombstoned slot entries, so the slot directory of a recycled page grew until it could no
  longer host a full tuple; and pages too full for one request were dropped from the reuse
  list for good. 300 overwrites of a 512 KB value now keep the file at two copies' worth of
  pages instead of growing it eightfold.
