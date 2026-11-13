# Encryption at rest runbook

Design, threat model and limits: [ADR 0002](../adr/0002-encryption-at-rest.md). Encryption is **off by default**. It protects the data file, checkpoint journal, spill file, WAL and backup bundles from someone who copies the disk or a backup without the key. It does not protect against a compromised running server, and it does not detect an old valid copy of a file being put back.

## Turn it on for a new database

```bash
beastctl keygen --out /etc/beastdb/master.key        # mode 0600, 64 hex characters
beastdb-server -data-dir /var/lib/beastdb -encryption-key-file /etc/beastdb/master.key
```

(`BEASTDB_ENCRYPTION_KEY_FILE` works too.) On first start the server creates `beast.bin.keyring` next to the data file. Studio, Settings, Storage shows **Encryption at rest: On (aes-256-gcm, key 1)** only when the running engine reports it.

## Convert an existing plaintext database

Stop the server first. The conversion copies every record into a new database, checks that it opens and holds the same number of records, then swaps the files.

```bash
beastctl encrypt --db /var/lib/beastdb/beast.bin --wal /var/lib/beastdb/beast.wal --key-file /etc/beastdb/master.key
```

- The originals are kept as `beast.bin.preenc.bak`, `beast.wal.preenc.bak` and so on. **They are still plaintext**: move them to safe storage or delete them once you have checked the new database, or pass `--delete-original`. Deleting a file does not guarantee the bytes cannot be recovered from the disk.
- The WAL restarts at LSN 0, so re-seed any replicas from the new database.
- Start the server with `-encryption-key-file`. Without it the server refuses to open the file.

`beastctl decrypt --key-file ...` goes back to plaintext; `beastctl rekey --key-file OLD --new-key-file NEW` writes a new database under a new master key and a new data key (key ID goes up by one). Both keep `*.predec.bak` / `*.prekey.bak` unless `--delete-original`.

## Keys: what to back up

- The master key file and `beast.bin.keyring` are both required to read the data. **Losing either loses the data.** Keep the master key somewhere other than the data volume, and back up the keyring (it holds only a wrapped data key and is useless without the master key).
- Each backup bundle contains a copy of the keyring as `keyring.json`. The master key is never copied into a bundle.
- Limit of random nonces: re-key (`beastctl rekey`) before about 2^32 page writes under one key (several terabytes of page writes).

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

## Not covered

The Studio user store, the audit log, runtime settings and the key file itself are separate files and are not encrypted by this feature.
