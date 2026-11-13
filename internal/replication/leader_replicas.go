package replication

import (
	"context"
	"sort"

	"google.golang.org/grpc/peer"
)

// ReplicaInfo is a read-only snapshot of one replica as seen by the leader.
type ReplicaInfo struct {
	ID string `json:"id"`
	// Address is the remote address of the replica's live stream; empty when
	// the replica is not currently connected or the transport hides it.
	Address string `json:"address,omitempty"`
	// Connected is true while the replica holds an open WAL stream.
	Connected bool `json:"connected"`
	// AckedLSN is the highest LSN the replica has acknowledged (0 if none yet).
	AckedLSN uint64 `json:"ackedLSN"`
}

// trackAddr remembers the remote address of a replica's stream for Replicas.
func (s *LeaderServer) trackAddr(ctx context.Context, replicaID string) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return
	}
	s.mu.Lock()
	s.addrs[replicaID] = p.Addr.String()
	s.mu.Unlock()
}

// Replicas returns every replica that is connected or has acknowledged an LSN
// since this process started, sorted by id. Safe for concurrent use.
func (s *LeaderServer) Replicas() []ReplicaInfo {
	connected := make(map[string]bool)
	for _, id := range s.broadcaster.SubscriberIDs() {
		connected[id] = true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make(map[string]struct{}, len(connected)+len(s.acks))
	for id := range connected {
		ids[id] = struct{}{}
	}
	for id := range s.acks {
		ids[id] = struct{}{}
	}
	out := make([]ReplicaInfo, 0, len(ids))
	for id := range ids {
		info := ReplicaInfo{ID: id, Connected: connected[id], AckedLSN: s.acks[id]}
		if info.Connected {
			info.Address = s.addrs[id]
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
