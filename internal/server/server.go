package server

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
)

// Config holds server configuration.
type Config struct {
	Addr string // listen address, default ":8314"
}

// Server is the golem API server.
type Server struct {
	cfg       Config
	mux       *http.ServeMux
	mu        sync.RWMutex
	projects  map[string]*project
	processes map[string]*managedProcess
}

// New creates a new Server.
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8314"
	}
	s := &Server{
		cfg:       cfg,
		mux:       http.NewServeMux(),
		projects:  make(map[string]*project),
		processes: make(map[string]*managedProcess),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Handler returns the HTTP handler with CORS middleware.
func (s *Server) Handler() http.Handler {
	return cors(s.mux)
}

// ListenAndServe starts the server.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	return http.Serve(ln, s.Handler())
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
