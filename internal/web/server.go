package web

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/ChromaBeast/beastdb/internal/audit"
	"github.com/ChromaBeast/beastdb/internal/backup"
	"github.com/ChromaBeast/beastdb/internal/logbuf"
	"github.com/ChromaBeast/beastdb/internal/metrics"
	"github.com/ChromaBeast/beastdb/internal/runtimecfg"
	"github.com/ChromaBeast/beastdb/internal/web/auth"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
)

//go:embed all:static
var staticFiles embed.FS

// Server is the embedded HTTP admin server running alongside the gRPC server.
type Server struct {
	httpServer  *http.Server
	stopSampler func()
}

// EngineBackend is the combined interface required by the web server.
type EngineBackend interface {
	handler.EngineReader
	auth.EngineWriter
}

// NewServer constructs and wires all HTTP routes.
// secret is the 32-byte HMAC key for session signing.
// partitions is the optional project-defined partition registry served to the Studio.
// Call Serve() to start accepting connections.
func NewServer(addr string, engine EngineBackend, secret []byte, role, version string, partitions []handler.PartitionEntry, tokenStore ...*auth.TokenStore) (*Server, error) {
	return NewServerWithOptions(addr, engine, secret, role, version, partitions, Options{}, tokenStore...)
}

// Options carries optional settings for NewServerWithOptions.
type Options struct {
	// DataDir enables the disk-free metric.
	DataDir string
	// MetricsTokens are bearer tokens accepted by /metrics in addition to
	// admin sessions. With none configured only admin sessions may scrape.
	MetricsTokens []string
	// Settings is the non-secret runtime configuration served by GET /api/settings.
	Settings handler.RuntimeSettings
	// Logs backs GET /api/logs; nil serves an empty log.
	Logs *logbuf.Buffer
	// Replication supplies the leader's replica view for /api/replication (nil on followers).
	Replication handler.ReplicationSource
	// Cluster backs the epoch/role fields of /api/replication and
	// POST /api/replication/promote; nil disables promotion.
	Cluster handler.ClusterControl
	// Audit records admin actions and backs GET /api/audit; nil disables both.
	Audit *audit.Log
	// Runtime holds the settings an admin may change live and backs
	// /api/settings/runtime; nil makes that endpoint answer 404.
	Runtime *runtimecfg.Store
	// Backups backs /api/backups; nil makes those endpoints answer 404.
	Backups *backup.Manager
}

// NewServerWithOptions is NewServer plus optional settings.
func NewServerWithOptions(addr string, engine EngineBackend, secret []byte, role, version string, partitions []handler.PartitionEntry, opts Options, tokenStore ...*auth.TokenStore) (*Server, error) {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}

	sessions := auth.NewSessionManager(secret)
	users := auth.NewUserStore(engine)
	apiH := handler.NewAPIHandler(engine, sessions, role, version, partitions)
	if len(tokenStore) > 0 && tokenStore[0] != nil {
		apiH.SetTokenStore(tokenStore[0])
	}
	apiH.SetReplicationSource(opts.Replication)
	apiH.SetCluster(opts.Cluster)
	apiH.SetAudit(opts.Audit)
	rateLimiter := handler.NewLoginRateLimiter(5, 30*time.Second)
	if opts.Runtime != nil {
		opts.Runtime.Subscribe(func(v runtimecfg.Values) {
			rateLimiter.Configure(v.LoginMaxAttempts, time.Duration(v.LoginWindowSeconds)*time.Second)
		})
	}

	deps := &handler.Deps{
		Users:       users,
		Sessions:    sessions,
		StaticFS:    staticFS,
		RateLimiter: rateLimiter,
		Audit:       opts.Audit,
	}

	mux := http.NewServeMux()

	// Public static assets (CSS, JS, Next.js artifacts)
	mux.Handle("/css/", http.FileServer(http.FS(staticFS)))
	mux.Handle("/js/", http.FileServer(http.FS(staticFS)))
	mux.Handle("/brand/", http.FileServer(http.FS(staticFS)))
	mux.Handle("/icon.svg", http.FileServer(http.FS(staticFS)))
	mux.Handle("/_next/", http.FileServer(http.FS(staticFS)))

	// Public operational health routes
	mux.HandleFunc("/healthz", handler.HealthzHandler(version))
	mux.HandleFunc("/readyz", handler.ReadyzHandler(engine, role))

	// Prometheus metrics: admin session or bearer token required (never anonymous).
	mux.Handle("/metrics", handler.MetricsHandler(engine, handler.MetricsConfig{
		Role: role, Version: version, DataDir: opts.DataDir,
		Tokens: opts.MetricsTokens, Sessions: sessions,
	}))

	// Public auth routes
	mux.HandleFunc("/login", handler.LoginHandler(deps))
	mux.HandleFunc("/logout", handler.LogoutHandler(deps))

	// Protected dashboard — serves index.html
	dashboardFS := http.FileServer(http.FS(staticFS))
	mux.Handle("/", handler.AuthMiddleware(sessions, dashboardFS))

	// Protected telemetry and record exploration routes
	mux.Handle("/api/stats", handler.AuthMiddleware(sessions, http.HandlerFunc(apiH.GetStats)))
	mux.Handle("/api/me", handler.AuthMiddleware(sessions, http.HandlerFunc(apiH.GetMe)))
	mux.Handle("/api/records", handler.AuthMiddleware(sessions, http.HandlerFunc(apiH.GetRecords)))
	mux.Handle("/api/partitions", handler.AuthMiddleware(sessions, http.HandlerFunc(apiH.GetPartitions)))

	// Protected key-value CRUD routes
	mux.Handle("/api/key", handler.AuthMiddleware(sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiH.GetKey(w, r)
		case http.MethodPost:
			apiH.PutKey(w, r)
		case http.MethodDelete:
			apiH.DeleteKey(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})))

	// Search route — authenticated users only
	mux.Handle("/api/search", handler.AuthMiddleware(sessions, http.HandlerFunc(apiH.SearchRecords)))

	// User bulk-delete route — admin only
	mux.Handle("/api/user", handler.AdminMiddleware(sessions, http.HandlerFunc(apiH.DeleteUser)))

	// API token management — admin only
	mux.Handle("/api/tokens", handler.AdminMiddleware(sessions, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			apiH.GetTokens(w, r)
		case http.MethodPost:
			apiH.CreateToken(w, r)
		case http.MethodDelete:
			apiH.DeleteToken(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})))

	// Logs tail and read-only runtime settings — admin only (handlers re-check the session)
	mux.Handle("/api/logs", handler.AdminMiddleware(sessions, handler.LogsHandler(sessions, opts.Logs)))
	opts.Settings.Version = version
	opts.Settings.Role = role
	mux.Handle("/api/settings", handler.AdminMiddleware(sessions, handler.SettingsHandler(sessions, opts.Settings)))
	// Operations (checkpoint, replica view) — admin only
	mux.Handle("/api/checkpoint", handler.AdminMiddleware(sessions, http.HandlerFunc(apiH.PostCheckpoint)))
	mux.Handle("/api/replication", handler.AdminMiddleware(sessions, http.HandlerFunc(apiH.GetReplication)))
	mux.Handle("/api/replication/promote", handler.AdminMiddleware(sessions, http.HandlerFunc(apiH.PostPromote)))

	mux.Handle("/api/settings/runtime", handler.AdminMiddleware(sessions, handler.RuntimeSettingsHandler(sessions, opts.Runtime, opts.Audit)))
	backupsH := handler.AdminMiddleware(sessions, handler.BackupsHandler(sessions, opts.Backups, opts.Audit))
	mux.Handle("/api/backups", backupsH)
	mux.Handle("/api/backups/", backupsH)
	mux.Handle("/api/audit", handler.AdminMiddleware(sessions, handler.AuditHandler(sessions, opts.Audit)))

	// Server-side metric history (memory only), sampled in the background.
	history := metrics.NewHistory(historyCapacity)
	mux.Handle("/api/metrics/history", handler.AdminMiddleware(sessions, handler.MetricsHistoryHandler(sessions, history, historyInterval)))

	srv := &http.Server{
		Addr:    addr,
		Handler: securityHeaders(handler.HTTPMetrics(mux)),
	}

	return &Server{httpServer: srv, stopSampler: startSampler(engine, opts.Replication, history, historyInterval)}, nil
}

// securityHeaders injects defense-in-depth headers into all web and API responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// Serve starts the HTTP server — call this in a goroutine.
func (s *Server) Serve() {
	log.Printf("BeastDB web console listening on http://%s", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("Web server error: %v", err)
	}
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown() {
	if s.stopSampler != nil {
		s.stopSampler()
		s.stopSampler = nil
	}
	_ = s.httpServer.Close()
}
