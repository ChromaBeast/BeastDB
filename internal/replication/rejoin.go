package replication

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/wal"
	"google.golang.org/grpc"
)

var (
	// ErrNotFenced means Rejoin was called on a node that is not fenced.
	ErrNotFenced = errors.New("replication: only a fenced node can be rejoined")
	// ErrRejoinUnavailable means the process did not give the cluster what it
	// needs to dial a leader (see SetRejoinSupport).
	ErrRejoinUnavailable = errors.New("replication: rejoin is not available on this node")
	// ErrRejoinRefused means the node must not rejoin: its log is not provably a
	// prefix of the leader's. The message says what is different.
	ErrRejoinRefused = errors.New("replication: rejoin refused")
)

// RejoinRefusal is the error behind ErrRejoinRefused.
type RejoinRefusal struct {
	Reason string
	// UnreplicatedRecords is how many records this node holds that the leader
	// lacks, when that is known (0 when it is not).
	UnreplicatedRecords uint64
}

func (e *RejoinRefusal) Error() string { return ErrRejoinRefused.Error() + ": " + e.Reason }
func (e *RejoinRefusal) Unwrap() error { return ErrRejoinRefused }

// RejoinResult describes a node that rejoined as a follower.
type RejoinResult struct {
	Epoch      uint64 `json:"epoch"`
	LeaderAddr string `json:"leaderAddr"`
	AppliedLSN uint64 `json:"appliedLSN"`
	LeaderLSN  uint64 `json:"leaderLSN"`
}

// RejoinDialer opens a connection to a leader's replication address.
type RejoinDialer func(addr string) (*grpc.ClientConn, error)

// SetRejoinSupport gives the cluster what Rejoin needs: a dialer that applies
// the process's TLS and message-size settings, and the context follower loops
// run under (the process lifetime).
func (c *Cluster) SetRejoinSupport(ctx context.Context, dial RejoinDialer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runCtx, c.dial = ctx, dial
}

// Rejoin turns a fenced former leader into a follower of the leader at
// leaderAddr, without wiping its data, when that is provably safe.
//
// Safe means this node's log is a prefix of the leader's: every epoch the node
// logged began at the same LSN on the leader, and the node's last record is
// not past where the leader's log leaves that epoch. Otherwise the node holds
// writes the leader never had (acknowledged to clients, then orphaned by the
// promotion) and Rejoin refuses with a RejoinRefusal saying how many; those
// writes are not silently discarded, and the node stays fenced.
func (c *Cluster) Rejoin(ctx context.Context, leaderAddr string) (RejoinResult, error) {
	c.mu.Lock()
	dial, runCtx := c.dial, c.runCtx
	cur := c.state.Get()
	if cur.Role != RoleFenced {
		c.mu.Unlock()
		return RejoinResult{}, fmt.Errorf("%w (this node is %s)", ErrNotFenced, cur.Role)
	}
	if dial == nil || runCtx == nil {
		c.mu.Unlock()
		return RejoinResult{}, ErrRejoinUnavailable
	}
	if c.rejoining {
		c.mu.Unlock()
		return RejoinResult{}, errors.New("replication: a rejoin is already in progress")
	}
	c.rejoining = true
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.rejoining = false; c.mu.Unlock() }()

	conn, err := dial(leaderAddr)
	if err != nil {
		return RejoinResult{}, fmt.Errorf("dial %s: %w", leaderAddr, err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = conn.Close()
		}
	}()

	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	info, err := beastv1.NewReplicationServiceClient(conn).LogEpochs(qctx, &beastv1.LogEpochsRequest{ReplicaId: c.nodeID, Epoch: cur.Epoch})
	cancel()
	if err != nil {
		return RejoinResult{}, fmt.Errorf("ask %s for its epoch history: %w", leaderAddr, err)
	}
	if Role(info.Role) != RoleLeader {
		return RejoinResult{}, &RejoinRefusal{Reason: fmt.Sprintf("%s is a %s, not the leader; give the address of the current leader", leaderAddr, info.Role)}
	}
	if info.Epoch < cur.Epoch {
		return RejoinResult{}, &RejoinRefusal{Reason: fmt.Sprintf("%s leads at epoch %d, below this node's epoch %d", leaderAddr, info.Epoch, cur.Epoch)}
	}

	mine := c.engine.CurrentLSN()
	remote := make([]wal.EpochMark, len(info.EpochStarts))
	for i, m := range info.EpochStarts {
		remote[i] = wal.EpochMark{LSN: m.Lsn, Epoch: m.Epoch}
	}
	if refusal := checkPrefix(cur, mine, remote, info.LastLsn); refusal != nil {
		return RejoinResult{}, refusal
	}

	// Persist first: a crash after this restarts as a follower of the leader's
	// epoch and the stream re-checks the log match. Then stop serving WAL, then
	// follow.
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.state.becomeFollower(info.Epoch); err != nil {
		return RejoinResult{}, err
	}
	c.engine.SetCommitObserver(nil)
	c.leader.SetActive(false)
	c.engine.SetReadOnly(true)
	f := NewFollowerClient(c.nodeID, c.engine, conn)
	keep = true
	c.attachFollower(runCtx, f)
	log.Printf("replication: fenced node rejoined as a follower of %s at epoch %d (applied LSN %d, leader LSN %d)", leaderAddr, info.Epoch, mine, info.LastLsn)
	return RejoinResult{Epoch: info.Epoch, LeaderAddr: leaderAddr, AppliedLSN: mine, LeaderLSN: info.LastLsn}, nil
}

// checkPrefix decides whether a log ending at lsn mine, with history st, is a
// prefix of a leader log with history remote ending at remoteLast. It returns
// nil when it is provably so.
func checkPrefix(st State, mine uint64, remote []wal.EpochMark, remoteLast uint64) *RejoinRefusal {
	if mine == 0 {
		return nil // an empty log is a prefix of anything
	}
	tail := st.EpochAt(mine)
	if tail == 0 {
		return &RejoinRefusal{Reason: fmt.Sprintf("this node's last record (LSN %d) predates epoch markers in the log, so it cannot be proven to be a prefix of the leader's; wipe the data directory and start it as a follower to reseed it", mine)}
	}
	if mine > remoteLast {
		return &RejoinRefusal{Reason: fmt.Sprintf("this node is at LSN %d, ahead of the leader (%d): it holds %d record(s) the leader never had", mine, remoteLast, mine-remoteLast), UnreplicatedRecords: mine - remoteLast}
	}
	theirs := make(map[uint64]uint64, len(remote))
	for _, m := range remote {
		theirs[m.LSN] = m.Epoch
	}
	// Every epoch this node's log began, up to its tail, must have begun at the
	// same LSN on the leader.
	for _, m := range st.EpochStarts {
		if m.LSN > mine {
			break
		}
		if e, ok := theirs[m.LSN]; !ok || e != m.Epoch {
			n := mine - m.LSN + 1
			return &RejoinRefusal{Reason: fmt.Sprintf("this node's epoch %d began at LSN %d but the leader's log does not say so: the %d record(s) from LSN %d to %d were never on the leader (acknowledged writes lost to the promotion); wipe the data directory to rejoin, or export them first", m.Epoch, m.LSN, n, m.LSN, mine), UnreplicatedRecords: n}
		}
	}
	// And the tail must not run past where the leader's log leaves that epoch:
	// that is the LSN before the leader's next epoch marker, or its last LSN.
	end := remoteLast
	for i, m := range remote {
		if m.Epoch == tail {
			if i+1 < len(remote) {
				end = remote[i+1].LSN - 1
			}
			break
		}
	}
	if mine > end {
		n := mine - end
		return &RejoinRefusal{Reason: fmt.Sprintf("this node wrote %d record(s) at epoch %d after LSN %d, where the leader's epoch %d ended; the leader never had them", n, tail, end, tail), UnreplicatedRecords: n}
	}
	return nil
}
