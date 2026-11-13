package handler

import (
	"net/http"

	"github.com/ChromaBeast/beastdb/internal/replication"
)

// Checkpointer is implemented by engines that can flush dirty pages and rotate
// the WAL on demand. It is optional so read-only EngineReader fakes stay valid.
type Checkpointer interface {
	Checkpoint() error
}

// ReplicationSource exposes the leader's view of its replicas.
type ReplicationSource interface {
	Replicas() []replication.ReplicaInfo
}

// ReplicaStatus is one replica row of the /api/replication response.
type ReplicaStatus struct {
	replication.ReplicaInfo
	// LagLSN is leader LSN minus the replica's acked LSN (never negative).
	LagLSN uint64 `json:"lagLSN"`
}

// ReplicationStatus is the /api/replication response body.
type ReplicationStatus struct {
	Role string `json:"role"`
	LSN  uint64 `json:"lsn"`
	// Tracked is true only on a leader, the node that sees its replicas.
	Tracked  bool            `json:"tracked"`
	Replicas []ReplicaStatus `json:"replicas"`
}

// SetReplicationSource wires the leader's replica tracker (nil on followers).
func (h *APIHandler) SetReplicationSource(src ReplicationSource) {
	h.replication = src
}

// PostCheckpoint handles POST /api/checkpoint (admin-only via middleware).
// It reports the real outcome of the engine checkpoint.
func (h *APIHandler) PostCheckpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	cp, ok := h.engine.(Checkpointer)
	if !ok {
		http.Error(w, "checkpoint not supported by this engine", http.StatusNotImplemented)
		return
	}
	if err := cp.Checkpoint(); err != nil {
		http.Error(w, "checkpoint failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	m := h.engine.StorageMetrics()
	writeJSON(w, map[string]any{
		"ok":                 true,
		"lsn":                m.CurrentLSN,
		"lastCheckpointLSN":  m.LastCheckpointLSN,
		"lastCheckpointUnix": m.LastCheckpointUnix,
		"walSizeBytes":       m.WALSizeBytes,
		"dirtyPages":         m.DirtyPages,
	})
}

// GetReplication handles GET /api/replication (admin-only via middleware).
func (h *APIHandler) GetReplication(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	lsn := h.engine.CurrentLSN()
	resp := ReplicationStatus{Role: h.role, LSN: lsn, Replicas: []ReplicaStatus{}}
	if h.replication != nil {
		resp.Tracked = true
		for _, rep := range h.replication.Replicas() {
			var lag uint64
			if lsn > rep.AckedLSN {
				lag = lsn - rep.AckedLSN
			}
			resp.Replicas = append(resp.Replicas, ReplicaStatus{ReplicaInfo: rep, LagLSN: lag})
		}
	}
	writeJSON(w, resp)
}
