package handler

import (
	"net/http"
	"time"
)

var serverStartTime = time.Now()

// HealthzResponse is the structured payload for liveness probes.
type HealthzResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// ReadyzResponse is the structured payload for readiness probes.
type ReadyzResponse struct {
	Status     string `json:"status"`
	Role       string `json:"role"`
	LSN        uint64 `json:"lsn"`
	TotalPages uint64 `json:"total_pages"`
}

// HealthzHandler returns 200 OK as long as the HTTP listener is alive.
func HealthzHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := int64(time.Since(serverStartTime).Seconds())
		writeJSON(w, HealthzResponse{
			Status:        "ok",
			Version:       version,
			UptimeSeconds: uptime,
		})
	}
}

// ReadyzHandler returns 200 OK if the engine is responsive, or 503 if unready.
func ReadyzHandler(engine EngineReader, role string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if engine == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			writeJSON(w, map[string]string{"status": "not ready", "error": "engine uninitialized"})
			return
		}

		metrics := engine.StorageMetrics()
		lsn := engine.CurrentLSN()

		writeJSON(w, ReadyzResponse{
			Status:     "ready",
			Role:       role,
			LSN:        lsn,
			TotalPages: metrics.TotalPages,
		})
	}
}
