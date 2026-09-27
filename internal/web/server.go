package web

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/ChromaBeast/beastdb/internal/web/auth"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
)

//go:embed all:static
var staticFiles embed.FS

// Server is the embedded HTTP admin server running alongside the gRPC server.
type Server struct {
	httpServer *http.Server
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
	rateLimiter := handler.NewLoginRateLimiter(5, 30*time.Second)

	deps := &handler.Deps{
		Users:       users,
		Sessions:    sessions,
		StaticFS:    staticFS,
		RateLimiter: rateLimiter,
	}

	mux := http.NewServeMux()

	// Public static assets (CSS, JS, Next.js artifacts)
	mux.Handle("/css/", http.FileServer(http.FS(staticFS)))
	mux.Handle("/js/", http.FileServer(http.FS(staticFS)))
	mux.Handle("/brand/", http.FileServer(http.FS(staticFS)))
	mux.Handle("/icon.svg", http.FileServer(http.FS(staticFS)))
	mux.Handle("/_next/", http.FileServer(http.FS(staticFS)))

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


	srv := &http.Server{
		Addr:    addr,
		Handler: securityHeaders(mux),
	}

	return &Server{httpServer: srv}, nil
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
	_ = s.httpServer.Close()
}
