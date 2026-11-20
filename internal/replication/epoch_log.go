package replication

import (
	"context"
	"fmt"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/wal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrDivergedHistory means a node's log and the leader's disagree about which
// epoch wrote the record at the node's last LSN: the node holds writes the
// leader does not (or the leader's history is not the one the node followed).
var ErrDivergedHistory = fmt.Errorf("replication: log history diverged")

// logEpoch is the epoch of this follower's last logged record, per the epoch
// markers in its WAL; 0 when the log predates markers or has no state store.
func (f *FollowerClient) logEpoch() uint64 {
	if f.state == nil {
		return 0
	}
	f.mu.RLock()
	lsn := f.lastAppliedLSN
	f.mu.RUnlock()
	return f.state.Get().EpochAt(lsn)
}

// checkFrame validates a frame against this follower's own WAL, independent of
// the epoch the RPC layer reports: a frame from an epoch below the epoch the
// log has already reached is a stale leader's, whatever the stream claims, and
// an epoch marker may not claim an epoch above its own frame's.
func (f *FollowerClient) checkFrame(msg *beastv1.WALRecordMessage) error {
	if f.state == nil {
		return nil
	}
	have := f.logEpoch()
	if msg.Epoch < have {
		return fmt.Errorf("%w: frame at LSN %d is from epoch %d, this log is already at epoch %d", ErrStaleLeader, msg.Lsn, msg.Epoch, have)
	}
	if byte(msg.OpType) == wal.OpEpoch {
		e, ok := wal.DecodeEpoch(msg.Value)
		switch {
		case !ok:
			return wal.ErrBadEpochMarker
		case e < have:
			return fmt.Errorf("%w: epoch marker %d at LSN %d is below this log's epoch %d", ErrStaleLeader, e, msg.Lsn, have)
		case e > msg.Epoch:
			return fmt.Errorf("replication: epoch marker %d at LSN %d is above the epoch %d the leader stamped on it", e, msg.Lsn, msg.Epoch)
		}
	}
	return nil
}

// noteMarker records an epoch marker that was just applied in the persisted
// history. A crash before this runs is repaired from the WAL at startup.
func (f *FollowerClient) noteMarker(msg *beastv1.WALRecordMessage) error {
	if f.state == nil || byte(msg.OpType) != wal.OpEpoch {
		return nil
	}
	e, _ := wal.DecodeEpoch(msg.Value)
	return f.state.MergeEpochMarks([]wal.EpochMark{{LSN: msg.Lsn, Epoch: e}})
}

// checkLogMatch refuses a follower whose last record was written at an epoch
// this leader's log does not show at that LSN. Both sides derive that epoch
// from OpEpoch markers; a follower with no epoch info (an old log) is not
// checked.
func (s *LeaderServer) checkLogMatch(req *beastv1.StreamWALRequest) error {
	if s.cluster == nil || req.FromLsn <= 1 || req.LastLogEpoch == 0 {
		return nil
	}
	last := req.FromLsn - 1
	if mine := s.cluster.state.Get().EpochAt(last); mine != req.LastLogEpoch {
		return status.Errorf(codes.FailedPrecondition,
			"%v: follower's record at LSN %d was written at epoch %d, this leader's log has epoch %d there; the follower holds writes this leader never had",
			ErrDivergedHistory, last, req.LastLogEpoch, mine)
	}
	return nil
}

// LogEpochs reports this node's epoch history and last LSN so a fenced node
// can prove its log is a prefix of the leader's before rejoining.
func (s *LeaderServer) LogEpochs(ctx context.Context, req *beastv1.LogEpochsRequest) (*beastv1.LogEpochsResponse, error) {
	if err := s.admit(req.Epoch, req.ReplicaId); err != nil {
		return nil, err
	}
	if s.cluster == nil {
		return nil, status.Error(codes.FailedPrecondition, "this node does not track epochs")
	}
	st := s.cluster.state.Get()
	resp := &beastv1.LogEpochsResponse{Epoch: st.Epoch, LastLsn: s.cluster.engine.CurrentLSN(), Role: string(st.Role)}
	for _, m := range st.EpochStarts {
		resp.EpochStarts = append(resp.EpochStarts, &beastv1.EpochStart{Lsn: m.LSN, Epoch: m.Epoch})
	}
	return resp, nil
}
