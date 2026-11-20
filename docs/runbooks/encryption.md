# Encryption at rest runbook

Design, threat model and limits: [ADR 0003](../adr/0003-encryption-at-rest.md). Encryption is **off by default**. It protects the data file, checkpoint journal, spill file, WAL and backup bundles from someone who copies the disk or a backup without the key. It does not protect against a compromised running server, and it does not detect an old valid copy of a file being put back (ADR 0003 describes a design for that; it is not built).

## Turn it on for a new database

```bash
beastctl keygen --out /etc/beastdb/master.key        # mode 0600, 64 hex characters
beastdb-server -data-dir /var/lib/beastdb -encryption-key-file /etc/beastdb/master.key
```

(`BEASTDB_ENCRYPTION_KEY_FILE` works too.) On first start the server creates `beast.bin.keyring` next to the data file. Studio, Settings, Storage shows **Encryption at rest: On (aes-256-gcm, key 1)** only when the running engine reports it.

## Where the master key comes from

| Option | Use |
|---|---|
| `-encryption-key-file PATH` | A key file (32 raw bytes or 64 hex). Mode must be 0600. The default choice. |
| `-encryption-key-cmd "CMD ARGS"` | A command that prints the key to stdout: the hook for a KMS (a few-line wrapper around your provider's CLI). Run directly, no shell; use a JSON array (`'["/usr/local/bin/get-key","--id","a b"]'`) when an argument has spaces. 30 s timeout; stderr is discarded. Test it by hand first. |
| `-encryption-key-env NAME` | The environment variable `NAME`. The weakest option: the value stays in the process environment (and `/proc/<pid>/environ` on Linux) for the life of the process. |

Set only one. `beastctl --key-file` and `--new-key-file` accept the same forms as specs: `env:NAME`, `exec:CMD ARGS`, `file:PATH`. The key is never logged and never shown in Studio; Settings shows only the kind of source ("Key file", "Environment variable", "Command (KMS hook)"). The source is read each time the database is opened, so if the KMS is down the server does not start (it never falls back to plaintext).

## Convert an existing plaintext database

Stop the server first. The conversion copies every record into a new database, checks that it opens and holds the same number of records, then swaps the files.

```bash
beastctl encrypt --db /var/lib/beastdb/beast.bin --wal /var/lib/beastdb/beast.wal --key-file /etc/beastdb/master.key
```

- The originals are kept as `beast.bin.preenc.bak`, `beast.wal.preenc.bak` and so on. **They are still plaintext**: move them to safe storage or delete them once you have checked the new database, or pass `--delete-original`. Deleting a file does not guarantee the bytes cannot be recovered from the disk.
- The WAL restarts at LSN 0, so re-seed any replicas from the new database.
- Start the server with `-encryption-key-file`. Without it the server refuses to open the file.

`beastctl decrypt --key-file ...` goes back to plaintext; `beastctl rekey --key-file OLD --new-key-file NEW` writes a new database under a new master key **and a new data key** (key ID goes up by one). Both keep `*.predec.bak` / `*.prekey.bak` unless `--delete-original`. `audit.log` in the same directory is converted together with the database (encrypt seals it, decrypt returns it to plaintext, rekey re-seals it); its original is kept as `audit.log.pre*.bak` and, like the other backups, is readable until you remove it.

## Rotate the master key without rewriting data

```bash
beastctl rekey --rewrap-only --db /var/lib/beastdb/beast.bin --key-file OLD --new-key-file NEW
```

This re-wraps the data key in `beast.bin.keyring` under the new master key and touches nothing else, so it takes a moment however large the database is. It checks the old key against every page first, then swaps the keyring atomically: the old key works until the swap, the new key after it, and a crash leaves one of the two. Then update the server's key source and restart. Run it with the server stopped (a running server keeps its keys in memory and is not affected until it restarts).

Limits: the data key does not change, so someone who kept a copy of the **old keyring** and has the old master key can still read the data, and the nonce budget is not reset (use plain `rekey` for that). Backups made earlier keep the master key they were made under. A leftover `beast.bin.keyring.tmp` after a crash is harmless.

## Audit log

When the database is encrypted the server writes `audit.log` sealed (one encrypted record per line) and Studio Settings shows **Audit log: Encrypted**. The hash chain is unchanged and **Verify** in Studio still works. An existing plaintext audit log is converted when the server first starts with encryption on. An encrypted audit log cannot be opened without the key: the server stops with a message instead of reading it as plaintext. Studio users and API tokens are stored inside the database and are encrypted with it. Removing the newest audit records, or restoring an older copy of the log, is not detected.

## Keys: what to back up

- The master key file and `beast.bin.keyring` are both required to read the data. **Losing either loses the data.** Keep the master key somewhere other than the data volume, and back up the keyring (it holds only a wrapped data key and is useless without the master key).
- Each backup bundle contains a copy of the keyring as `keyring.json`. The master key is never copied into a bundle.
- Limit of random nonces: re-key with a new data key (`beastctl rekey`, not `--rewrap-only`) before about 2^32 page writes under one key (several terabytes of page writes).

## Backups and restore

```bash
beastctl backup  --db ... --wal ... --out ./bundle --key-file /etc/beastdb/master.key
beastctl verify  --backup ./bundle                                   # checksum and size only
beastctl verify  --backup ./bundle --key-file /etc/beastdb/master.key # authenticates every page
beastctl restore --backup ./bundle --dest ./restored --key-file /etc/beastdb/master.key
```

Without the key, `verify` cannot tell tampering from the original when someone also edits `manifest.json`; with it, every page is authenticated.

## Errors you may see

| Message | Meaning |
|---|---|
| `master key does not match this database` | wrong key file for this keyring |
| `database is encrypted; supply -encryption-key-file` | opened without a key |
| `database is not encrypted; run beastctl encrypt first` | key given for a plaintext file (the server will not silently run unencrypted) |
| `page failed authentication` | tampering, truncation or disk damage in the data file; restore from a backup |
| `wal: record failed authentication but later records are valid` | the WAL was edited or damaged mid-log |
| `key file must not be accessible by group or others` | `chmod 600` the key file |
| `key command failed: PROG exited with status N` / `timed out` | the `-encryption-key-cmd` program failed; run it by hand (its output is deliberately not shown here) |
| `key environment variable is not set` | `-encryption-key-env` names an empty or missing variable |
| `audit log is encrypted; start the server with the database's encryption key` | the database was decrypted or replaced without its audit log; run `beastctl decrypt` on the database that matches, or restore the matching `audit.log.pre*.bak` |

## Not covered

Separate files that are **not** encrypted: `session.key` (the Studio cookie signing secret; anyone who copies it can forge session cookies), the runtime settings file, each backup's `manifest.json`, and the key file itself. Also not covered: rollback (an older valid copy of the files put back), online data-key rotation and an external anchor; both are designed in the ADR and not built.
