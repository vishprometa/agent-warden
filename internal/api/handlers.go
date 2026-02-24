package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// --- Sessions ---

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	filter := trace.SessionFilter{
		AgentID: r.URL.Query().Get("agent_id"),
		Status:  r.URL.Query().Get("status"),
		Limit:   queryInt(r, "limit", 50),
		Offset:  queryInt(r, "offset", 0),
	}

	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			filter.Since = &t
		}
	}

	sessions, total, err := s.store.ListSessions(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"sessions": sessions,
		"total":    total,
	})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session, err := s.store.GetSession(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if session == nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// Also fetch traces for this session
	traces, _, err := s.store.ListTraces(trace.TraceFilter{SessionID: id, Limit: 1000})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"session": session,
		"traces":  traces,
	})
}

func (s *Server) handleTerminateSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.UpdateSessionStatus(id, "terminated"); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "terminated"})
}

// --- Traces ---

func (s *Server) handleListTraces(w http.ResponseWriter, r *http.Request) {
	filter := trace.TraceFilter{
		SessionID:  r.URL.Query().Get("session_id"),
		AgentID:    r.URL.Query().Get("agent_id"),
		ActionType: trace.ActionType(r.URL.Query().Get("action_type")),
		Status:     trace.TraceStatus(r.URL.Query().Get("status")),
		Limit:      queryInt(r, "limit", 50),
		Offset:     queryInt(r, "offset", 0),
	}

	traces, total, err := s.store.ListTraces(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{
		"traces": traces,
		"total":  total,
	})
}

func (s *Server) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.store.GetTrace(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}
	writeJSON(w, t)
}

func (s *Server) handleSearchTraces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	traces, err := s.store.SearchTraces(q, queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, map[string]interface{}{"traces": traces})
}

// --- Agents ---

func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"agents": agents})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, err := s.store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, agent)
}

func (s *Server) handleGetAgentStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stats, err := s.store.GetAgentStats(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleListAgentVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	versions, err := s.store.ListAgentVersions(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"versions": versions})
}

func (s *Server) handlePauseAgent(w http.ResponseWriter, r *http.Request) {
	// TODO: implement session-level pause via session manager
	writeJSON(w, map[string]string{"status": "paused"})
}

func (s *Server) handleResumeAgent(w http.ResponseWriter, r *http.Request) {
	// TODO: implement session-level resume via session manager
	writeJSON(w, map[string]string{"status": "resumed"})
}

// --- Policies ---

func (s *Server) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	cfg := s.cfgLoader.Get()
	writeJSON(w, map[string]interface{}{"policies": cfg.Policies})
}

func (s *Server) handleReloadPolicies(w http.ResponseWriter, r *http.Request) {
	if err := s.cfgLoader.Reload(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reload: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "reloaded"})
}

// --- Approvals ---

func (s *Server) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	approvals, err := s.store.ListPendingApprovals()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"approvals": approvals})
}

func (s *Server) handleApproveAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.approvals.Resolve(id, true, "dashboard"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "approved"})
}

func (s *Server) handleDenyAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.approvals.Resolve(id, false, "dashboard"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "denied"})
}

// --- Violations ---

func (s *Server) handleListViolations(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("agent_id")
	violations, err := s.store.ListViolations(agentID, queryInt(r, "limit", 50))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"violations": violations})
}

// --- System ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetSystemStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, stats)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
