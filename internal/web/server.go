package web

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/ChromaBeast/beastdb/internal/web/auth"
	"github.com/ChromaBeast/beastdb/internal/web/handler"
)

//go:embed static
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
// Call Serve() to start accepting connections.
func NewServer(addr string, engine EngineBackend, secret []byte) (*Server, error) {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil, err
	}

	sessions := auth.NewSessionManager(secret)
	users := auth.NewUserStore(engine)
	apiH := handler.NewAPIHandler(engine, sessions)

	deps := &handler.Deps{Users: users, Sessions: sessions, StaticFS: staticFS}

	mux := http.NewServeMux()

	// Public static assets (CSS, JS)
	mux.Handle("/css/", http.FileServer(http.FS(staticFS)))
	mux.Handle("/js/", http.FileServer(http.FS(staticFS)))

	// Public auth routes
	mux.HandleFunc("/login", handler.LoginHandler(deps))
	mux.HandleFunc("/logout", handler.LogoutHandler(deps))

	// Protected dashboard — serves index.html
	dashboardFS := http.FileServer(http.FS(staticFS))
	mux.Handle("/", handler.AuthMiddleware(sessions, dashboardFS))

	// Protected telemetry route
	mux.Handle("/api/stats", handler.AuthMiddleware(sessions, http.HandlerFunc(apiH.GetStats)))

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

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return &Server{httpServer: srv}, nil
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
