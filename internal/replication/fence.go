package replication

import (
	"context"
	"errors"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SetActive marks whether this node serves its WAL. A follower that has never
// led is inactive; a leader, and a fenced former leader (which may still be
// the source a candidate catches up from), are active.
func (s *LeaderServer) SetActive(active bool) { s.inactive.Store(!active) }

// servedEpoch is the epoch stamped on records and ACK replies.
func (s *LeaderServer) servedEpoch() uint64 {
	if s.cluster == nil {
		return 0
	}
	return s.cluster.ServedEpoch()
}

// admit gates every replication RPC. It refuses when this node is not serving
// a WAL and, when the peer has seen a higher epoch than ours, steps this node
// down first: a higher epoch proves a newer leader exists.
func (s *LeaderServer) admit(peerEpoch uint64, peerID string) error {
	if s.inactive.Load() {
		return status.Error(codes.FailedPrecondition, "this node is not a leader")
	}
	if s.cluster == nil {
		return nil
	}
	if peerEpoch > s.cluster.Epoch() {
		if err := s.cluster.stepDown(peerEpoch, peerID); err != nil {
			return status.Errorf(codes.Internal, "could not step down: %v", err)
		}
		return status.Errorf(codes.FailedPrecondition, "stale leader: peer is at epoch %d, this node stepped down", peerEpoch)
	}
	return nil
}

// Fence freezes this leader's writes at the candidate's epoch and returns the
// final LSN. It is idempotent for the same candidate and epoch.
func (s *LeaderServer) Fence(ctx context.Context, req *beastv1.FenceRequest) (*beastv1.FenceResponse, error) {
	if s.inactive.Load() || s.cluster == nil {
		return nil, status.Error(codes.FailedPrecondition, "this node is not a leader")
	}
	lsn, err := s.cluster.fence(req.Epoch, req.CandidateId)
	switch {
	case err == nil:
		return &beastv1.FenceResponse{FinalLsn: lsn, Epoch: req.Epoch}, nil
	case errors.Is(err, ErrFenceRefused):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	default:
		return nil, status.Error(codes.Internal, err.Error())
	}
}
