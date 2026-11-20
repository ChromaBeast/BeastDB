# ADR 0004: Replication membership and manual promotion

**Status:** proposed, 13 November 2026; updated 20 November 2026. Sections marked *built* are
implemented. The first slice was manual promotion with fencing; the follow-up (epoch in the WAL,
rejoin of a fenced node, Studio replica view) is built and marked *built (follow-up)*. Automatic
election is a design only and is **not built**. Items under "Decisions for the owner" are open.

## Context

Replication today is one leader streaming its WAL to followers over gRPC. Roles come from the
`-role` flag. A follower finds its leader from `-leader-addr`; a leader learns of followers only
while they hold a stream open, and forgets them on restart. Nothing records which node is the
leader, nothing stops two nodes from both accepting writes, and there is no way to move
leadership short of editing flags and restarting.

The Studio design shows *Add replica* and *Promote replica*. Promotion without fencing creates
two writable leaders (split-brain): both acknowledge writes the other never sees, and there is
no way to merge them afterwards. So fencing comes first and the button comes last.

BeastDB replicates asynchronously: a write is acknowledged to the client once it is fsynced on
the leader, before any follower has it. That decides what promotion can promise (below).

## Decision

Manual promotion first. A human decides to move leadership; the system makes that move safe
when the old leader is reachable and loudly unsafe when it is not. Automatic election is out of
scope and is described at the end only to say what it would still need.

### Membership: static, no protocol

- A follower knows its leader from `-leader-addr`. A leader knows its replicas from live streams
  (unchanged). There is no cluster-wide member list and no discovery.
- The one piece of cluster state a node persists is its **epoch** and **role**, in
  `<data-dir>/cluster-state.json` (*built*): `{epoch, role, servedEpoch, fencedFor, promotedAtLSN}`
  written by temp file, fsync, rename. The file is the source of truth; `-role` is only the
  starting hint for a node with no file.
- After a promotion the operator repoints the other followers (restart with the new
  `-leader-addr`) and clients. *Add replica* stays "run this command" in Studio; there is no
  join protocol.

### Epoch (term)

- An epoch is a number that only increases. The first leader runs at epoch 1. Every promotion
  runs at the previous epoch + 1. A node never lowers its epoch (`StateStore.Set` refuses).
- **Carried in replication RPCs (*built*):** `StreamWALRequest.epoch` (the highest epoch the
  follower has seen), `WALRecordMessage.epoch` (the epoch the record was written at),
  `ReplicationAck.epoch`, `AckResponse.epoch` and `leader_lsn`, and the new `Fence` RPC.
- **In the WAL (*built (follow-up)*).** When a node starts leading at epoch E it appends an
  `OpEpoch` record (type `0x05`, no key, value = E as 8 little-endian bytes) before it opens for
  client writes: at startup if the log's tail is not already at E, and on promotion after the new
  epoch is persisted. The marker is an ordinary frame (same CRC, same sealing when the log is
  encrypted) and takes an LSN, so it replicates like any record and a follower logs it at the same
  LSN. Every record's epoch is the epoch of the last marker at or before it.
- **Format and compatibility.** The frame layout is unchanged, so there is no log-format version to
  bump; the record type is the version, as `headerCRCFlag` was before it. Logs written before this
  change hold no markers and are read exactly as before: their records are at epoch 0 ("unknown")
  and the first leader start after upgrade appends a marker after them. Recovery and the plaintext
  and encrypted scanners skip a type they do not know, so an older binary can still open a log that
  holds markers. **An older *follower* must not stream from a newer leader**: it applies nothing for
  a marker it does not know and its LSN numbering drifts one behind the leader's. Upgrade every
  follower before restarting the leader on the new build.
- **Epoch history.** The marker positions (`epochStarts`: `{lsn, epoch}` pairs) are also kept in
  `cluster-state.json`, because checkpoint rotation keeps only one archived WAL generation. At start
  the history is merged with the markers found in the log (so a crash between logging a marker and
  recording it loses nothing, and a deleted history is rebuilt), and a follower records each marker
  as it applies it. History that contradicts the log (same LSN, different epoch, or an epoch that
  goes down as the LSN goes up) refuses to start.
- **What it is used for.**
  1. *Log matching at stream start.* A follower sends the epoch of its last record
     (`StreamWALRequest.last_log_epoch`). The leader refuses with `FailedPrecondition` ("log
     history diverged") if its own log has a different epoch at that LSN. This catches the case the
     LSN check cannot: an old leader and its successor at the **same LSN with different records**.
  2. *Frames judged against the follower's own log.* A follower rejects a frame stamped with an
     epoch below the one its log has reached, and an epoch marker below the log's epoch or above
     its own frame's, whatever epoch the stream reports (`ErrStaleLeader`). This does not replace
     the RPC-level check; it holds even if the node's `cluster-state.json` epoch were lost.
  3. *Rejoin* of a fenced node (below).
- **What it does not prove.** Two leaders forced to the same epoch with both leaders unreachable
  (the "two forced promotions" row below) still share an epoch, so matching epochs do not prove
  matching records there. A log from before markers (epoch 0) cannot be checked at all.

### Fencing the old leader

Fencing makes the old leader stop accepting writes *before* the new one starts.

- **`Fence(epoch, candidate)` RPC (*built*).** The candidate calls it on the old leader. The
  leader persists `role=fenced, epoch=<new>, fencedFor=<candidate>`, sets its engine read-only,
  and returns its final LSN. It is idempotent for the same candidate and epoch, and refuses a
  different candidate at that epoch, a stale epoch, and any node that is not a leader. The
  refusal of a second candidate is what stops two followers being promoted at one epoch while
  the leader is up.
- **No write can land after the fence returns.** The read-only check runs inside the
  group-commit batch under the engine lock (`runBatch`), not only at the API entry, so a write
  already queued is refused too. The final LSN is read after the freeze and is exact.
- **Fencing by contact (*built*).** Any replication RPC carrying an epoch higher than the
  leader's makes it step down: persist `fenced`, go read-only, refuse the call. So an old
  leader that comes back is fenced by the first follower that already knows the new epoch.
- **Persisted.** A fenced node restarts fenced whatever `-role` says. It refuses to start as a
  follower by flag (its data may hold writes the new leader never saw); it leaves the fence only
  through the rejoin operation below, or by wiping its data directory. A promoted node restarted with `-role follower` stays leader. A persisted follower
  cannot be started as leader by flag; it must be promoted, which bumps the epoch.
- **The fenced leader keeps serving WAL.** The candidate catches up from it after the fence.
- **Retry after an unfenced promotion (*built*).** If the old leader could not be reached, the
  new leader keeps calling `Fence` every 2 s until it answers, so a healed partition or a
  restarted old leader is stopped without operator action.

### Rejoining a fenced node (*built (follow-up)*)

Replaces "wipe the data directory" for the common case. `POST /api/replication/rejoin`
(admin-only, body `{"leaderAddr": "host:port"}`) and `beastctl rejoin --web <fenced node's Studio
URL> --leader <leader gRPC address>` (admin password from `BEASTDB_ADMIN_PASSWORD` or
`--password-file`, never an argument).

- The fenced node asks the leader for its role, epoch, last LSN and epoch history
  (`LogEpochs` RPC, allowed to the replication and admin token roles). It proceeds only if the
  target is a leader at an epoch at least its own and its own log is **provably a prefix** of the
  leader's: every epoch marker it logged up to its last record began at the same LSN on the
  leader, and its last record is not past the LSN where the leader's log leaves that epoch.
- **It refuses, and says so**, when the node holds records the leader lacks (writes acknowledged by
  the old leader during a partition, or after an unfenced promotion). The message names how many
  records, from which LSN, and that they were acknowledged writes lost to the promotion; the
  response carries `unreplicatedRecords`. The node stays fenced and nothing is changed or deleted.
  Its remaining options are to export those records, then wipe and reseed.
- It also refuses when its last record predates epoch markers (cannot be proven), when it is ahead
  of the leader, when the target is not the leader, and when the node is not fenced.
- On success it persists `role=follower` at the leader's epoch first, then stops serving WAL, then
  follows; a crash in between restarts as a follower and the stream re-checks the log match. The
  operation is audited as `replication.rejoin` with actor, target leader and the outcome in words,
  refusals included.
- **Not covered:** the "fenced for X, X dies" row (no new leader exists, so there is nothing to
  rejoin to) is unchanged and still open, and rejoin does not truncate: a node that holds extra
  writes is refused, not repaired. Truncating applied writes needs the engine to roll back state it
  has already applied (see the automatic-election design).

### What a follower needs before promotion

- **Old leader reachable (the safe path).** Fence, take the final LSN from the reply, wait until
  the follower has applied that LSN (default timeout 15 s), then persist the new epoch, stop
  following, and open writes, in that order. No acknowledged write is lost: the leader accepted
  nothing past the final LSN and the candidate has everything up to it. *Caught up* is therefore
  not a safety requirement on this path; it is a UI precondition (below) and a cheap sanity
  check. A fence that succeeds but a catch-up that times out leaves the old leader fenced and
  the candidate a follower; the promotion can be retried.
- **Old leader unreachable.** Refused with `leader_unreachable` unless the caller passes
  `acceptDataLoss`. If forced, the promoted node has only what it had replicated: anything the
  old leader acknowledged after that is lost to the cluster, and the old leader may keep taking
  writes until the retry above reaches it. The audit entry says so in words. Studio does not
  offer this path; it is API-only.
- **Studio gate (*built*).** The Promote button is enabled only when the follower has heard from
  its leader within 3 s (record or heartbeat ACK, sent at least once per second) and its applied
  LSN equals the leader's last reported LSN. Otherwise it is disabled and shows the reason.
- **Where it runs.** Promotion is performed by the node being promoted, from its own Studio, as
  that node holds the leader connection. The leader's replica table does not carry a Promote
  button; it says where to go.

### How clients find the new leader

*Built:* a write to a read-only or fenced node fails with `FailedPrecondition`; `/api/replication`
on any node reports role and epoch. Clients are expected to hold a list of node addresses and
retry writes elsewhere on that error. *Not built:* a `GetClusterInfo` RPC returning the leader's
advertised address and epoch, and an advertise-address flag. Neither is needed for manual
promotion, and both are needed before clients can follow a leader without configuration.

### Failure cases

| Case | What happens | Loss or risk |
|---|---|---|
| Promote, old leader up, follower lagging | Fence, wait for final LSN, then promote. | None. Writers see read-only errors after the fence. |
| Fence ok, catch-up times out | Old leader stays fenced, nothing promoted. Retry is idempotent. | Writes unavailable until a retry succeeds. |
| Fenced for X, X dies before promoting | Y cannot fence at the same epoch (refused by design). | **No supported recovery yet.** Open decision (2). |
| Partition, old leader alive on its side | Candidate cannot fence; refused unless `acceptDataLoss`. If forced, old leader keeps accepting writes until the partition heals. | Writes acknowledged by the old leader in that window never reach the new leader: **split-brain for the length of the partition.** Stop the old leader first (power, network, process). |
| Old leader returns, never fenced | Restarts as leader at its old epoch. Fenced by the new leader's retry or by any follower already at the new epoch. | Writes it took before being fenced exist only on it. Rejoin refuses and reports how many; export them, then wipe to reseed. |
| Old leader returns, was fenced | Stays read-only. Refuses `-role follower` by flag. | None. Rejoin succeeds if its log is a prefix of the new leader's, else refuses. |
| Crash during promotion | New epoch is persisted before writes open. A crash between restarts as leader at the new epoch, never as follower of the old leader. | None. |
| Two forced promotions with both leaders unreachable | Nothing prevents it: both reach the same epoch. | **Split-brain.** Needs a quorum to prevent. |
| Other followers ahead of the promoted node | Leader refuses a stream whose `from_lsn` is past its own LSN, or whose last record's epoch differs from the leader's at that LSN. | Those followers hold writes the new leader lacks and need a rebuild. |
| Equal LSN, different history | Detected by the epoch markers: stream start is refused ("log history diverged") and rejoin is refused. | Not detected for logs from before markers (epoch 0) or for two leaders forced to the same epoch. |
| Leader crash with acknowledged, unreplicated writes | Replication is async. Fence cannot run on a dead node. | Lost on forced promotion; this is the cost of async replication. |

### Automatic election: design only, **not built**

Nothing in this ADR is high availability, and this section does not make it so. It records the
design that would be needed, what it would cost and how it fails, and why it was not built.

**Prerequisites that now exist:** persisted epochs, fencing by contact, epoch markers in the WAL
(`(last epoch, last LSN)` is now computable for any node), log matching at stream start, and a way
to find where two logs diverge.

**Design: Raft-lite on the existing WAL.** Leader election and log replication with
quorum-acknowledged writes; no joint-consensus membership change and no pre-built snapshots.

1. *Membership.* A persisted voter list (`cluster-members.json`) with an odd count, identical on
   every node, changed only by editing it and restarting (no online reconfiguration). A node that is
   not in the list never votes.
2. *Vote RPC.* `RequestVote(epoch, candidate, lastLogEpoch, lastLSN)`. A voter grants at most one
   vote per epoch, persists `votedFor` **before** replying, and grants only if
   `(lastLogEpoch, lastLSN)` is at least its own. A candidate that wins a majority logs its
   `OpEpoch` marker and starts leading. Randomized election timeouts break split votes; a pre-vote
   round stops a node that was merely partitioned from forcing an epoch bump on return.
3. *Quorum-acknowledged writes.* The leader acknowledges a client write only after it is fsynced
   locally and on a majority of voters (a `commit LSN`). Followers must ack their **durable** LSN, not
   their applied LSN. Without this, an election loses writes the old leader acknowledged, which is
   the data loss manual promotion already documents. This is a new write mode, and the design choice
   in decision 5.
4. *Followers apply only up to the commit LSN.* The engine today applies a record to its pages as
   it logs it. A follower holding an uncommitted tail that a new leader never had must be able to
   discard it. Either apply after commit (a change to the replicated apply path and to read
   semantics), or treat any diverged follower as needing a snapshot reseed
   (`CreateSnapshot` exists; an automated install does not).
5. *Leader lease.* A leader that has not heard a majority within the lease stops accepting writes
   and steps down by itself. With (3), write safety does **not** depend on clocks: a stale leader
   cannot reach a majority because voters reject its epoch. The lease only bounds how long a
   partitioned leader serves stale reads, and followers wait out the lease plus a drift allowance
   before standing for election.
6. *Client discovery.* `GetClusterInfo` and advertised addresses (decision 6) so clients and the
   remaining followers re-point without operator action.

**Failure modes the design must survive, each needing a test:**

| Failure | Required outcome |
|---|---|
| Leader isolated with a minority | It cannot commit (no majority), steps down at lease expiry; the majority elects a new leader; writes the old leader appended but never committed are never acknowledged. |
| Partition heals with two leaders at different epochs | The lower epoch steps down on first contact; its uncommitted tail is discarded (reseed or truncate). No acknowledged write is lost. |
| Clock pause (GC, VM freeze) on the leader longer than the lease | It resumes believing it leads; its next replication is rejected by the higher epoch; its reads may be stale until then. |
| Crash after granting a vote, before replying | Restart remembers `votedFor`; it must not vote twice in one epoch. |
| Crash of the new leader between winning and logging its marker | Restart is a follower at the new epoch; a new election runs at a higher one. |
| Split vote | Randomized timeouts converge; no leader is elected without a majority. |
| Lost majority (2 of 3 down) | Writes stop. This is intentional: availability is given up to keep acknowledged writes. |
| Slow or full-disk voter in the quorum | Commit latency follows the majority, so one slow voter of three is tolerated and two are not. |
| Flapping node | Pre-vote prevents epoch inflation. |

**What it would cost.** A write waits for a majority fsync and a round trip instead of one local
fsync, so write latency and throughput change; the size of that change has not been measured and
no figure is claimed. It adds a vote RPC, durable vote state, a commit LSN and apply-after-commit
(or snapshot install), and it makes membership a configuration artifact that must be right on every
node. It also needs a test harness that does not exist yet: a controllable network and clock
(drop, delay, partition, pause) around the in-process nodes, with crash injection at each step above.
The in-process tests added so far (partition, restart, stale leader, divergence) cover the manual
path only.

**Decision:** not built. It would be unsafe to ship without (3) and (4), and (3) is an owner
decision about write semantics. Until all of it exists and is tested under partition and restart,
promotion stays manual and BeastDB is not described as highly available. Nothing about this
section is implemented.

## Consequences

- Followers reject a leader whose epoch is lower than theirs (`ErrStaleLeader`) and apply
  nothing from it. Leaders step down on a higher epoch. Both persist before acting.
- Every node now serves the replication service on gRPC; a node that is not a leader answers
  `FailedPrecondition`. `Fence` joins `StreamWAL` and `Ack` in the replication role's allowed
  methods.
- New admin endpoint `POST /api/replication/promote` (body `{"acceptDataLoss": false}`), audited
  as `replication.promote` with the actor, epoch, whether the old leader was fenced, and for an
  unfenced promotion the words "WITHOUT fencing" and "data loss possible". Refusals are audited
  too. `GET /api/replication` gains a `cluster` object (role, epoch, lag, `promotable` and
  the reason when not).
- Followers send a heartbeat ACK once a second even when idle, a small constant load per replica.
- Tests (in-process nodes, real gRPC): lossless promotion under concurrent writers, idempotent
  and exclusive fencing, step-down on a higher epoch (both RPCs) and across a restart, stale
  leader rejected by a follower, partition then forced promotion then heal, old leader restarted
  after promotion, crash between persisting the epoch and opening writes, lagging follower with
  catch-up timeout then retry, follower ahead of leader, state file torn write and corruption.
- Studio: the Promote action and its reasons; a Studio Replication page on a leader shows
  replicas with real acknowledged LSN, lag and time since last ack, classified as caught up,
  lagging, no recent ack (connected but silent) or disconnected. Lag is only as fresh as the last ack.
  *Add replica* shows the exact `beastdb-server -role follower ...` command built from the leader's
  real settings (leader address, TLS and encryption caveats) and says plainly that Studio cannot
  start a replica: there is no join protocol. A fenced node shows a Rejoin form instead.
- Follow-up tests: epoch marker written once and replicated at the same LSN, history rebuilt from the
  WAL, legacy logs at epoch 0, promotion and crash-before-marker repair, equal-LSN divergence refused,
  stale frames rejected from the follower's own log, rejoin of a prefix node, refusal with the count
  of orphaned records, plaintext and encrypted markers across checkpoint rotation.
- New `LogEpochs` RPC, `POST /api/replication/rejoin`, audit action `replication.rejoin`,
  `beastctl rejoin`. `GET /api/replication` gains a `join` object; replica rows gain `lastAckAgoMS`.

## Decisions for the owner

1. ~~WAL epoch marker.~~ Done (follow-up), see "In the WAL".
2. **Stuck fence.** If a leader is fenced for a candidate that then dies, no other node can be
   promoted. Options: an operator-only "re-fence at a higher epoch" that names the abandoned
   candidate, or a documented wipe-and-reseed of the old leader. Not built (rejoin needs a leader to
   rejoin to).
3. ~~Rejoining a fenced node.~~ Done (follow-up): rejoin when the log is a provable prefix, refuse
   otherwise. Truncating a node that holds extra writes (instead of refusing it) is not built.
4. **Forced promotion in Studio.** Kept API-only so the dangerous path needs a deliberate
   `acceptDataLoss`. Say if you want it in the UI behind a typed confirmation.
5. **Sync replication.** Is a `min-sync-replicas` write mode wanted before any automatic
   election? Without it, election loses acknowledged writes. This gates the whole election design.
6. **Client discovery.** Build `GetClusterInfo` and an advertise-address flag now, so SDKs can
   follow a promotion without configuration?
7. **Leader-driven promotion.** Studio on the leader could promote a replica it sees (a control
   message on the WAL stream), at the cost of replicas advertising an address. Today promotion
   runs on the replica.
8. ~~Replication under gRPC token auth.~~ Found and fixed (follow-up): `cmd/server` always
   enforces bearer tokens on gRPC, and the follower client sent none, so a real follower process
   was refused by a real leader ("missing authorization header"; the in-process tests bypass the
   interceptor). Followers and `rejoin` now send `-leader-token` (default `-api-token`), which may be
   an admin or `replication`-role token. Checked end to end with real processes: replicate, promote,
   fence, rejoin.
