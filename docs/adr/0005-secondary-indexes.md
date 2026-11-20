# ADR 0005: Single-field secondary indexes over JSON records

**Status:** accepted, 20 November 2026. Builds on [ADR 0002](0002-collections.md) (collections) and follows [ADR 0003](0003-encryption-at-rest.md) (encryption) and [ADR 0004](0004-replication-membership-and-manual-promotion.md) (replication).

## Context

`internal/index` has the primary B+ tree (`uint64` key to record id) and one fixed in-memory inverted text index (`SecondaryIndex`, rebuilt at open or restored from a clean-shutdown snapshot). There is no way to find records by the value of a field without scanning a whole key range, so Studio cannot offer a real filter, and Scan cannot push a predicate down.

Constraints:

- Storage format v2, the WAL record format, the replication wire format and the backup format must not change. No migration.
- WAL before page: a data page never reaches the data file before the WAL records describing it are durable.
- Domain-agnostic: an index is declared by a collection, a JSON field path and a type. No built-in field names.
- Records are opaque bytes today. This ADR only gives meaning to values that happen to be JSON objects.

## Options

### A. Persistent index as a second on-disk B+ tree

Index entries `(value, primary key)` in pages, WAL-logged, checkpointed, replicated physically.

- Survives restart without a rebuild, scales past RAM.
- Needs a new page type, a second tree root in the meta page (format bump), index-page redo records in the WAL, free-space handling, encryption of the new pages, and a backup/restore story for them. That is a format version, not a first slice.
- Deferred. Option B does not preclude it: the catalog (below) and the query API stay the same; only where the entries live changes.

### B. Catalog on disk, entries in memory, rebuilt from the data (chosen)

The *definition* of an index is durable (one ordinary record). The *entries* are derived data held in memory and rebuilt from the records. They are never written to a data page.

- No format change; backups, replicas and crash recovery carry the definitions for free because they are one WAL-logged record.
- WAL before page is satisfied trivially: no index state is ever written to the data file, so there is no index page that could get ahead of the WAL. The records that the index is derived from follow the existing WAL-before-page path unchanged.
- Cost, stated plainly: after a restart an index is unavailable until it has been rebuilt by a scan of its collection, and memory use grows with the number of indexed records (see Limits).

## Decision

Option B.

### Index definition and catalog

An index is `(name, collection prefix, field path, type)`:

- **name**: same rules as collection names (`[a-z0-9][a-z0-9_-]*`, 1-64 bytes), unique across the database.
- **collection**: an existing named collection (ADR 0002). The index covers keys in `[prefix<<56, prefix<<56|2^56-1]`. Only collections can be indexed; prefix `0x00` (default partition), `0x05` and `0xFF` cannot.
- **field**: dotted path of object members, 1-8 segments of `[A-Za-z0-9_]+`, at most 128 bytes (for example `address.city`). No array indexing and no wildcards in this slice.
- **type**: `int` (a JSON number written as an integer that fits `int64`) or `string` (a JSON string, at most 256 bytes after unescaping).

The catalog is **one ordinary record** at reserved key `0xFF00_0000_0000_0002`, written with the normal `PutIf` compare-and-swap path exactly like the collection catalog. Value: magic `BIDX`, version byte `1`, uvarint count, then per entry: name, prefix byte, type byte (1 int, 2 string), path, int64 created-at unix seconds. Decoding is strict; an unreadable record fails index operations instead of being overwritten. At most **32 indexes** per database. Writes to this key through `Put`, `Delete` and `BatchWrite` over gRPC and the Studio key API are rejected like the collection catalog key.

A collection with indexes cannot be dropped (`FailedPrecondition`); drop its indexes first.

### Which records are indexed

A record is indexed only when its value is a JSON object, the path resolves, and the value has the declared type. Anything else (not JSON, path missing, `null`, wrong type, out-of-range integer, over-long string) is **not indexed** and is counted as `skipped` in the index status, so a caller can see that coverage is partial. A query through an index returns only indexed records; it never guesses at skipped ones.

### Memory structure and lookup

Entries are `(typed value, primary key)` in a sorted, chunked list (binary search over chunks), plus a `primary key -> value` map so an update or delete can remove the old entry without reading the old record. Equality and inclusive range lookups return primary keys ordered by `(value, key)`. Lookup is O(log n + matches).

### Maintenance and WAL interaction

Index maintenance runs in the same places the text index is maintained today (`putLocked`, `deleteLocked`, batch apply, WAL replay, replicated apply), under the engine write lock, after the record has been applied in memory and logged. Every path that changes a record also changes the index, so a record and its index entry move together under one lock. A write to a prefix that has no index does one array lookup and nothing else; indexed-field extraction is a byte scanner over the value with no allocation.

The same hook sees writes to the catalog key itself. When that record changes (a local create or drop, a WAL replay at open, or a record replicated from a leader) the engine reconciles its in-memory indexes with the record. There is one source of truth and one code path for leader, follower and recovery.

### Background build

A new index starts in state `building` and is not queryable (`FailedPrecondition`, never a partial answer). A goroutine scans the collection range in chunks of 1024 keys. For each chunk it takes the engine read lock, reads the records and inserts their entries before releasing the lock, so a chunk is atomic with respect to writers. Writers that run between chunks update the index live (upsert and delete are idempotent per primary key). The two together make the finished index equal to a full scan taken at one instant, without holding the lock for the whole scan, so reads and writes proceed while it builds. When the scan reaches the end of the range the state becomes `ready`. Dropping an index during the build cancels it.

### Recovery after a crash

Nothing about an index is in the data file, so there is nothing to repair. At open the engine reads the catalog record (as of the last checkpoint), registers each index as `building`, replays the WAL (replay maintains the indexes live, and a catalog change in the WAL reconciles the set), and after recovery completes starts the builders. A crash during a build, during create, or during drop therefore ends in one of two states: the definition is in the WAL and the index rebuilds, or it is not and the index never existed. There is no half-created index. Time to `ready` after a restart is proportional to the size of the indexed collection; a restart makes indexes unavailable for that period. That is a stated limitation of Option B, not a hidden one.

### Encryption and backup

Encryption at rest (ADR 0003) covers the definition record like any record. The entries exist only in process memory, as the decrypted records already do in the buffer pool, so there is no unencrypted index file. Backups and snapshots contain the definition record and the data; a restored database rebuilds the indexes on first open. Nothing needs to be excluded or re-keyed.

### Filter push-down in Scan

`ScanRequest` gains an optional `filter` (`index` name plus `eq`, or `gte` and/or `lte`, each a typed value). With a filter, Scan resolves matching primary keys through the index, restricts them to `[start_key, end_key]`, and returns the records **in ascending key order**, so the existing `limit` and "resume at last key + 1" contract is unchanged. The index lookup and the record reads happen under one read lock, so each returned record matches the filter at that instant. Cost: the matching key set is materialised and sorted on every call (O(m log m) for m matches; keys only, 8 bytes each). Bounds are inclusive; there are no exclusive bounds, `in`, `or` or multi-field predicates yet. A filter against an index that is `building` fails with `FailedPrecondition`; it never silently falls back to a full scan.

No performance comparison with other databases is claimed. The only claims are the complexity statements above; a benchmark against an equivalent scan workload is needed before any speed-up number is quoted (`docs/benchmarking.md`).

### API surface

gRPC (additive; old clients unaffected): `CreateIndex`, `ListIndexes`, `DropIndex`, and the optional `filter` on `ScanRequest`. `ListIndexes` is viewer/writer/admin; create and drop are admin only. HTTP: `/api/indexes` (GET any signed-in user; POST and DELETE admin only, audited), and `/api/records` accepts the same filter fields. `ListIndexes` reports name, collection, field, type, state (`building`/`ready`), indexed entry count, skipped count and build progress; Studio shows only those fields.

## Limits (this slice)

- Single field, `int` and `string` only; no floats, bools, arrays, composite or unique indexes.
- Entries live in RAM (roughly 40-60 bytes per indexed record plus the string length); 32 indexes per database.
- Index unavailable after restart until rebuilt; a leader promoted by ADR 0004 rebuilds from its own data.
- Filtered Scan is key-ordered and materialises the match set per call.
- No query statistics, so no slow-query index suggestions.

## Consequences

- Smallest safe slice: no format change, no migration, replication and backup work unchanged.
- Option A (persistent index) remains open as a later format version behind the same catalog and API.
- The catalog hook makes index definitions replicate like any record, so followers build their own indexes.
