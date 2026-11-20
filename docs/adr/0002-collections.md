# ADR 0002: Collections as a catalog over prefix partitions

**Status:** accepted, 13 November 2026. Implements the Phase 3 "Collections / namespaces" row of `docs/studio-figma-gap-plan.md`.

## Context

Keys are `uint64`. By long-standing convention the top 8 bits (bits 56-63) are a partition prefix and the engine treats the key as a scalar ([v1 semantics](../v1-semantics.md) section 1). Studio's "Collections" rail already groups records by that prefix, labelled from `partitions.json`. The engine has no notion of a named collection, so Studio cannot create, list or drop one, and a visual query builder or secondary indexes have nothing to attach to.

Constraints:

- Existing clients and data files must keep working unchanged. Prefix `0x00` holds plain small keys (1, 2, 3...), `0x05` holds API tokens, and downstream applications already use other prefixes (`0x01`, `0x03`, `0x06`, `0x51`, ...).
- WAL format, page format and the replication wire format are stable. Backups and replicas must carry collection metadata with no new mechanism.
- AGENTS.md: domain-agnostic engine, WAL before page, no allocations on the data path.

## Options

### A. Catalog of named collections bound to prefix partitions (chosen)

A small catalog maps `name -> prefix (uint8)`. A collection's keys are `prefix<<56 | local`, with `local` a 56-bit integer. A collection scan is the range scan `[prefix<<56, prefix<<56 | 2^56-1]`.

- No key re-encoding, no migration: every existing key already lives in some prefix. Existing data is *adopted* by binding a name to its prefix.
- Reads and scans are unchanged and remain a single B+ tree range; the data path never consults the catalog.
- Limits: at most 256 prefixes total (254 usable; see reservations), 56 bits of key per collection, and keys remain integers. String keys are not solved here.

### B. String keys hashed into 64 bits

Collections and user keys become strings hashed into `uint64`.

- Needs the original string stored somewhere (a second record per key or a changed value format), plus collision handling, since a 64-bit hash collides and the key space is the primary key.
- Destroys range order, so `Scan`, Studio paging and the 256-way prefix grouping stop meaning anything.
- Breaks every existing client that sends integer keys and the on-disk meaning of existing keys.
- Rejected.

### C. Widen the key to a composite `(collection_id, key)`

Change the B+ tree key, WAL key field, proto and replication stream to carry a collection id.

- Cleanest long-term model (no 256 cap), but changes the on-disk and wire formats, requires a file migration and a coordinated leader/follower upgrade, and rewrites the tree comparator on the hottest path. That is a quarter of work and a format-version bump, not a first slice.
- Rejected for now. Option A does not preclude it: a later format version can map each prefix-collection to a wide collection id, because the catalog already owns the name-to-key-range mapping (see Consequences).

## Decision

Option A.

### Key encoding

`key = uint64(prefix)<<56 | local`, `local <= 0x00FFFFFFFFFFFFFF`. Helpers `api.CollectionKey(prefix, local)` and `api.CollectionRange(prefix)` build keys and scan bounds. Plain `uint64` keys keep working exactly as before; a key belongs to whichever collection owns its prefix, if any.

### Catalog storage

The catalog is **one ordinary record** at a reserved key, `0xFF00_0000_0000_0001` (prefix `0xFF`, local `1`), written with the normal `PutIf` path. Consequences:

- It is WAL-logged, checkpointed, replicated to followers, and included in `beastctl backup` and snapshot exports with no new code.
- It is updated with compare-and-swap (`VALUE_EQUALS`, or `ABSENT` for the first write), so two admins creating collections concurrently cannot lose an update.
- No data-file format change, no `MetaVersion` bump. Older binaries ignore the record (they see one more key in prefix `0xFF`).

Value format: magic `BCAT`, version byte `1`, uvarint count, then per entry: prefix byte, uvarint name length, name bytes, int64 created-at unix seconds. Decoding is strict; an unreadable record makes catalog operations fail with an error rather than being overwritten.

### Reserved prefixes and system partitions

| Prefix | Rule |
|---|---|
| `0x00` | Default partition for plain keys. Not reservable by name; it is shown in Studio as the unnamed default. |
| `0x05` | API tokens. Never bindable or droppable: `ErrReservedPrefix`. |
| `0xFF` | Catalog record. Never bindable. |
| `0x01`-`0x04`, `0x06`-`0x0F` | Legacy/system range. Not auto-allocated. May be bound **only by explicit prefix**, which is how existing downstream data (for example users at `0x01`) is adopted. |
| `0x10`-`0xFE` | Auto-allocation range. `CreateCollection` without a prefix picks the lowest prefix in this range that is neither in the catalog nor holding any record (checked by a one-record bounded scan). |

Explicit binding of a non-empty prefix is allowed (that is adoption). Auto-allocation never claims a prefix that holds data.

Tokens stay in `0x05`; the token store is untouched. No existing record moves.

### Names

1-64 bytes, `[a-z0-9][a-z0-9_-]*`, unique. Lowercase only so Studio, CLI and URLs never disagree on case.

### Operations

- **Create**: name, optional prefix. Fails with `AlreadyExists` for a duplicate name or a bound prefix, `InvalidArgument` for bad names or reserved prefixes, `ResourceExhausted` when no prefix is free.
- **List**: returns name, prefix, created-at. Studio adds record counts from the existing `partitionCounts` stats.
- **Drop**: unbinds the name. **In this slice a drop is refused (`FailedPrecondition`) while the prefix holds any record.** Deleting a collection's data atomically across a large range is not safe with one WAL batch (8 MiB cap) and a non-atomic chunked delete could leave a half-dropped collection. Delete-with-data is a follow-up; until then clients delete records first.

Reads (`List`) work on replicas; writes return the existing read-only replica error because they are normal `Put`s.

### gRPC API

Additive RPCs on `BeastDBService`: `CreateCollection`, `ListCollections`, `DropCollection`. Existing messages and RPCs are unchanged, so old clients and generated stubs keep working. Roles: `ListCollections` viewer/writer/admin; `Create`/`Drop` admin only (registered in `methodRoles`, deny by default). `Get`/`Put`/`Delete`/`Scan` do **not** take a collection argument in this slice; clients compose keys with the documented encoding. A collection-aware data plane (name in the request, 56-bit local keys, bounds checking) is the next step.

Writes to the catalog key via `Put`, `Delete` or `BatchWrite` over gRPC and the Studio key API are rejected with `InvalidArgument`/HTTP 400, so a client cannot corrupt the catalog by accident. This is the one behavior change to the plain key API, and it affects a single reserved key.

### Replication and backup

The catalog is a normal WAL record, so followers replay it and see collections. Followers serve `ListCollections` from their own copy. `beastctl backup`/`restore` and WAL-based recovery carry it with no changes. Catalog operations are rare admin actions and never touch the read or write hot path, so this ADR makes no performance claim.

### Studio

The existing Table Editor "Collections" rail now loads `/api/collections` (session-authenticated; admin-only create/drop) and labels each prefix with its catalog name, falling back to `partitions.json` and then the generic prefix label. Named but empty collections appear in the rail with a count of 0. Admins get a "New collection" form and a drop action (disabled with the reason while non-empty). Studio shows no field schema; collections are still key ranges of opaque values.

## Consequences

- Smallest safe slice, no migration, fully backwards compatible, backed up and replicated for free.
- The hard ceiling is 254 usable collections per database and 56-bit local keys. That is a stated limit, not a hidden one.
- String keys are still not supported; that needs its own ADR (secondary-key index mapping strings to integers is the likely route and builds on collections and the future index catalog).
- Future migration to wide collection ids (Option C) stays possible: the catalog is the only place that knows name -> key range, and clients using names rather than raw prefixes would not notice.
- Concurrency: catalog edits are CAS-protected; the first-write `ABSENT` race is handled the same way.

## Not done in this ADR

Collection-scoped Get/Put/Scan, drop-with-data, rename, per-collection schema or indexes, string keys, per-collection quotas or permissions.
