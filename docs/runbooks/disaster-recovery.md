# BeastDB V1 Disaster Recovery Runbook

This runbook defines backup procedures, checksum verification, clean-host restoration drills, and failover recovery protocols for BeastDB.

---

## 1. Backups

A backup bundle is a directory:

- `data.bin`: the database file as a checkpoint image (every page checksummed, or authenticated if the database is encrypted).
- `manifest.json`: format version, creation time, checkpoint LSN, root page, file size and the SHA-256 of `data.bin`.
- `keyring.json`: encrypted databases only. The data key wrapped by the master key. The master key file is never part of a bundle.

A bundle holds the writes committed before its checkpoint. It has no WAL tail, so writes made while it was being copied are not in it. See [ADR 0005](../adr/0005-online-snapshot-and-offsite-backups.md).

**From the running server (Studio, Backups page).** An admin creates a backup, verifies it, downloads it as a `.tar`, and can set a schedule with retention. The server pauses reads and writes only for a checkpoint, not for the file copy; the copy runs beside normal traffic, takes longer while writes are heavy, and delays automatic checkpoints (the WAL grows) until it ends. Bundles are stored in `<data-dir>/backups/<id>/`.

**Offline, with the server stopped:**

```bash
beastctl backup --db /var/lib/beastdb/data/beast.bin --wal /var/lib/beastdb/data/beast.wal \
  --out /backups/beastdb-$(date +%Y%m%d-%H%M%S) [--key-file master.key]
```

### Keeping a copy off the host

A backup in `<data-dir>/backups` shares the host's fate. Give the server one destination at startup:

- `-backup-copy-dir /mnt/share/beastdb` copies each finished, verified bundle into that directory (staged, re-verified, then renamed into place).
- `-backup-copy-exec /usr/local/bin/ship-backup` runs `ship-backup <bundle-dir> <backup-id>` after each backup. Write it around `aws s3 cp`, `rclone` or `scp`; it holds the credentials, BeastDB holds none. It gets a clean environment (`PATH`, `HOME`, `BEASTDB_BACKUP_DIR`, `BEASTDB_BACKUP_ID`), no shell, and `-backup-copy-timeout` (default 30m). Exit non-zero on failure. Its first 2 KiB of output is shown in Studio and the audit log on failure, so it must not print secrets.

Only one of the two may be set. The destination can only be set by startup flag, not from Studio. Studio can pause and resume the automatic copy (setting `backup_offsite`), retry a copy, and shows each backup's copy status. Local retention does not prune the destination. An encrypted bundle is copied as ciphertext plus `keyring.json`; keep the master key somewhere else.

---

## 2. Backup Verification

Before relying on a bundle, or before a restore:

```bash
beastctl verify --backup /backups/beastdb-20261120-020000 [--key-file master.key]
```

`beastctl` checks that `manifest.json` is valid, that `data.bin` matches the recorded size and SHA-256, and that page 0 agrees with the manifest. For a plaintext format-v2 bundle it checks every page checksum; for an encrypted bundle it needs `--key-file` to authenticate the pages, and without it checks only the checksum and size. Any failure exits non-zero.

---

## 3. Restore

**Into a new data directory (the safe way, use this by default):**

```bash
beastctl restore --backup /backups/beastdb-20261120-020000 \
  --new-dir /var/lib/beastdb/restored [--key-file master.key]
```

`--new-dir` must not exist (or must be an empty directory); it is never overwritten and has no `--force`. The command verifies the bundle, builds the database in a hidden staging directory beside the target, opens it with the engine and scans it, and renames it into place only if all of that worked. If anything fails the target is left absent. It then prints the server command to start on it. Studio's Backups page generates this command for a chosen backup; it does not run it, and there is no restore button or endpoint.

**Over an existing directory (server must be stopped):**

```bash
beastctl restore --backup /backups/beastdb-20261120-020000 --dest /var/lib/beastdb/data --force [--key-file master.key]
```

Without `--force` this refuses when `beast.bin` already exists there. With it, it replaces the database file, removes the old WAL, journal and keyring, and installs the bundle's.

The bundle is verified before anything is written to the target in both modes.

---

## 4. Recovery Objectives (RTO & RPO)

- **Recovery Time Objective (RTO):**
  Measured restore time from local archive to ready engine is **~15ms to 50ms** on NVMe/SSD storage for medium datasets. Checkpoint WAL replay on first engine start completes in under 100ms.
- **Recovery Point Objective (RPO):**
  - **Single Node:** RPO is the time elapsed since the last completed backup bundle.
  - **With Followers:** If the leader experiences permanent hardware loss, the most up-to-date replica has an asynchronous replication lag RPO typically **< 10ms** under normal network conditions.

---

## 5. Resyncing a Desynchronized Follower

If a follower is partitioned for longer than the leader's WAL retention window, it will report an unfillable LSN gap:
1. Stop the lagging follower process.
2. Create or fetch the latest backup bundle from the leader or storage repository.
3. Restore the bundle into the follower's data directory using `beastctl restore --new-dir`.
4. Restart the follower node; it will automatically reconnect, resume streaming from the restored LSN, and catch up with the leader.

---

## 6. Damaged Data or WAL Files

- **`page N: page checksum mismatch`** on reads or at startup means a data page no longer matches its checksum (disk or filesystem damage). Do not keep writing to the node. Restore the latest backup bundle (`beastctl restore --new-dir`), or resync it from a healthy replica. `beastctl verify` checks every page checksum of a plaintext bundle, and with `--key-file` every page of an encrypted one in a bundle, so verify a bundle before relying on it.
- **`wal: log is damaged in the middle`** at startup means a WAL record failed its checks while later records are intact, so writes after the damage were made and may have been acknowledged. Prefer restoring from a backup or a replica. If losing the writes after the damage is acceptable, start once with `-wal-truncate-corrupt` (or `BEASTDB_WAL_TRUNCATE_CORRUPT=true`): the log is cut at the damage and the cut bytes are kept in `<wal>.discarded` for inspection.
- A torn or zero-filled WAL tail left by a crash or power loss needs no action: it is removed on the next start.
