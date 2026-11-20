# ADR 0003: Encryption at rest

**Status:** accepted and implemented for the first slice (data file, spill file, checkpoint journal, WAL, backups as a consequence), 13 November 2026. Reconciled with storage format version 2 (page checksums, page reuse, saved maps) on 20 November 2026; see "Interaction with format version 2". Items under "Not in this slice" are planned, not built.

## Context

The data file (4 KB pages), the checkpoint journal, the spill file, the WAL (and its `.bak` generation) and backup bundles are plaintext. Anyone who copies the disk, a snapshot of the volume or a backup directory reads every value.

## Threat model

In scope:

- **Stolen or discarded disk, volume snapshot, or backup bundle** (offline attacker who never sees the running process or the key file).
- **Offline tampering** of those files before they are returned or restored: bit flips, page edits, swapping one page for another, editing a WAL record, truncating a page out of the middle of the file.

Out of scope (stated so nobody assumes otherwise):

- A compromised running process or host (the key and plaintext are in memory).
- Someone who holds both the data directory and the master key file.
- **Rollback**: replacing a page, or the whole file, with an older, valid, authentic copy is not detected. Detecting it needs a Merkle tree or a trusted counter. Truncating the WAL tail is likewise not detected.
- Metadata that stays visible: file sizes, page count, WAL record count, record lengths and LSNs (see WAL below), and access patterns.
- Network traffic (TLS already covers gRPC) and the Studio session cookie.
- Secure erase of the old plaintext files after migration. Filesystems and SSDs cannot promise it.

## Decision

### Cipher and mode

AES-256-GCM (Go standard library, hardware accelerated on amd64 and arm64) with a fresh random 96-bit nonce for every encryption. Random nonces are used instead of a counter because there is no durable place to keep a counter that survives a crash between "page written" and "counter persisted". The NIST limit for random-nonce GCM is 2^32 encryptions per key, about 16 TB of 4 KB page writes. Rotation (below) resets it; a counter of page encryptions is a planned metric so operators can see how far they are.

Candidate rejected: XChaCha20-Poly1305 (24-byte nonce removes the limit) needs `golang.org/x/crypto`, currently only an indirect dependency, and is slower than AES-GCM on CPUs with AES instructions. It stays the fallback if the 2^32 limit becomes a practical constraint. AES-XTS was rejected because it has no authentication.

### Page layout: the tail of every page is reserved

A page is still exactly 4096 bytes, aligned, and encrypted in place, so there is no stride change and no tag side file. Encrypted databases reserve the **last 32 bytes** of every page. The slotted page's usable end is `EncDataEnd = PageSize - 32 = 4064` (a plaintext version 2 file ends at `PageDataEnd = 4092`, before its 4-byte checksum trailer). `DiskManager.PageEnd()` returns whichever applies, and the buffer pool, the free list and page chains (`WriteChain`/`ReadChain`) use it instead of a constant. The largest inline tuple (`MaxRecordSize`, 4,000 bytes) plus header and slot still fits.

```
0                                          4064              4096
+------------------------------------------+------------------------------+
| header(24) | slots ... free ... tuples   | tag(16) nonce(12) key id(2) magic(2) |
+------------------------------------------+------------------------------+
  AES-256-GCM ciphertext of bytes 0..4063     clear text (32 bytes)
```

The ciphertext covers bytes 0 to 4063 (including the page header, so the page's own PageID and LSN are authenticated). AAD is `page ID (8 bytes) | key ID (2 bytes)`. Binding the page ID stops a page being moved to another position. The LSN lives inside the encrypted header, so it is authenticated as content but cannot also be in the AAD; that is acceptable because LSN rollback is the out-of-scope replay case anyway. `magic` (0xBE01) marks "this page is encrypted" in clear so the open path can tell an encrypted file from a plaintext one before it has a key.

Plaintext databases are unchanged: reserve 0, `Upper` 4096.

A page inside the file whose tail is all zeros is rejected as corrupt (it would otherwise let an attacker erase a page); pages past the end of the file still read as zeros, as before.

### Where encryption happens

At the `DiskManager` boundary only. The buffer pool, B+ tree and slotted pages work on plaintext in memory exactly as today, so `WAL.EnsureDurable` and the WAL-before-page barrier are untouched. `ReadPage` decrypts, `SpillPage` and the checkpoint journal writer encrypt. The journal and data file therefore hold ciphertext; installing the journal into the data file copies bytes. The spill file holds ciphertext too (same key, fresh nonce per spill), so evicted dirty pages are not left in clear next to an encrypted data file.

### Key hierarchy and key source

```
master key (32 bytes, operator-supplied)
   └─ HKDF-SHA256("beastdb kek v1") ─► KEK ── wraps ──► DEK (32 random bytes, in <db>.keyring)
                                                         ├─ HKDF("beastdb page v1") ─► page key
                                                         └─ HKDF("beastdb wal v1")  ─► WAL key
```

- The master key comes from a **key file** (`-encryption-key-file`): 32 raw bytes or 64 hex characters. On Unix the file must not be readable by group or others, or the server refuses to start. Environment variable and KMS sources (envelope encryption with the KEK held by a KMS) are planned and fit behind the same `crypt.KeySource` boundary; they are not built.
- `<db>.keyring` is a small JSON file holding the wrapped DEK, key ID and format version. It contains no usable secret without the master key. It is created once, before the first encrypted page, by write-to-temp, fsync, rename. **Losing the keyring or the master key loses the data**; the runbook says to back both up separately and to put the key file somewhere other than the data volume.
- A wrong master key fails the unwrap and the engine reports `ErrWrongKey` at open, before any page is read. A damaged keyring or page reports `ErrTampered`/`ErrAuth`.
- A separate DEK means master-key rotation can be a cheap re-wrap of 32 bytes (planned), while a DEK change is a full re-encryption.

### Key rotation

In this slice rotation is offline and complete: `beastctl rekey` writes a fresh database with a new random DEK under the new master key, using the same logical copy as the migration, then swaps files. It resets the 2^32 nonce budget. Online rotation and re-wrap-only rotation are planned. Key ID is stored per page (2 bytes) and in the keyring so a future online rotation can mix generations while it runs.

### WAL

Each record is one AEAD frame:

```
recLen(4) | LSN(8) | nonce(12) | ciphertext( type | keyLen | valLen | key | value ) | tag(16)
```

`recLen` and the LSN stay clear (replay and `findHighestLSN` need them to walk the file, and replication ships records by LSN). AAD is `recLen | LSN`, so a record cannot be moved to another LSN or resized. The WAL key is a different HKDF subkey from the page key. The `.bak` generation uses the same format. Keys, values and operation types are not visible; frame sizes and LSNs are.

Torn-tail handling is kept: a frame that is short, or whose tag fails and which nothing authentic follows, is treated as a torn tail and replay stops, as the plaintext CRC path does. A frame whose tag fails **while a later frame still authenticates** cannot be a torn write, so replay returns `ErrWALTampered` instead of silently dropping the rest of the log.

Mixed modes are refused: an encrypted engine does not read a plaintext WAL, and a plaintext engine does not read an encrypted one (the frame fails to parse), so a half-migrated directory fails loudly.

Replication: the leader's catch-up reads the WAL with the same cipher (`LeaderServer.SetWALCipher`) and sends plaintext records over gRPC. Replica-to-leader traffic needs TLS, as before; followers keep their own local encryption setting. Replicas do not share keys.

### Backups, `verify` and `restore`

Backups are physical copies of the data file, so an encrypted database's backup is ciphertext automatically; nothing extra is written in clear. The keyring is copied into the bundle as `keyring.json` (wrapped DEK only) and the manifest gets `encrypted: true` and `key_id`.

- `beastctl verify` always checks the SHA-256 and size. For an encrypted bundle page 0 is ciphertext, so without a key it cannot validate metadata and says so ("checksum only"). With `--key-file` it authenticates every page of the file (tamper detection) and checks the root page.
- `beastctl restore` of an encrypted bundle requires `--key-file`, restores `data.bin` plus the keyring, and opens the result with the key as its final check. A wrong key fails before any files are replaced.
- Restoring an encrypted bundle on a server that has the key but different `-encryption-key-file` fails with `ErrWrongKey`.

### On-disk format versioning and migration

- `MetaData` gains a 32-bit `Flags` word (bytes 64..68 of page 0, after the version 2 fields); bit 0 = encrypted. Plaintext databases have zero there and nothing existing is rewritten. The flag is inside the authenticated page, so it cannot be cleared by an attacker; the clear `magic` in the page tail is what routes the open path before decryption.
- Opening an encrypted database without a key fails with `ErrEncryptedNoKey`. Opening a plaintext database with a key configured fails with `ErrNotEncrypted` (it does **not** silently run plaintext while the operator believes it is encrypted).
- Migration is explicit and offline: `beastctl encrypt`, `decrypt` and `rekey` (all one routine) open the source, replay its WAL, copy every record to a new database in the target mode, close it, and only then swap the files, keeping the originals as `*.preenc.bak` (or `*.prekey.bak`). The plaintext backup it leaves defeats the purpose until it is removed, so the command prints that and offers `--delete-original`, which removes the files but cannot guarantee the bytes are unrecoverable. Page-level in-place conversion was rejected: plaintext pages may use up to 4092 bytes (4096 in a version 1 file) and cannot take a 32-byte tail.
- The logical copy re-packs pages, so the migrated file is also compact. Large values (ADR 0001) are copied through the engine and re-chained.

### Interaction with format version 2

Decisions made when this ADR met page checksums, page reuse and saved maps:

- **No separate checksum in an encrypted file.** The AES-GCM tag authenticates the page ID and every byte of the page (a stronger check than CRC-32C), so the version 2 checksum trailer is not written. A CRC over the ciphertext would add nothing, and one over the plaintext would publish a hash of the plaintext beside the ciphertext. `verifyPage`'s rule that an all-zero page is acceptable does not apply either: inside the file an all-zero page is `ErrPageAuth`; only a page past the end of the file reads as zeros.
- **Encrypted means version 2.** An encrypted file has a version 2 meta page, page reuse, and the saved free-space map and search index (stored in chain pages, which are sealed like every other page). `DiskManager.Checksums()` reports true for it ("version 2 layout, pages integrity-protected") so the engine never tries to migrate it as a version 1 file; automatic v0/v1 migration is for plaintext files only.
- **Pre-release encrypted layout.** The first encrypted build put the flag word in meta bytes 32..36 on a version 1 meta page; version 2 uses those bytes for the free list head. No release contains that layout. A database written by that build fails to open with `ErrInvalidMetaVersion` and a message naming `beastctl decrypt` of the build that wrote it; it is never migrated automatically.
- **Plaintext v1 plus a key.** Opening a version 1 (or v0) file with a key is refused with `ErrNotEncrypted` and leaves the file unchanged. `beastctl encrypt` opens the source as plaintext, which migrates v0/v1 to version 2 and replays its WAL tail, then copies the records into a new encrypted database. Opening the file without a key still performs the usual automatic migration.
- **Saved maps stay valid.** A restart loads the free list, the space map and the index snapshot through the same chain code with `PageEnd()`. A failed read of the space map or snapshot is a hint failure (rebuild), but authentication failure of an ordinary page is an error, as before.
- **WAL.** The encrypted log keeps the plaintext log's recovery rules: a torn tail is cut at open through a non-append handle (Windows-safe, as for plaintext) so new records are not appended behind it; a frame that fails authentication with an authentic, higher-LSN frame after it is damage in the middle and fails the open with `ErrCorruptLog` (`ErrWALTampered` is the same error); `-wal-truncate-corrupt` cuts at the damage and saves the cut bytes to `<wal>.discarded`. Resynchronisation after damage uses the tag, not a CRC, as proof that a later frame is intact.
- **Migration swap.** `encrypt`, `decrypt` and `rekey` preserve each original as a hard link or copy before replacing it by atomic rename, so no path is ever missing; the keyring and WAL are installed before the data file, and a crash between steps fails loudly (key or WAL format mismatch) with the preserved originals available for a manual restore.

### Studio

The status API reports `encryption.at_rest` from the open engine (`disk.Encrypted()`), the cipher name and key ID, never from configuration. Settings shows "On" only when pages are in fact being encrypted by the running engine; otherwise "Off", plus a pointer to `beastctl encrypt`. It never shows key material.

### Performance

Overhead is measured, not assumed, with `cmd/beastbench --self-contained [--encrypt]` and the engine benchmarks `BenchmarkEncryption*` (`internal/api`), on the same machine, with and without encryption. The cost sits in buffer pool misses (one AES-GCM open of 4 KB, done outside the disk manager lock so concurrent misses do not serialize on it), checkpoint and spill writes (one seal per page), and WAL appends (one seal per record); cache hits do no cryptography. Measured results and their noise are in [benchmarks](../benchmarks.md#encryption-at-rest-overhead); no comparison with any other database is made.

## Not in this slice (planned)

- KMS and environment-variable key sources; re-wrap-only master-key rotation; online DEK rotation.
- Encrypting the Studio user store and the audit log (separate files, currently plaintext). Check them if they matter for your threat model.
- Rollback and WAL-truncation detection.
- Metrics for the nonce budget.
- Encrypting `backup` bundles at the byte level beyond what physical page encryption already gives (manifest stays clear).
- `encrypt`, `decrypt` and `rekey` copy key/value records (the data model that exists today). Any new persistent structure (for example collections, index catalogs) must be added to that copy, or it will be dropped by a migration; a test should enumerate structures when they land.

## Consequences

- Usable bytes per page in an encrypted database are 4064, so an encrypted file is a little larger for the same data than a plaintext one.
- A plaintext-opening older binary meeting an encrypted file sees bad magic and refuses to open it; there is no downgrade path other than `beastctl decrypt`.
- Every dirty page written adds one AES-GCM pass; see the measured numbers in the benchmark section of the final report and `docs/benchmarks.md`.
