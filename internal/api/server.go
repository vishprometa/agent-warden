package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentwarden/agentwarden/internal/approval"
	"github.com/agentwarden/agentwarden/internal/config"
	"github.com/agentwarden/agentwarden/internal/trace"
)

// Server is the management API + dashboard server.
type Server struct {
	config    config.ServerConfig
	store     trace.Store
	cfgLoader *config.Loader
	approvals *approval.Queue
	wsHub     *WebSocketHub
	mux       *http.ServeMux
	httpServer *http.Server
	logger    *slog.Logger
}

// NewServer creates a new management API server.
func NewServer(
	cfg config.ServerConfig,
	store trace.Store,
	cfgLoader *config.Loader,
	approvals *approval.Queue,
	logger *slog.Logger,
) *Server {
	s := &Server{
		config:    cfg,
		store:     store,
		cfgLoader: cfgLoader,
		approvals: approvals,
		wsHub:     NewWebSocketHub(logger),
		mux:       http.NewServeMux(),
		logger:    logger,
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Sessions
	s.mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("DELETE /api/sessions/{id}", s.handleTerminateSession)

	// Traces
	s.mux.HandleFunc("GET /api/traces", s.handleListTraces)
	s.mux.HandleFunc("GET /api/traces/{id}", s.handleGetTrace)
	s.mux.HandleFunc("GET /api/traces/search", s.handleSearchTraces)

	// Agents
	s.mux.HandleFunc("GET /api/agents", s.handleListAgents)
	s.mux.HandleFunc("GET /api/agents/{id}", s.handleGetAgent)
	s.mux.HandleFunc("GET /api/agents/{id}/stats", s.handleGetAgentStats)
	s.mux.HandleFunc("GET /api/agents/{id}/versions", s.handleListAgentVersions)
	s.mux.HandleFunc("POST /api/agents/{id}/pause", s.handlePauseAgent)
	s.mux.HandleFunc("POST /api/agents/{id}/resume", s.handleResumeAgent)

	// Policies
	s.mux.HandleFunc("GET /api/policies", s.handleListPolicies)
	s.mux.HandleFunc("POST /api/policies/reload", s.handleReloadPolicies)

	// Approvals
	s.mux.HandleFunc("GET /api/approvals", s.handleListApprovals)
	s.mux.HandleFunc("POST /api/approvals/{id}/approve", s.handleApproveAction)
	s.mux.HandleFunc("POST /api/approvals/{id}/deny", s.handleDenyAction)

	// Violations
	s.mux.HandleFunc("GET /api/violations", s.handleListViolations)

	// System
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)

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
