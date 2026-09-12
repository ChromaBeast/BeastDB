package replication

import (
	"context"
	"sync"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/wal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// LeaderServer implements the ReplicationService gRPC interface on the primary node.
type LeaderServer struct {
	beastv1.UnimplementedReplicationServiceServer
	walPath     string
	broadcaster *Broadcaster
	acks        map[string]uint64
	mu          sync.RWMutex
}

// NewLeaderServer creates a replication server tracking connected replicas and log history.
func NewLeaderServer(walPath string) *LeaderServer {
	return &LeaderServer{
		walPath:     walPath,
		broadcaster: NewBroadcaster(),
		acks:        make(map[string]uint64),
	}
}

// RegisterService registers the replication handler with the gRPC server instance.
func (s *LeaderServer) RegisterService(grpcServer *grpc.Server) {
	beastv1.RegisterReplicationServiceServer(grpcServer, s)
}

// Broadcast broadcasts a newly committed WAL record to all active streaming followers.
func (s *LeaderServer) Broadcast(lsn uint64, opType byte, key, value []byte) {
	s.broadcaster.Broadcast(&beastv1.WALRecordMessage{
		Lsn:    lsn,
		OpType: uint32(opType),
		Key:    key,
		Value:  value,
	})
}

// StreamWAL streams historical WAL replay followed by real-time committed WAL frames.
func (s *LeaderServer) StreamWAL(req *beastv1.StreamWALRequest, stream grpc.ServerStreamingServer[beastv1.WALRecordMessage]) error {
	var maxReplayedLSN uint64

	// 1. Catch-up Phase: Replay historical records from local WAL file
	_, err := wal.Replay(s.walPath, func(rec *wal.Record) error {
		if rec.LSN >= req.FromLsn {
			msg := &beastv1.WALRecordMessage{
				Lsn:    rec.LSN,
				OpType: uint32(rec.Type),
				Key:    rec.Key,
				Value:  rec.Value,
			}
			if err := stream.Send(msg); err != nil {
				return err
			}
			maxReplayedLSN = rec.LSN
		}
		return nil
	})
	if err != nil {
		return status.Errorf(codes.Internal, "wal replay error: %v", err)
	}

	// 2. Real-time Phase: Subscribe to live committed records
	ch, unsubscribe := s.broadcaster.Subscribe(req.ReplicaId, 128)
	defer unsubscribe()

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if msg.Lsn > maxReplayedLSN {
				if err := stream.Send(msg); err != nil {
					return err
				}
			}
		}
	}
}

// Ack records the latest applied LSN acknowledgment from a replica node.
func (s *LeaderServer) Ack(ctx context.Context, req *beastv1.ReplicationAck) (*beastv1.AckResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.acks[req.ReplicaId] = req.AcknowledgedLsn
	return &beastv1.AckResponse{Success: true}, nil
}

// GetReplicaAck returns the highest acknowledged LSN for a given replica.
func (s *LeaderServer) GetReplicaAck(replicaID string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.acks[replicaID]
}
