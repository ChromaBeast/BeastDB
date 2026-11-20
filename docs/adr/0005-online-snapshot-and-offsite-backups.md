# ADR 0005: Online snapshot, off-host copies and restore into a new directory

**Status:** accepted and built, 20 November 2026. Number taken from the next free slot on
`main` (0004 was the last); renumber if another branch claimed 0005 first.

## Context

Server-managed backups (`internal/backup`) had three known limits:

1. `Engine.CreateSnapshot` held the engine lock while it copied the data file, so every read and
   write waited for the whole copy. The pause grew with the file size.
2. A backup lived only under `<data-dir>/backups`, on the host it protects.
3. Restore was `beastctl restore --dest`, which with `--force` overwrites a directory.

## Decision

### 1. Snapshot without stalling writes

The data file is not a live file. Since format v2 (`internal/storage/disk_manager.go`) it only
ever holds a complete **checkpoint image**:

- Pages evicted between checkpoints go to the spill file, never to the data file.
- `WriteCheckpoint` is the only thing that writes the data file after open: journal first, then
  install, then the journal is emptied. `Close` ends in a checkpoint too.

So between two checkpoints the data file does not change. The snapshot uses that:

1. Take `imageMu`, a new mutex that every writer of the data file takes (`Checkpoint`, `Close`,
   `CreateSnapshot`). Lock order is `imageMu` then `e.mu`.
2. Under `e.mu` (the only pause): sync the WAL, run the normal checkpoint (`persistCheckpointLocked`),
   read the data file's path. Release `e.mu`.
3. Copy the data file with `e.mu` released. Reads and writes proceed: writes go to the WAL and
   buffer pool, evictions go to the spill file. A checkpoint (automatic or from Studio) and
   `Close` wait on `imageMu` until the copy finishes.

WAL-before-page is unchanged: the checkpoint is the existing one and the WAL is synced first. No
page reaches the data file except through it.

**What the snapshot contains:** exactly the writes committed before its checkpoint, as one
consistent image. Writes made during the copy are not in it. It stays a single file, so the bundle
format (`data.bin`, `manifest.json`, `keyring.json`) and `beastctl verify` and `restore` are
unchanged. I did **not** add a WAL tail to the bundle: with it a bundle would hold a later state,
but it would need a new bundle member, a restore step that replays it, and a way to authenticate
it for encrypted databases. The benefit (a few seconds of extra recency) did not justify a format
change.

**Costs that remain, stated plainly:**

- The pause is the checkpoint: flush of dirty pages, write of the free-space map and meta page,
  two fsyncs. It scales with the data written since the last checkpoint, not with the file size.
- While a snapshot copies, automatic checkpoints are delayed, so the WAL grows for the length of
  the copy. `Close` (shutdown) also waits for a running copy.
- The copy competes with writes for disk bandwidth, so the backup itself takes longer than it did
  under the lock (measured below).

### 2. Off-host copies: a destination interface, two implementations

`backup.Destination` has `Put(ctx, id, bundleDir)`. Implementations:

- `DirDestination`: copies into a local or mounted directory (NFS, a second disk). It stages as
  `<id>.partial`, re-verifies the copy against its manifest and renames into place.
- `ExecDestination`: runs an operator-supplied program as `program <bundle-dir> <backup-id>`,
  directly (no shell), with a cleaned environment (`PATH`, `HOME`, `BEASTDB_BACKUP_DIR`,
  `BEASTDB_BACKUP_ID`) and a time limit. A non-zero exit is a failed copy.

I chose the hook over a built-in S3 client. Signed S3 requests need credentials in the server's
hands, region and endpoint quirks, multipart uploads for files beyond 5 GiB, and a lot of surface to
test without a real service. A hook wrapping `aws s3 cp`, `rclone` or `scp` gets all of that from
tools that are maintained for it, and **BeastDB never holds a cloud credential**, so there is none
to log, return from an API or leak.

Rules:

- The destination is configured only by startup flag (`-backup-copy-dir`, `-backup-copy-exec`,
  `-backup-copy-timeout`, or the matching environment variables). It is deliberately **not**
  settable from the web API: letting an admin session choose a program to run would turn any admin
  login into command execution on the host.
- One runtime setting, `backup_offsite` (1 or 0), pauses or resumes the automatic copy. It
  changes through the existing settings API, so the audit log records it (`settings.update`).
- Each copy, automatic or requested, is recorded in the audit log (`backup.copy`, with
  `FAILED: ...` on error). The listing API shows the destination's kind and path only, never
  arguments or environment. The hook's output (first 2 KiB) appears in the failure text, so a hook
  must not print secrets.
- Only a backup that passed its own verification is copied. A failed copy does not fail or delete
  the local backup; `POST /api/backups/copy?id=` retries it.
- Retention (`backup_keep`) applies to the local directory only. Pruning the destination is the
  operator's job.
- An encrypted bundle goes off-host as ciphertext plus `keyring.json` (the data key wrapped by the
  master key). The master key file is never copied. The off-host copy is useless without it.

### 3. Restore into a new data directory

`beastctl restore --backup <bundle> --new-dir <dir> [--key-file <key>]`:

- Refuses any target that exists and is not an empty directory, and any target inside the bundle.
  It has no `--force`; combining `--new-dir` with `--dest` or `--force` is an error.
- Verifies the bundle first (checksum, size, and every page for an encrypted bundle with the key).
- Builds the database in a hidden staging directory next to the target, opens it with the engine
  and scans it, and only then renames it into place. Any failure removes the staging directory and
  leaves the target absent.
- Prints the command that starts a server on it.

Studio shows this as a **generated command** plus the bundle's readiness (verified, encrypted so
a key file is needed, size). There is no live in-place restore button and no restore endpoint:
restoring over a running server's directory stays impossible from the UI. `--dest ... --force`
remains for an operator who has stopped the server.

## Measured

`TestSnapshotPauseReport` (`BEAST_PAUSE_REPORT=1 go test ./internal/api -run SnapshotPauseReport -v`)
writes 1 KiB records in a loop and records the worst `Put` latency during a snapshot of a 404 MiB
file. "Legacy" is the previous design (checkpoint and copy under `e.mu`), run in the same test.
Apple Silicon laptop, local SSD, warm page cache, so absolute numbers are indicative only:

| Design | Snapshot duration | Worst write latency during it |
|---|---|---|
| legacy, run 1 | 213 ms | 209 ms |
| online, run 1 | 1.44 s | 19 ms |
| legacy, run 2 | 262 ms | 262 ms |
| online, run 2 | 1.59 s | 30 ms |

The stall drops from the whole copy to roughly a checkpoint, and the advantage grows with file
size (the legacy stall is the copy time; the online one is not). The backup itself takes about
six times longer here because the writer keeps the disk busy. The 19 to 30 ms includes ordinary
group-commit fsync latency that a write without a snapshot also sees; the test does not measure
an idle baseline.

## Consequences

- `Snapshotter.CreateSnapshot` keeps its signature; its contract changes as above.
- `Close` can wait for a snapshot copy of a very large file before it returns.
- Bundle format and `beastctl verify`/`restore --dest` are unchanged.

## Not decided here

- A built-in S3-compatible destination. It can be added behind `Destination` without touching the
  rest if the owner wants it; the exec hook is the supported route until then.
- Pruning remote copies.
- A WAL tail in the bundle for a more recent restore point.
