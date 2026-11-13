# ADR 0002: Replication membership and manual promotion

**Status:** proposed, 13 November 2026. The first slice (sections marked *built*) is
implemented; the rest is design only. Items under "Decisions for the owner" are open.

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
- **Not yet in WAL frames.** The slice stamps the epoch on records as they are streamed but
  does not write it into the WAL. The next step is an `OpEpoch` marker record appended when a
  node starts leading, so every record's epoch is derivable from the log. That changes the WAL
  format (recovery, `beastctl`, a version bump) and is deferred; see decisions. What it buys:
  a follower could prove its log is a prefix of the leader's at a given `(LSN, epoch)` and
  could detect equal-LSN divergence. Without it, the slice refuses on the signals it does have
  (below) and requires a rebuild after any unfenced promotion.

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
  follower (its data may hold writes the new leader never saw): wipe the data directory to
  rejoin it. A promoted node restarted with `-role follower` stays leader. A persisted follower
  cannot be started as leader by flag; it must be promoted, which bumps the epoch.
- **The fenced leader keeps serving WAL.** The candidate catches up from it after the fence.
- **Retry after an unfenced promotion (*built*).** If the old leader could not be reached, the
  new leader keeps calling `Fence` every 2 s until it answers, so a healed partition or a
  restarted old leader is stopped without operator action.

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
| Fenced for X, X dies before promoting | Y cannot fence at the same epoch (refused by design). | **No supported recovery yet.** Open decision. |
| Partition, old leader alive on its side | Candidate cannot fence; refused unless `acceptDataLoss`. If forced, old leader keeps accepting writes until the partition heals. | Writes acknowledged by the old leader in that window never reach the new leader: **split-brain for the length of the partition.** Stop the old leader first (power, network, process). |
| Old leader returns, never fenced | Restarts as leader at its old epoch. Fenced by the new leader's retry or by any follower already at the new epoch. | Writes it took before being fenced are discarded with its data; wipe it to rejoin. |
| Old leader returns, was fenced | Stays read-only. Refuses `-role follower` on the stale data. | None; rebuild is manual. |
| Crash during promotion | New epoch is persisted before writes open. A crash between restarts as leader at the new epoch, never as follower of the old leader. | None. |
| Two forced promotions with both leaders unreachable | Nothing prevents it: both reach the same epoch. | **Split-brain.** Needs a quorum to prevent. |
| Other followers ahead of the promoted node | Leader refuses a stream whose `from_lsn` is past its own LSN. | Those followers hold writes the new leader lacks and need a rebuild. |
| Equal LSN, different history | **Not detected** in this slice (needs epochs in the WAL). | Mitigated by wiping any node that was fenced or whose promotion was unfenced. |
| Leader crash with acknowledged, unreplicated writes | Replication is async. Fence cannot run on a dead node. | Lost on forced promotion; this is the cost of async replication. |

### What automatic election would still need

None of this is built, and until all of it is, nothing here is high availability.

1. **Membership and quorum:** a persisted peer list and an odd voter count, so a minority
   partition cannot elect a leader.
2. **Leader lease:** a leader that stops writing when it cannot reach a majority within a bound,
   so an isolated leader fences *itself* without hearing from anyone.
3. **Vote RPC:** grant at most one vote per epoch; grant only to a candidate whose log is at
   least as new, compared by `(last epoch, last LSN)`. Needs the epoch in the WAL.
4. **Log matching and truncation (or snapshot rejoin)** for a returning node whose tail
   diverged.
5. **Quorum-acknowledged writes:** otherwise election still loses acknowledged writes, and the
   system would claim availability while silently losing data.
6. **Client discovery** (`GetClusterInfo`, advertise addresses) and re-pointing of followers.
7. **Fault-injection tests** (partitions, clock pauses, crash at every step) at the scale of
   the failure table above, not only the in-process tests below.

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
  replicas as before.

## Decisions for the owner

1. **WAL epoch marker.** Add an `OpEpoch` record (WAL format version bump, recovery and
   `beastctl` changes) now, or after promotion has been used in practice? Needed for automatic
   election and for detecting equal-LSN divergence.
2. **Stuck fence.** If a leader is fenced for a candidate that then dies, no other node can be
   promoted. Options: an operator-only "re-fence at a higher epoch" that names the abandoned
   candidate, or a documented wipe-and-reseed of the old leader. Not built.
3. **Rejoining a fenced node.** Keep "wipe and rejoin", or allow a fenced node to rejoin as a
   follower when the new leader can prove (via item 1) that its log is a prefix?
4. **Forced promotion in Studio.** Kept API-only so the dangerous path needs a deliberate
   `acceptDataLoss`. Say if you want it in the UI behind a typed confirmation.
5. **Sync replication.** Is a `min-sync-replicas` write mode wanted before any automatic
   election? Without it, election loses acknowledged writes.
6. **Client discovery.** Build `GetClusterInfo` and an advertise-address flag now, so SDKs can
   follow a promotion without configuration?
7. **Leader-driven promotion.** Studio on the leader could promote a replica it sees (a control
   message on the WAL stream), at the cost of replicas advertising an address. Today promotion
   runs on the replica.
