package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/agentwarden/agentwarden/internal/approval"
	"github.com/agentwarden/agentwarden/internal/auth"
	"github.com/agentwarden/agentwarden/internal/config"
	"github.com/agentwarden/agentwarden/internal/session"
	"github.com/agentwarden/agentwarden/internal/trace"
)

// Server is the management API + dashboard server.
type Server struct {
	config       config.ServerConfig
	store        trace.Store
	cfgLoader    *config.Loader
	approvals    *approval.Queue
	sessions     *session.Manager
	tokenManager *auth.TokenManager
	wsHub        *WebSocketHub
	mux          *http.ServeMux
	httpServer   *http.Server
	logger       *slog.Logger
}

// NewServer creates a new management API server.
func NewServer(
	cfg config.ServerConfig,
	store trace.Store,
	cfgLoader *config.Loader,
	approvals *approval.Queue,
	sessions *session.Manager,
	tokenManager *auth.TokenManager,
	logger *slog.Logger,
) *Server {
	s := &Server{
		config:       cfg,
		store:        store,
		cfgLoader:    cfgLoader,
		approvals:    approvals,
		sessions:     sessions,
		tokenManager: tokenManager,
		wsHub:        NewWebSocketHub(logger, cfg.CORS),
		mux:          http.NewServeMux(),
		logger:       logger,
	}

	s.registerRoutes()
	return s
}

// authRequired wraps a handler with token-based authentication. If auth is
// disabled in config, the handler is returned unwrapped with no overhead.
func (s *Server) authRequired(action string, next http.HandlerFunc) http.HandlerFunc {
	if !s.config.Auth.Enabled || s.tokenManager == nil {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
			return
		}
		secret := strings.TrimPrefix(header, "Bearer ")

		token, err := s.tokenManager.ValidateToken(secret, r.RemoteAddr)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		if !auth.HasPermission(token.Role, action) {
			writeError(w, http.StatusForbidden, "insufficient permissions")
			return
		}

		next(w, r)
	}
}

func (s *Server) registerRoutes() {
	// Sessions
	s.mux.HandleFunc("GET /api/sessions", s.authRequired("session.read", s.handleListSessions))
	s.mux.HandleFunc("GET /api/sessions/{id}", s.authRequired("session.read", s.handleGetSession))
	s.mux.HandleFunc("DELETE /api/sessions/{id}", s.authRequired("session.terminate", s.handleTerminateSession))

	// Traces
	s.mux.HandleFunc("GET /api/traces", s.authRequired("trace", s.handleListTraces))
	s.mux.HandleFunc("GET /api/traces/{id}", s.authRequired("trace", s.handleGetTrace))
	s.mux.HandleFunc("GET /api/traces/search", s.authRequired("trace", s.handleSearchTraces))

	// Agents
	s.mux.HandleFunc("GET /api/agents", s.authRequired("session.read", s.handleListAgents))
	s.mux.HandleFunc("GET /api/agents/{id}", s.authRequired("session.read", s.handleGetAgent))
	s.mux.HandleFunc("GET /api/agents/{id}/stats", s.authRequired("session.read", s.handleGetAgentStats))
	s.mux.HandleFunc("GET /api/agents/{id}/versions", s.authRequired("session.read", s.handleListAgentVersions))
	s.mux.HandleFunc("POST /api/agents/{id}/pause", s.authRequired("session.terminate", s.handlePauseAgent))
	s.mux.HandleFunc("POST /api/agents/{id}/resume", s.authRequired("session.terminate", s.handleResumeAgent))

	// Policies
	s.mux.HandleFunc("GET /api/policies", s.authRequired("session.read", s.handleListPolicies))
	s.mux.HandleFunc("POST /api/policies/reload", s.authRequired("config.change", s.handleReloadPolicies))

	// Approvals
	s.mux.HandleFunc("GET /api/approvals", s.authRequired("session.read", s.handleListApprovals))
	s.mux.HandleFunc("POST /api/approvals/{id}/approve", s.authRequired("session.terminate", s.handleApproveAction))
	s.mux.HandleFunc("POST /api/approvals/{id}/deny", s.authRequired("session.terminate", s.handleDenyAction))

	// Violations
	s.mux.HandleFunc("GET /api/violations", s.authRequired("session.read", s.handleListViolations))

	// System — health is always public
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/stats", s.authRequired("session.read", s.handleStats))

	// WebSocket
	s.mux.HandleFunc("GET /api/ws/traces", s.wsHub.HandleWebSocket)
}

// Handler returns the HTTP handler (for embedding in the proxy server).
func (s *Server) Handler() http.Handler {
	if s.config.CORS {
		return corsMiddleware(s.mux)
	}
	return s.mux
}

// Start starts the API server on the given address.
func (s *Server) Start(addr string) error {
	go s.wsHub.Run()

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("management API listening", "addr", addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.wsHub.Close()
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// BroadcastTrace sends a trace event to all WebSocket clients.
func (s *Server) BroadcastTrace(t *trace.Trace) {
	s.wsHub.Broadcast(t)
}

// corsMiddleware adds CORS headers for development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-AgentWarden-Agent-Id, X-AgentWarden-Session-Id")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Mux returns the underlying ServeMux for mounting additional routes.
func (s *Server) Mux() *http.ServeMux {
	return s.mux
}

// Store returns the trace store.
func (s *Server) Store() trace.Store {
	return s.store
}

// Fmt helper — makes port string from int.
func APIAddr(port int) string {
	return fmt.Sprintf(":%d", port)
}
