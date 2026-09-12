package replication

import (
	"context"
	"io"
	"sync"
	"time"

	beastv1 "github.com/ChromaBeast/beastdb/api/proto"
	"github.com/ChromaBeast/beastdb/internal/api"
	"google.golang.org/grpc"
)

const (
	ackBatchSize     = 100              // send ACK every N records
	ackBatchInterval = 100 * time.Millisecond // or every 100 ms, whichever first
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
// Fix #4: ACKs are batched — sent every ackBatchSize records OR every ackBatchInterval ms,
// whichever comes first. This reduces ACK round-trips from O(writes/sec) to O(writes/100).
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

	var count int
	lastAck := time.Now()

	for {
		msg, err := stream.Recv()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				f.sendAck(ctx) // flush final ACK on clean shutdown
				return nil
			}
			return err
		}

		if err := f.engine.ApplyReplicatedRecord(byte(msg.OpType), msg.Key, msg.Value); err != nil {
			return err
		}

		f.mu.Lock()
		if msg.Lsn > f.lastAppliedLSN {
			f.lastAppliedLSN = msg.Lsn
		}
		f.mu.Unlock()

		count++
		if count >= ackBatchSize || time.Since(lastAck) >= ackBatchInterval {
			f.sendAck(ctx)
			count = 0
			lastAck = time.Now()
		}
	}
}

// sendAck fires a single ACK for the current lastAppliedLSN.
func (f *FollowerClient) sendAck(ctx context.Context) {
	f.mu.RLock()
	lsn := f.lastAppliedLSN
	f.mu.RUnlock()
	_, _ = f.client.Ack(ctx, &beastv1.ReplicationAck{
		ReplicaId:       f.replicaID,
		AcknowledgedLsn: lsn,
	})
}

// LastAppliedLSN returns the latest Log Sequence Number applied by this replica.
func (f *FollowerClient) LastAppliedLSN() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.lastAppliedLSN
}
