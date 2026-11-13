# Studio: closing the gap to the Figma design

Status: plan, nothing here is shipped. Written 2026-11-13 after comparing Figma file `rE4dmZFSRUzMElTXIKtnhG` (frames 1-11) with the running Studio. Complements [studio.md](studio.md) and [upgrade-spec.md](upgrade-spec.md).

Principle: Studio only shows what the server reports. A Figma control ships only when the engine or API behind it exists. Until then the page keeps its current honest state (read-only, or a "Planned" label). No dead buttons.

## What the engine exposes today (checked in code)

| Area | Reality | Where |
|---|---|---|
| Keys | uint64 keys, 64-bit prefix partitions, no collections | `api/proto/beastdb.proto` |
| gRPC | Get, Put, Delete, Scan, BatchWrite; replication StreamWAL, Ack | proto |
| Indexes | One primary B+ tree plus one fixed in-memory text index | `internal/index/` |
| Tokens | Roles and `expires_at` exist on the server; no last-used | `internal/web/auth/token_store.go`, `handler/token_api.go` |
| Logs | Ring buffer of `{seq, time, level, message}`; no source field | `internal/logbuf/logbuf.go` |
| Replication | Leader lists replicas with connected and acked LSN; followers start by flags; manual promotion with epoch fencing (ADR 0002), no automatic election | `internal/replication/` |
| Settings | Read-only startup config | `handler/settings_api.go` |
| Metrics | Prometheus `/metrics`; Studio keeps ~5 s polling history in the browser only | `internal/metrics/`, `useMetricsHistory.ts` |
| Backups | Offline `beastctl backup/verify/restore`; server only runs checkpoints | `cmd/beastctl`, `handler/ops.go` |

## Phase 0: UI only, no engine work (done 2026-11-13)

Shipped: Settings sub-menu, API Keys role cards, Logs table header and CSV export. Already existed before this plan: the key expiry field and Expires column, Table Editor Prev/Next, and the Split view with a record detail pane. Not done from the table below: page-top header buttons beyond those that already exist.

Everything maps to data that already exists.

| Item | Figma frame | Work |
|---|---|---|
| Settings sub-menu (General / Security / Networking / Node) | 10 | Split the existing read-only cards into sections with a left menu. No Team or Danger zone. |
| API Keys: four role cards on top | 7 | Re-skin `RolesReference` as a 4-up card row. |
| API Keys: expiry field and Expires column | 7 | Send `expires_in_seconds` on create (backend done, spec calls this out); show `expires_at` in the table. |
| Table Editor: record side panel, Prev/Next | 1 | Existing `RecordSheet` content as a persistent right panel; page-based paging on top of the current load-more. |
| Logs: table layout (Time / Level / Event) and client-side Export CSV | 9 | Reuse entries; drop the Source column. |
| Header actions that already work | all | Refresh, Checkpoint now, Download logs placed top-right like Figma. |
| Page titles | all | Keep honest titles ("Logs", not "Logs & Audit"). |

Done when: each page matches its frame's layout using real data only, and `go build ./... && go vet ./... && go test ./... -race` plus `bun test` pass.

## Phase 1: small server additions (done 2026-11-13)

Shipped, with these differences from the table below:
- **Token last-used** is held in memory only (never written, so authenticating does not touch the engine) and resets on restart. The Keys table shows "-" until a key is used after a restart.
- **Log source** is derived from the message text (keywords in `internal/logbuf`), not from tags at each call site, because the server logs through the standard logger. Treat it as a filter aid. `/api/logs` takes `source=`; the Studio filters the time range in the browser.
- **Metrics history** is sampled every 10 s into a 24 h in-memory ring at `/api/metrics/history`; the Metrics page has a Live / 1 h / 24 h picker and Overview shows a request-rate chart. Not persisted.

| Item | Server work | Studio work |
|---|---|---|
| Token last-used | Update a `last_used` timestamp in the gRPC auth path (throttled, e.g. once a minute per token, so reads do not write). | "Last used" column. |
| Metrics history | Server-side ring of sampled gauges (WAL size, dirty pages, ops/s, replica lag) exposed at `/api/metrics/history?window=`. In-memory only, cleared on restart; say so in the UI. | Time-range picker; the Figma charts become real. Overview throughput chart reuses it. |
| Structured log source | Add `source` (engine, auth, replication, studio, grpc) to `logbuf.Entry`; tag existing call sites. | Source column and filter. |
| Log time-range filter | Query params on `/api/logs`. | Filter dropdown. |

Risks: write amplification from last-used (mitigate with throttling and no WAL entry); history memory (bounded ring).

## Phase 2: medium features (done 2026-11-13)

Shipped, with these notes against the descriptions below:
- **Backups** (`internal/backup`, `/api/backups`): create, list, verify and download (tar) from Studio, plus an optional schedule and retention. Built on the engine's existing `CreateSnapshot`, which holds the engine lock while it copies the data file, so **reads and writes pause for the copy** (time grows with data size). Bundles are the same format `beastctl` writes; a test proves `beastctl verify` and `beastctl restore` accept them. **Restore stays offline**: Studio shows the command, not a button. Backups live on the same host as the data (`<data-dir>/backups`), so they do not protect against losing the machine; off-host copy is still manual (Download).
- **Runtime settings** (`internal/runtimecfg`, `/api/settings/runtime`): the WAL checkpoint threshold, the sign-in rate limit, and the backup schedule. Saved to `runtime-settings.json` in the data directory and override the matching flags on the next start. Everything else (TLS, addresses, pool size) stays startup-only.
- **Audit log** (`internal/audit`, `/api/audit`): append-only, hash-chained `audit.log` in the data directory. Records sign-ins (and failures), key create/revoke, record put/delete, user delete, checkpoints, settings changes and backup actions. Never records record values or secrets. The chain detects edits, removals and reordering; it does not stop someone who can rewrite the whole file, and it is not replicated off the host. The server refuses to start if the existing chain is broken.

## Phase 2 (original description)

1. **Server-managed backups.** Move `beastctl` backup, verify and restore logic into an internal package. Add admin endpoints to create a bundle into a configured backup directory, list manifests with size, time and verify status, verify, and stream a download. Scheduling comes last (simple interval, retention count). Restore stays an offline `beastctl` step until restore-to-new-database is designed; the Figma "Restore" button is shown as copy-the-command, not a live action. Needs: concurrent-safe backup while serving (today beastctl assumes a stopped node), so the first design question is a consistent snapshot using checkpoint plus WAL position.
2. **Runtime settings.** Pick the subset that is safe to change live (checkpoint threshold, log level, rate limits), persist to a settings file, apply without restart, audit each change. Everything else stays startup-only. TLS and mTLS toggles are not live-changeable and stay read-only.
3. **Audit log.** Separate append-only log of admin actions (login, token create/revoke, delete user, checkpoint, settings change) with actor and timestamp. Tamper-evident chaining is already on the upgrade-spec "next wave"; start with plain append-only and a hash chain. Studio gets the Logs & Audit tab and the "Audit" level the Figma shows.

## Phase 3: large, engine-level (quarters, each starts with an ADR)

| Feature | Blocker | First step |
|---|---|---|
| Collections / namespaces | Keys are uint64; no string keys or namespace concept. Upgrade spec lists string keys plus namespaces as not started. | ADR in `docs/adr/`: key encoding, migration of existing data, partition interaction. |
| Visual query builder + generated beastctl/SDK command | Needs collections and field-level filters; engine only has Get, Scan, text search. | Define filter push-down in Scan after collections land. |
| Secondary indexes (list, create, slow-query suggestion) | Only one fixed text index. Needs index definitions, persistence, build-in-background, and query stats for suggestions. | ADR on index catalog; depends on collections. |
| Replication management (add replica, promote) | **Partly built (2026-11-13), see [ADR 0002](adr/0002-replication-membership-and-manual-promotion.md).** Built: persisted epoch and role, followers reject a stale-epoch leader, a leader steps down on a higher epoch, a `Fence` RPC, admin `POST /api/replication/promote` with an audit entry, and a Studio Promote action that is disabled with a stated reason unless the node is caught up. Promotion is manual and runs on the replica being promoted, not from the leader's table. **Not built:** the epoch in WAL frames, add-replica (still a command), membership or discovery, client leader lookup, any automatic election. This is not high availability. | Owner decisions listed at the end of the ADR (WAL epoch marker, stuck-fence recovery, sync replication, client discovery). |
| Encryption at rest | Not built (data file, WAL, backups, spill file). | ADR on key management and on-disk format versioning. |
| Two-factor for admins | Studio auth is a signed cookie with local users. | Design TOTP storage and recovery; depends on audit log. |
| Restore to new database | Needs snapshot-consistent backups (Phase 2) and a swap-in procedure. | After backups design. |

## Order and dependencies

```
Phase 0 ──► Phase 1 ──► Phase 2.1 backups ─────► restore-to-new-db
                  │      Phase 2.3 audit log ──► 2FA
                  └──►   Phase 2.2 runtime settings
Phase 3 collections ──► query builder, secondary indexes
Phase 3 replication membership (independent ADR)
Phase 3 encryption at rest (independent ADR)
```

## Rules while this is in flight

- Keep "Planned" labels and read-only states wherever a feature is not shipped; remove them only when the endpoint exists.
- Do not hard-code downstream-application schema in Studio (AGENTS.md); partition labels stay in `partitions.json`.
- No performance or feature-parity claims versus other databases without a benchmark.
- Each phase ends with a Figma-vs-running-Studio screenshot comparison at 1440x900 for the pages it touched.
- Each file stays one responsibility, around 400 lines (split `SettingsView.tsx` while doing the sub-menu).
