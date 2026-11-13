package replication

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// ErrNotFollower means Promote was called on a node that is not a follower.
	ErrNotFollower = errors.New("replication: only a follower can be promoted")
	// ErrPromoteInProgress means another Promote call is running.
	ErrPromoteInProgress = errors.New("replication: a promotion is already in progress")
	// ErrLeaderUnreachable means the old leader could not be fenced because it
	// did not answer. Promoting anyway can lose its latest writes and can
	// leave two writable leaders until it is fenced, so it needs AcceptDataLoss.
	ErrLeaderUnreachable = errors.New("replication: leader unreachable, so it cannot be fenced; promoting now can lose its latest writes")
	// ErrCatchUpTimeout means the old leader was fenced but this follower did
	// not apply everything up to the leader's final LSN in time. Nothing was
	// promoted; retrying the same promotion is safe.
	ErrCatchUpTimeout = errors.New("replication: old leader is fenced but this follower has not caught up to its final LSN")
)

// Tunables, variables so tests can shorten them.
var (
	defaultFenceTimeout   = 3 * time.Second
	defaultCatchUpTimeout = 15 * time.Second
	fenceRetryEvery       = 2 * time.Second
	// leaderFreshness is how recent leader contact must be for a follower to
	// count as caught up.
	leaderFreshness = 3 * time.Second
)

// PromoteOptions tunes Promote. Zero values use safe defaults.
type PromoteOptions struct {
	// AcceptDataLoss allows promotion when the old leader cannot be reached to
	// fence it. Writes the leader acknowledged but this follower never
	// received are lost, and the old leader may keep accepting writes until
	// it can be fenced (the new leader keeps trying to fence it).
	AcceptDataLoss bool
	FenceTimeout   time.Duration
	CatchUpTimeout time.Duration
}

// PromoteResult describes a completed promotion.
type PromoteResult struct {
	Epoch uint64 `json:"epoch"`
	// OldLeaderFenced is true when the old leader confirmed the fence; the
	// promotion then lost no acknowledged write.
	OldLeaderFenced bool `json:"oldLeaderFenced"`
	// FinalLSN is the old leader's last LSN as reported by the fence (0 if unreached).
	FinalLSN uint64 `json:"finalLSN"`
	// AppliedLSN is this node's LSN when it began leading.
	AppliedLSN uint64 `json:"appliedLSN"`
	// DataLossPossible is true for an unfenced promotion.
	DataLossPossible bool `json:"dataLossPossible"`
	// LeaderLSNKnown is the highest old-leader LSN this node had heard of (unfenced only).
	LeaderLSNKnown uint64 `json:"leaderLSNKnown,omitempty"`
}

// Status is a node's replication view, for Studio and the audit trail.
type Status struct {
	NodeID     string `json:"nodeID"`
	Role       Role   `json:"role"`
	Epoch      uint64 `json:"epoch"`
	FencedFor  string `json:"fencedFor,omitempty"`
	AppliedLSN uint64 `json:"appliedLSN"`
	// LeaderLSN and LeaderContactMS describe a follower's view of its leader;
	// LeaderContactMS is -1 when it has never heard from one.
	LeaderLSN       uint64 `json:"leaderLSN"`
	LeaderContactMS int64  `json:"leaderContactMS"`
	LagLSN          uint64 `json:"lagLSN"`
	// Promotable is true only for a follower that is in contact with its
	// leader and has applied everything the leader has. When false,
	// PromoteBlockedReason says why.
	Promotable           bool   `json:"promotable"`
	PromoteBlockedReason string `json:"promoteBlockedReason,omitempty"`
}

// Status snapshots this node's role, epoch and, on a follower, whether it may be promoted.
func (c *Cluster) Status() Status {
	st := c.state.Get()
	out := Status{NodeID: c.nodeID, Role: st.Role, Epoch: st.Epoch, FencedFor: st.FencedFor,
		AppliedLSN: c.engine.CurrentLSN(), LeaderContactMS: -1}
	switch st.Role {
	case RoleLeader:
		out.PromoteBlockedReason = "This node is the leader."
	case RoleFenced:
		out.PromoteBlockedReason = fmt.Sprintf("This node was fenced at epoch %d and its data may be behind or diverged; rebuild it as a follower.", st.Epoch)
	default:
		c.mu.Lock()
		f := c.follower
		c.mu.Unlock()
		if f == nil {
			out.PromoteBlockedReason = "No leader connection configured."
			return out
		}
		out.AppliedLSN = f.LastAppliedLSN()
		out.LeaderLSN = f.LeaderLSN()
		if out.LeaderLSN > out.AppliedLSN {
			out.LagLSN = out.LeaderLSN - out.AppliedLSN
		}
		contact := f.LastContact()
		switch {
		case contact.IsZero():
			out.PromoteBlockedReason = "Has not heard from the leader yet."
		case time.Since(contact) > leaderFreshness:
			out.LeaderContactMS = time.Since(contact).Milliseconds()
			out.PromoteBlockedReason = fmt.Sprintf("Leader not heard from for %.0f s; its latest writes may not be here.", time.Since(contact).Seconds())
		case out.LagLSN > 0:
			out.LeaderContactMS = time.Since(contact).Milliseconds()
			out.PromoteBlockedReason = fmt.Sprintf("Behind the leader by %d records (applied %d of %d).", out.LagLSN, out.AppliedLSN, out.LeaderLSN)
		default:
			out.LeaderContactMS = time.Since(contact).Milliseconds()
			out.Promotable = true
		}
	}
	return out
}

// Promote turns this follower into the leader at epoch+1.
//
// With the old leader reachable it fences the leader first (freezing its
// writes and learning its final LSN), waits until it has applied everything up
// to that LSN, and only then starts leading, so no acknowledged write is lost.
// With the leader unreachable it refuses unless AcceptDataLoss is set; the new
// leader then keeps trying to fence the old one in the background.
func (c *Cluster) Promote(ctx context.Context, opts PromoteOptions) (PromoteResult, error) {
	if opts.FenceTimeout <= 0 {
		opts.FenceTimeout = defaultFenceTimeout
	}
	if opts.CatchUpTimeout <= 0 {
		opts.CatchUpTimeout = defaultCatchUpTimeout
	}
	c.mu.Lock()
	if c.state.Get().Role != RoleFollower || c.follower == nil {
		c.mu.Unlock()
		return PromoteResult{}, ErrNotFollower
	}
	if c.promoting {
		c.mu.Unlock()
		return PromoteResult{}, ErrPromoteInProgress
	}
	c.promoting = true
	f := c.follower
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.promoting = false; c.mu.Unlock() }()

	newEpoch := c.state.Epoch() + 1
	res := PromoteResult{Epoch: newEpoch}

	fctx, cancel := context.WithTimeout(ctx, opts.FenceTimeout)
	fr, err := f.client.Fence(fctx, &beastv1.FenceRequest{Epoch: newEpoch, CandidateId: c.nodeID})
	cancel()
	switch {
	case err == nil:
		res.OldLeaderFenced, res.FinalLSN = true, fr.FinalLsn
		if err := c.waitApplied(ctx, f, fr.FinalLsn, opts.CatchUpTimeout); err != nil {
			return res, err
		}
	case status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded:
		if !opts.AcceptDataLoss {
			return res, ErrLeaderUnreachable
		}
		res.DataLossPossible, res.LeaderLSNKnown = true, f.LeaderLSN()
	default:
		// The leader answered and refused (already fenced for someone else, or
		// at a higher epoch): never override that, even with AcceptDataLoss.
		return res, fmt.Errorf("%w: old leader answered: %s", ErrFenceRefused, status.Convert(err).Message())
	}

	if err := c.becomeLeader(f, newEpoch, &res); err != nil {
		return res, err
	}
	if res.DataLossPossible {
		go c.fenceUntilDone(f, newEpoch, fenceRetryEvery)
	}
	return res, nil
}

// waitApplied blocks until the follower has applied lsn.
func (c *Cluster) waitApplied(ctx context.Context, f *FollowerClient, lsn uint64, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for f.LastAppliedLSN() < lsn {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("%w (applied %d of %d)", ErrCatchUpTimeout, f.LastAppliedLSN(), lsn)
		case <-tick.C:
		}
	}
	return nil
}

// becomeLeader persists the new epoch, then stops following, then opens
// writes. Persisting first means a crash at any point restarts as leader at
// the new epoch (ResolveRole), never as a follower of the old leader.
func (c *Cluster) becomeLeader(f *FollowerClient, epoch uint64, res *PromoteResult) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.state.Set(State{Epoch: epoch, Role: RoleLeader, ServedEpoch: epoch, PromotedAtLSN: f.LastAppliedLSN()}); err != nil {
		return err
	}
	if c.syncStop != nil {
		c.syncStop()
		<-c.syncDone
	}
	res.AppliedLSN = c.engine.CurrentLSN()
	c.engine.SetCommitObserver(c.leader)
	c.leader.SetActive(true)
	c.engine.SetReadOnly(false)
	log.Printf("replication: promoted to leader at epoch %d (LSN %d, old leader fenced: %t)", epoch, res.AppliedLSN, res.OldLeaderFenced)
	return nil
}

// fenceUntilDone keeps asking the old leader to freeze until it answers, so a
// partitioned or restarted old leader is stopped as soon as it is reachable.
func (c *Cluster) fenceUntilDone(f *FollowerClient, epoch uint64, every time.Duration) {
	c.mu.Lock()
	ctx := c.runCtx
	c.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	for ctx.Err() == nil {
		fctx, cancel := context.WithTimeout(ctx, every)
		_, err := f.client.Fence(fctx, &beastv1.FenceRequest{Epoch: epoch, CandidateId: c.nodeID})
		cancel()
		if err == nil {
			log.Printf("replication: old leader reached and fenced at epoch %d", epoch)
			return
		}
		if status.Code(err) == codes.FailedPrecondition {
			log.Printf("replication: old leader will not be fenced at epoch %d: %v", epoch, err)
			return
		}
		select {
		case <-time.After(every):
		case <-ctx.Done():
		}
	}
}
