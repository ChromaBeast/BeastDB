package replication

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ChromaBeast/beastdb/internal/api"
)

// ErrFenceRefused means this leader will not be fenced by that candidate: its
// epoch is not newer, or it is already fenced for a different candidate.
var ErrFenceRefused = errors.New("replication: fence refused")

// Cluster ties a node's persisted epoch and role to its engine and its
// replication endpoints. It is the one place that decides whether this node
// may accept writes.
type Cluster struct {
	nodeID string
	engine *api.Engine
	state  *StateStore
	leader *LeaderServer

	mu        sync.Mutex // guards the fields below and serializes role changes
	follower  *FollowerClient
	syncStop  context.CancelFunc
	syncDone  chan struct{}
	runCtx    context.Context
	promoting bool
}

// NewCluster loads the persisted state, reconciles it with the -role flag
// (see ResolveRole), and puts the engine and leader server into the matching
// mode. A returned error means the node must not start.
func NewCluster(engine *api.Engine, dataDir, nodeID string, flag Role, leader *LeaderServer) (*Cluster, error) {
	st, err := LoadStateStore(dataDir)
	if err != nil {
		return nil, err
	}
	cur := st.Get()
	role, err := ResolveRole(cur, flag)
	if err != nil {
		return nil, err
	}
	if cur.Role != role {
		next := cur
		next.Role = role
		if role == RoleLeader {
			next.Epoch = max(cur.Epoch, 1)
			next.ServedEpoch = next.Epoch
		}
		if err := st.Set(next); err != nil {
			return nil, err
		}
	}
	if role != flag {
		log.Printf("replication: persisted state overrides -role %s: starting as %s at epoch %d", flag, role, st.Epoch())
	}
	c := &Cluster{nodeID: nodeID, engine: engine, state: st, leader: leader}
	leader.cluster = c
	engine.SetReadOnly(role != RoleLeader)
	leader.SetActive(role != RoleFollower)
	if role == RoleLeader {
		engine.SetCommitObserver(leader)
	}
	return c, nil
}

// Epoch is the highest epoch this node has seen.
func (c *Cluster) Epoch() uint64 { return c.state.Epoch() }

// ServedEpoch is the epoch this node leads (or last led) at.
func (c *Cluster) ServedEpoch() uint64 { return c.state.Get().ServedEpoch }

// Role is the node's current persisted role.
func (c *Cluster) Role() Role { return c.state.Get().Role }

// stepDown fences this leader because a peer reported a higher epoch. It
// persists before it freezes, so a crash in between restarts fenced.
func (c *Cluster) stepDown(epoch uint64, by string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur := c.state.Get()
	if cur.Role == RoleFollower || epoch <= cur.Epoch {
		return nil
	}
	next := cur
	next.Epoch, next.Role, next.FencedFor = epoch, RoleFenced, by
	if err := c.state.Set(next); err != nil {
		return err
	}
	c.engine.SetReadOnly(true)
	log.Printf("replication: stepped down: %s reported epoch %d (this node led at %d); writes are now refused", by, epoch, cur.ServedEpoch)
	return nil
}

// fence implements the Fence RPC: freeze at epoch for candidate and return the
// final LSN. No client write can land after this returns.
func (c *Cluster) fence(epoch uint64, candidate string) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cur := c.state.Get()
	switch {
	case cur.Role == RoleFollower:
		return 0, fmt.Errorf("%w: this node is a follower", ErrFenceRefused)
	case cur.Role == RoleFenced && cur.Epoch == epoch && cur.FencedFor == candidate:
		// Retry by the same candidate: already frozen, report the same LSN.
		return c.engine.CurrentLSN(), nil
	case epoch <= cur.Epoch:
		who := ""
		if cur.Role == RoleFenced {
			who = fmt.Sprintf(" (fenced for %q)", cur.FencedFor)
		}
		return 0, fmt.Errorf("%w: epoch %d is not newer than this node's %d%s", ErrFenceRefused, epoch, cur.Epoch, who)
	}
	next := cur
	next.Epoch, next.Role, next.FencedFor = epoch, RoleFenced, candidate
	if err := c.state.Set(next); err != nil {
		return 0, err
	}
	c.engine.SetReadOnly(true)
	lsn := c.engine.CurrentLSN()
	log.Printf("replication: fenced at epoch %d for %s; final LSN %d", epoch, candidate, lsn)
	return lsn, nil
}

// RunFollower attaches the follower and runs its sync loop (reconnecting on
// error) until ctx ends or the node is promoted.
func (c *Cluster) RunFollower(ctx context.Context, f *FollowerClient) {
	f.SetStateStore(c.state)
	c.mu.Lock()
	c.follower = f
	c.runCtx = ctx
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	c.syncStop, c.syncDone = cancel, done
	c.mu.Unlock()
	go func() {
		defer close(done)
		var lastErr string
		for loopCtx.Err() == nil {
			err := f.SyncLoop(loopCtx)
			if loopCtx.Err() != nil {
				continue
			}
			wait := 200 * time.Millisecond // clean end of stream
			if err != nil {
				wait = time.Second
				if msg := err.Error(); msg != lastErr {
					log.Printf("Follower sync stream dropped: %v. Reconnecting in 1s...", err)
					lastErr = msg
				}
			}
			select {
			case <-time.After(wait):
			case <-loopCtx.Done():
			}
		}
	}()
}
