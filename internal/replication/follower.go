package replication

import (
	"context"
	"io"
	"sync"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
)

// FollowerClient connects to the Leader node and replicates WAL records into local state.
type FollowerClient struct {
	replicaID      string
	engine         *api.Engine
	client         beastv1.ReplicationServiceClient
	lastAppliedLSN uint64
	mu             sync.RWMutex
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
func (f *FollowerClient) SyncLoop(ctx context.Context) error {
	f.mu.Lock()
	fromLSN := f.lastAppliedLSN + 1
	f.mu.Unlock()

	stream, err := f.client.StreamWAL(ctx, &beastv1.StreamWALRequest{
		ReplicaId: f.replicaID,
		FromLsn:   fromLSN,
	})
	if err != nil {
		return err
	}

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return nil
			}
			return err
		}

		// Apply record locally into follower's storage engine and B+ Tree
		if err := f.engine.ApplyReplicatedRecord(byte(msg.OpType), msg.Key, msg.Value); err != nil {
			return err
		}

		f.mu.Lock()
		if msg.Lsn > f.lastAppliedLSN {
			f.lastAppliedLSN = msg.Lsn
		}
		currentAck := f.lastAppliedLSN
		f.mu.Unlock()

		// Acknowledge progress back to leader
		_, _ = f.client.Ack(ctx, &beastv1.ReplicationAck{
			ReplicaId:       f.replicaID,
			AcknowledgedLsn: currentAck,
		})
	}
}

// LastAppliedLSN returns the latest Log Sequence Number applied by this replica.
func (f *FollowerClient) LastAppliedLSN() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastAppliedLSN
}
