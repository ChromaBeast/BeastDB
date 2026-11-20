package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/replication"
)

type promoteRequest struct {
	// AcceptDataLoss permits promotion when the old leader cannot be reached
	// to fence it. Without it an unreachable leader blocks promotion.
	AcceptDataLoss bool `json:"acceptDataLoss"`
}

// PostPromote handles POST /api/replication/promote (admin-only via middleware).
// It promotes this follower to leader at the next epoch. Every attempt, refused
// or not, is written to the audit log.
func (h *APIHandler) PostPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cluster == nil {
		http.Error(w, "promotion is not available on this node", http.StatusNotImplemented)
		return
	}
	var req promoteRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
	}
	node := h.cluster.Status().NodeID
	res, err := h.cluster.Promote(r.Context(), replication.PromoteOptions{AcceptDataLoss: req.AcceptDataLoss})
	if err != nil {
		h.record(r, audit.ActionPromote, node, fmt.Sprintf("refused: %v (acceptDataLoss=%t)", err, req.AcceptDataLoss))
		writePromoteError(w, err)
		return
	}
	if res.OldLeaderFenced {
		h.record(r, audit.ActionPromote, node, fmt.Sprintf("promoted to leader at epoch %d; old leader fenced at LSN %d, applied %d", res.Epoch, res.FinalLSN, res.AppliedLSN))
	} else {
		h.record(r, audit.ActionPromote, node, fmt.Sprintf("promoted to leader at epoch %d WITHOUT fencing the old leader (acceptDataLoss); data loss possible, old leader LSN known %d, applied %d", res.Epoch, res.LeaderLSNKnown, res.AppliedLSN))
	}
	writeJSON(w, map[string]any{"ok": true, "result": res, "cluster": h.cluster.Status()})
}

func writePromoteError(w http.ResponseWriter, err error) {
	code, status := "failed", http.StatusInternalServerError
	switch {
	case errors.Is(err, replication.ErrNotFollower):
		code, status = "not_follower", http.StatusConflict
	case errors.Is(err, replication.ErrPromoteInProgress):
		code, status = "in_progress", http.StatusConflict
	case errors.Is(err, replication.ErrLeaderUnreachable):
		code, status = "leader_unreachable", http.StatusConflict
	case errors.Is(err, replication.ErrCatchUpTimeout):
		code, status = "catch_up_timeout", http.StatusConflict
	case errors.Is(err, replication.ErrFenceRefused):
		code, status = "fence_refused", http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "code": code, "error": err.Error()})
}
