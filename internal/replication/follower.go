package replication

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
)

var (
	// ErrReplicationGap indicates a non-contiguous LSN was received.
	ErrReplicationGap = errors.New("replication gap detected: non-contiguous LSN")
	// ErrStaleLeader indicates the leader's epoch is lower than one this
	// follower has already seen: it was superseded by a promotion.
	ErrStaleLeader = errors.New("replication: leader epoch is older than this follower's; leader was superseded")
)

const (
	ackBatchSize     = 100                    // send ACK every N records
	ackBatchInterval = 100 * time.Millisecond // or every 100 ms, whichever first
	heartbeatEvery   = time.Second            // unchanged progress is still acked this often
)

// FollowerClient connects to the Leader node and replicates WAL records into local state.
type FollowerClient struct {
	replicaID      string
	engine         *api.Engine
	client         beastv1.ReplicationServiceClient
	lastAppliedLSN uint64
	mu             sync.RWMutex

	ackMu        sync.Mutex // serializes ACK RPCs so the leader sees them in LSN order
	lastAckedLSN uint64     // guarded by ackMu

	state       *StateStore // nil: epochs are not tracked
	leaderLSN   uint64      // highest leader LSN seen (records or ack replies); guarded by mu
	lastContact time.Time   // last record or ack reply from a leader at our epoch; guarded by mu
}

// NewFollowerClient initializes a replication worker targeting the primary leader.
func NewFollowerClient(replicaID string, engine *api.Engine, conn *grpc.ClientConn) *FollowerClient {
	return &FollowerClient{
		replicaID:      replicaID,
		engine:         engine,
		client:         beastv1.NewReplicationServiceClient(conn),
		lastAppliedLSN: engine.CurrentLSN(),
	}
}

// SyncLoop continuously streams and applies WAL records from the Leader until context cancels.
// Fix #4: ACKs are batched — sent every ackBatchSize records OR every ackBatchInterval ms,
// whichever comes first. This reduces ACK round-trips from O(writes/sec) to O(writes/100).
// The interval ACK runs on its own ticker, so progress is acknowledged even when
// the stream goes idle after fewer than ackBatchSize records.
func (f *FollowerClient) SyncLoop(ctx context.Context) error {
	tickCtx, stopTicker := context.WithCancel(ctx)
	defer stopTicker()
	go f.ackTicker(tickCtx)

	f.mu.Lock()
	fromLSN := f.lastAppliedLSN + 1
	f.mu.Unlock()

	stream, err := f.client.StreamWAL(ctx, &beastv1.StreamWALRequest{
		ReplicaId: f.replicaID,
		FromLsn:   fromLSN,
		Epoch:     f.epoch(),
	})
	if err != nil {
		return err
	}

	var count int

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				f.sendAck(ctx, false) // flush final ACK on clean shutdown
				return nil
			}
			return err
		}

		if err := f.checkEpoch(msg.Epoch); err != nil {
			return err
		}
		f.noteLeader(msg.Lsn)

		f.mu.Lock()
		expectedLSN := f.lastAppliedLSN + 1
		if msg.Lsn > expectedLSN {
			f.mu.Unlock()
			return fmt.Errorf("%w: expected %d, got %d", ErrReplicationGap, expectedLSN, msg.Lsn)
		}
		if msg.Lsn <= f.lastAppliedLSN {
			f.mu.Unlock()
			continue
		}
		f.mu.Unlock()

		if err := f.engine.ApplyReplicatedRecord(byte(msg.OpType), msg.Key, msg.Value); err != nil {
			return err
		}

		f.mu.Lock()
		f.lastAppliedLSN = msg.Lsn
		f.mu.Unlock()

		count++
		if count >= ackBatchSize {
			f.sendAck(ctx, false)
			count = 0
		}
	}
}

// ackTicker acknowledges applied-but-unacknowledged records every
// ackBatchInterval until ctx is cancelled.
func (f *FollowerClient) ackTicker(ctx context.Context) {
	t := time.NewTicker(ackBatchInterval)
	defer t.Stop()
	lastBeat := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			beat := now.Sub(lastBeat) >= heartbeatEvery
			if beat {
				lastBeat = now
			}
			f.sendAck(ctx, beat)
		}
	}
}

// sendAck fires a single ACK for the current lastAppliedLSN, unless that LSN
// is already acknowledged and force is false. A forced ACK is a heartbeat: the
// reply tells an idle follower the leader's LSN, so it can tell it is caught up.
func (f *FollowerClient) sendAck(ctx context.Context, force bool) {
	f.ackMu.Lock()
	defer f.ackMu.Unlock()
	f.mu.RLock()
	lsn := f.lastAppliedLSN
	f.mu.RUnlock()
	if lsn == f.lastAckedLSN && !force {
		return
	}
	resp, err := f.client.Ack(ctx, &beastv1.ReplicationAck{
		ReplicaId:       f.replicaID,
		AcknowledgedLsn: lsn,
		Epoch:           f.epoch(),
	})
	if err != nil {
		return
	}
	f.lastAckedLSN = lsn
	if f.checkEpoch(resp.Epoch) == nil {
		f.noteLeader(resp.LeaderLsn)
	}
}

// SetStateStore makes the follower track and persist leader epochs.
func (f *FollowerClient) SetStateStore(st *StateStore) { f.state = st }

func (f *FollowerClient) epoch() uint64 {
	if f.state == nil {
		return 0
	}
	return f.state.Epoch()
}

// checkEpoch rejects a leader older than one already seen and adopts a newer
// one (persisting it before any of that leader's records are applied).
func (f *FollowerClient) checkEpoch(leaderEpoch uint64) error {
	if f.state == nil {
		return nil
	}
	cur := f.state.Get()
	if leaderEpoch < cur.Epoch {
		return fmt.Errorf("%w: leader at epoch %d, follower at %d", ErrStaleLeader, leaderEpoch, cur.Epoch)
	}
	if leaderEpoch > cur.Epoch {
		next := cur
		next.Epoch = leaderEpoch
		next.Role = RoleFollower
		return f.state.Set(next)
	}
	return nil
}

// noteLeader records contact with a current-epoch leader at the given LSN.
func (f *FollowerClient) noteLeader(lsn uint64) {
	f.mu.Lock()
	if lsn > f.leaderLSN {
		f.leaderLSN = lsn
	}
	f.lastContact = time.Now()
	f.mu.Unlock()
}

// LeaderLSN returns the highest leader LSN this follower has heard of.
func (f *FollowerClient) LeaderLSN() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.leaderLSN
}

// LastContact is when a current-epoch leader last sent a record or answered
// an ACK; the zero time means never.
func (f *FollowerClient) LastContact() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastContact
}

// LastAppliedLSN returns the latest Log Sequence Number applied by this replica.
func (f *FollowerClient) LastAppliedLSN() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastAppliedLSN
}
