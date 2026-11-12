package handler

import (
	"bytes"
	"crypto/subtle"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ChromaBeast/beastdb/internal/metrics"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
)

// MetricsConfig configures the /metrics endpoint.
type MetricsConfig struct {
	Role    string
	Version string
	// DataDir is used for the disk-free gauge (omitted when empty).
	DataDir string
	// Tokens are static bearer tokens accepted for scraping (e.g. the gRPC
	// --api-token and BEASTDB_METRICS_TOKEN). Empty entries are ignored.
	Tokens []string
	// Sessions authenticates admin browser sessions. May be nil.
	Sessions *auth.SessionManager
}

var processStart = time.Now()

// authorized reports whether r carries an admin session or a configured
// bearer token. There is deliberately no unauthenticated/loopback bypass: a
// reverse proxy on the same host would make every remote peer look like
// loopback.
func (c MetricsConfig) authorized(r *http.Request) bool {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		got := []byte(strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")))
		for _, t := range c.Tokens {
			t = strings.TrimSpace(t)
			if t != "" && subtle.ConstantTimeCompare(got, []byte(t)) == 1 {
				return true
			}
		}
		return false
	}
	if c.Sessions != nil {
		if _, role, err := c.Sessions.ValidateSession(r); err == nil && role == auth.RoleAdmin {
			return true
		}
	}
	return false
}

// MetricsHandler serves Prometheus text exposition (version 0.0.4).
func MetricsHandler(engine EngineReader, cfg MetricsConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if !cfg.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="beastdb-metrics"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		var buf bytes.Buffer
		WriteMetrics(&buf, engine, cfg)
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
	})
}

// WriteMetrics renders all metrics.
func WriteMetrics(buf *bytes.Buffer, engine EngineReader, cfg MetricsConfig) {
	w := metrics.NewWriter(buf)
	w.Gauge("beastdb_build_info", "Build information; value is always 1.", 1,
		"version", cfg.Version, "role", cfg.Role)
	w.Gauge("beastdb_process_start_time_seconds", "Process start time, unix seconds.", float64(processStart.Unix()))
	w.Gauge("beastdb_uptime_seconds", "Seconds since process start.", time.Since(processStart).Seconds())

	if engine != nil {
		m := engine.StorageMetrics()
		w.Gauge("beastdb_wal_size_bytes", "Current WAL file size in bytes.", float64(m.WALSizeBytes))
		w.Gauge("beastdb_wal_current_lsn", "Highest LSN written to the WAL.", float64(m.CurrentLSN))
		w.Gauge("beastdb_checkpoint_last_lsn", "LSN of the last checkpoint completed by this process (0 if none).", float64(m.LastCheckpointLSN))
		if m.LastCheckpointUnix > 0 {
			w.Gauge("beastdb_checkpoint_last_timestamp_seconds", "Unix time of the last checkpoint.", float64(m.LastCheckpointUnix))
			age := time.Since(time.Unix(m.LastCheckpointUnix, 0)).Seconds()
			if age < 0 {
				age = 0
			}
			w.Gauge("beastdb_checkpoint_age_seconds", "Seconds since the last checkpoint.", age)
		}
		w.Gauge("beastdb_buffer_pool_size_pages", "Buffer pool capacity in pages.", float64(m.PoolSize))
		w.Gauge("beastdb_buffer_pool_cached_pages", "Pages resident in the buffer pool.", float64(m.CachedPages))
		w.Gauge("beastdb_buffer_pool_pinned_pages", "Pinned buffer pool pages.", float64(m.PinnedPages))
		w.Gauge("beastdb_buffer_pool_dirty_pages", "Dirty buffer pool pages.", float64(m.DirtyPages))
		w.Counter("beastdb_buffer_pool_hits_total", "Buffer pool fetches served from memory.", float64(m.BufferHits))
		w.Counter("beastdb_buffer_pool_misses_total", "Buffer pool fetches that read from disk.", float64(m.BufferMisses))
		if total := m.BufferHits + m.BufferMisses; total > 0 {
			w.Gauge("beastdb_buffer_pool_hit_ratio", "Lifetime buffer pool hit ratio (0..1).", float64(m.BufferHits)/float64(total))
		}
		w.Gauge("beastdb_data_file_size_bytes", "Data file size in bytes.", float64(m.DiskSizeBytes))
		w.Gauge("beastdb_data_pages", "Total pages in the data file.", float64(m.TotalPages))
		ro := 0.0
		if m.ReadOnly {
			ro = 1
		}
		w.Gauge("beastdb_read_only", "1 if the engine rejects writes (follower or fail-stop).", ro)
		if cfg.Role != "leader" {
			w.Gauge("beastdb_replica_applied_lsn",
				"Last LSN applied by this follower. Lag = leader beastdb_wal_current_lsn minus this.", float64(engine.CurrentLSN()))
		}
	}
	if cfg.DataDir != "" {
		if free, ok := metrics.DiskFreeBytes(filepath.Clean(cfg.DataDir)); ok {
			w.Gauge("beastdb_disk_free_bytes", "Free bytes on the data directory filesystem.", float64(free))
		}
	}
	w.Gauge("beastdb_grpc_requests_in_flight", "gRPC requests currently executing.", float64(metrics.InFlight.Load()))
	metrics.Default.WriteRequests(w)
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(c int) {
	if s.code == 0 {
		s.code = c
	}
	s.ResponseWriter.WriteHeader(c)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// HTTPMetrics records request count, status and latency for every HTTP
// request. The route label is the matched mux pattern (bounded cardinality).
func HTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		code := rec.code
		if code == 0 {
			code = http.StatusOK
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		metrics.Default.Observe("http", route, strconv.Itoa(code), time.Since(start))
	})
}
