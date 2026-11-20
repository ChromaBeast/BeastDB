package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/replication"
)

type rejoinRequest struct {
	// LeaderAddr is the gRPC address (host:port) of the current leader.
	LeaderAddr string `json:"leaderAddr"`
}

// PostRejoin handles POST /api/replication/rejoin (admin-only via middleware).
// It turns this fenced node into a follower of the named leader when its log is
// provably a prefix of the leader's, and refuses, saying what the node holds
// that the leader lacks, when it is not. Every attempt is audited.
func (h *APIHandler) PostRejoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.cluster == nil {
		http.Error(w, "rejoin is not available on this node", http.StatusNotImplemented)
		return
	}
	var req rejoinRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	req.LeaderAddr = strings.TrimSpace(req.LeaderAddr)
	if req.LeaderAddr == "" || len(req.LeaderAddr) > 255 || strings.ContainsAny(req.LeaderAddr, " /\\?#@") {
		http.Error(w, "leaderAddr must be a host:port gRPC address", http.StatusBadRequest)
		return
	}
	node := h.cluster.Status().NodeID
	res, err := h.cluster.Rejoin(r.Context(), req.LeaderAddr)
	if err != nil {
		h.record(r, audit.ActionRejoin, node, fmt.Sprintf("refused: leader %s: %v", req.LeaderAddr, err))
		writeRejoinError(w, err)
		return
	}
	h.record(r, audit.ActionRejoin, node, fmt.Sprintf("rejoined as a follower of %s at epoch %d; log proven to be a prefix of the leader's (applied LSN %d, leader LSN %d)", res.LeaderAddr, res.Epoch, res.AppliedLSN, res.LeaderLSN))
	writeJSON(w, map[string]any{"ok": true, "result": res, "cluster": h.cluster.Status()})
}

func writeRejoinError(w http.ResponseWriter, err error) {
	code, status := "failed", http.StatusInternalServerError
	body := map[string]any{"ok": false}
	var refusal *replication.RejoinRefusal
	switch {
	case errors.Is(err, replication.ErrNotFenced):
		code, status = "not_fenced", http.StatusConflict
	case errors.Is(err, replication.ErrRejoinUnavailable):
		code, status = "unavailable", http.StatusNotImplemented
	case errors.As(err, &refusal):
		code, status = "rejoin_refused", http.StatusConflict
		body["unreplicatedRecords"] = refusal.UnreplicatedRecords
	}
	body["code"], body["error"] = code, err.Error()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
