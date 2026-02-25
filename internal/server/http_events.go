package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/cost"
	"github.com/agentwarden/agentwarden/internal/detection"
	"github.com/agentwarden/agentwarden/internal/policy"
	"github.com/agentwarden/agentwarden/internal/session"
	"github.com/agentwarden/agentwarden/internal/trace"
	"github.com/oklog/ulid/v2"
)

// HTTPEventsServer provides HTTP fallback endpoints for the event evaluation
// API. These mirror the gRPC AgentWardenService RPCs for clients that cannot
// use gRPC (e.g. browser-based agents, simple curl-based integrations).
//
// Routes:
//
//	POST /v1/events/evaluate   — synchronous policy evaluation (same as gRPC EvaluateAction)
//	POST /v1/events/trace      — async trace-only ingestion (fire-and-forget, no verdict)
//	POST /v1/sessions/start    — start a session
//	POST /v1/sessions/{id}/end — end a session
//	POST /v1/sessions/{id}/score — score a session
type HTTPEventsServer struct {
	policy    PolicyEngine
	store     trace.Store
	sessions  *session.Manager
	cost      *cost.Tracker
	detection DetectionEngine
	alerts    AlertManager
	logger    *slog.Logger
}

// NewHTTPEventsServer creates a new HTTP events server with the given
// dependencies. The dependencies are identical to those used by GRPCServer.
func NewHTTPEventsServer(
	policyEngine PolicyEngine,
	store trace.Store,
	sessions *session.Manager,
	costTracker *cost.Tracker,
	detectionEngine DetectionEngine,
	alertManager AlertManager,
	logger *slog.Logger,
) *HTTPEventsServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &HTTPEventsServer{
		policy:    policyEngine,
		store:     store,
		sessions:  sessions,
		cost:      costTracker,
		detection: detectionEngine,
		alerts:    alertManager,
		logger:    logger.With("component", "server.httpEvents"),
	}
}

// RegisterRoutes mounts the event endpoints on the given ServeMux.
func (s *HTTPEventsServer) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/events/evaluate", s.handleEvaluate)
	mux.HandleFunc("POST /v1/events/trace", s.handleTrace)
	mux.HandleFunc("POST /v1/sessions/start", s.handleStartSession)
	mux.HandleFunc("POST /v1/sessions/{id}/end", s.handleEndSession)
	mux.HandleFunc("POST /v1/sessions/{id}/score", s.handleScoreSession)
}

// ---------------------------------------------------------------------------
// Request/response types
// ---------------------------------------------------------------------------

// evaluateRequest is the JSON body for POST /v1/events/evaluate.
type evaluateRequest struct {
	SessionID    string            `json:"session_id"`
	AgentID      string            `json:"agent_id"`
	AgentVersion string            `json:"agent_version"`
	Action       actionPayload     `json:"action"`
	Context      *contextPayload   `json:"context,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type actionPayload struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	ParamsJSON string `json:"params_json"`
	Target     string `json:"target"`
}

type contextPayload struct {
	SessionCost            float64 `json:"session_cost"`
	SessionActionCount     int     `json:"session_action_count"`
	SessionDurationSeconds int     `json:"session_duration_seconds"`
}

// verdictResponse is the JSON body returned from POST /v1/events/evaluate.
type verdictResponse struct {
	Verdict        string   `json:"verdict"`
	TraceID        string   `json:"trace_id"`
	PolicyName     string   `json:"policy_name,omitempty"`
	Message        string   `json:"message,omitempty"`
	ApprovalID     string   `json:"approval_id,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
	LatencyMs      int      `json:"latency_ms"`
	Suggestions    []string `json:"suggestions,omitempty"`
}

type startSessionRequest struct {
	SessionID    string            `json:"session_id"`
	AgentID      string            `json:"agent_id"`
	AgentVersion string            `json:"agent_version"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type startSessionResponse struct {
	SessionID string `json:"session_id"`
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
}

type endSessionResponse struct {
	SessionID       string  `json:"session_id"`
	TotalCost       float64 `json:"total_cost"`
	ActionCount     int     `json:"action_count"`
	DurationSeconds int     `json:"duration_seconds"`
	Status          string  `json:"status"`
}

type scoreSessionRequest struct {
	TaskCompleted bool              `json:"task_completed"`
	Quality       float64           `json:"quality"`
	Metrics       map[string]string `json:"metrics,omitempty"`
}

type ackResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// handleEvaluate is the synchronous evaluation endpoint. It mirrors
// gRPC EvaluateAction: evaluate policies, record trace, return verdict.
func (s *HTTPEventsServer) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req evaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeEventError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	defer func() { _ = r.Body.Close() }()

	if req.Action.Type == "" {
		writeEventError(w, http.StatusBadRequest, "action.type is required")
		return
	}

	// Parse action params.
	params := make(map[string]interface{})
	if req.Action.ParamsJSON != "" {
		_ = json.Unmarshal([]byte(req.Action.ParamsJSON), &params)
	}

	sessionCost := float64(0)
	actionCount := 0
	if req.Context != nil {
		sessionCost = req.Context.SessionCost
		actionCount = req.Context.SessionActionCount
	}

	policyCtx := policy.ActionContext{
		Action: policy.ActionInfo{
			Type:   req.Action.Type,
			Name:   req.Action.Name,
			Params: params,
			Target: req.Action.Target,
		},
		Session: policy.SessionInfo{
			ID:          req.SessionID,
			AgentID:     req.AgentID,
			Cost:        sessionCost,
			ActionCount: actionCount,
		},
		Agent: policy.AgentInfo{
			ID:   req.AgentID,
			Name: req.AgentID,
		},
	}

	// Evaluate policies.
	result := s.policy.Evaluate(policyCtx)

	latencyMs := int(time.Since(start).Milliseconds())
	traceID := ulid.Make().String()

	// Record trace asynchronously.
	go s.recordTrace(req, result, traceID, int32(latencyMs))

	// Fire async detection.
	go s.runDetection(req)

	// Alert on deny/terminate.
	if result.Effect == policy.EffectDeny || result.Effect == policy.EffectTerminate {
		go s.sendViolationAlert(req, result)
	}

	resp := verdictResponse{
		Verdict:    result.Effect,
		TraceID:    traceID,
		PolicyName: result.PolicyName,
		Message:    result.Message,
		LatencyMs:  latencyMs,
	}

	if result.Effect == policy.EffectApprove {
		resp.ApprovalID = traceID
		resp.TimeoutSeconds = 300
	}

	writeEventJSON(w, http.StatusOK, resp)
}

// handleTrace is the fire-and-forget trace ingestion endpoint. It records
// the action to the trace store without evaluating policies or returning a
// verdict. Useful for post-hoc auditing of actions the SDK already executed.
func (s *HTTPEventsServer) handleTrace(w http.ResponseWriter, r *http.Request) {
	var req evaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeEventError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	defer func() { _ = r.Body.Close() }()

	traceID := ulid.Make().String()

	// Record immediately (still async to not block the HTTP response).
	go func() {
		reqBody, _ := json.Marshal(req.Action)
		metaBytes, _ := json.Marshal(req.Metadata)

		t := &trace.Trace{
			ID:          traceID,
			SessionID:   req.SessionID,
			AgentID:     req.AgentID,
			Timestamp:   time.Now().UTC(),
			ActionType:  trace.ActionType(req.Action.Type),
			ActionName:  req.Action.Name,
			RequestBody: reqBody,
			Status:      trace.StatusAllowed,
			Metadata:    metaBytes,
		}

		if err := s.store.InsertTrace(t); err != nil {
			s.logger.Error("failed to insert trace",
				"trace_id", traceID,
				"error", err,
			)
		}

		// Increment session action count if session exists.
		_ = s.sessions.IncrementActions(req.SessionID, trace.ActionType(req.Action.Type))
	}()

	// Fire detection on the traced action.
	go s.runDetection(req)

	writeEventJSON(w, http.StatusAccepted, ackResponse{
		OK:      true,
		Message: fmt.Sprintf("trace %s recorded", traceID),
	})
}

// handleStartSession creates or retrieves a session.
func (s *HTTPEventsServer) handleStartSession(w http.ResponseWriter, r *http.Request) {
	var req startSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeEventError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	defer func() { _ = r.Body.Close() }()

	if req.AgentID == "" {
		writeEventError(w, http.StatusBadRequest, "agent_id is required")
		return
	}

	metadata, err := json.Marshal(req.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}

	sess, err := s.sessions.GetOrCreate(req.AgentID, req.SessionID, metadata)
	if err != nil {
		s.logger.Error("failed to start session",
			"session_id", req.SessionID,
			"agent_id", req.AgentID,
			"error", err,
		)
		writeEventJSON(w, http.StatusInternalServerError, startSessionResponse{
			SessionID: req.SessionID,
			OK:        false,
			Message:   err.Error(),
		})
		return
	}

	s.logger.Info("session started via HTTP",
		"session_id", sess.ID,
		"agent_id", req.AgentID,
	)

	writeEventJSON(w, http.StatusOK, startSessionResponse{
		SessionID: sess.ID,
		OK:        true,
		Message:   "session registered",
	})
}

// handleEndSession finalizes a session and returns its summary.
func (s *HTTPEventsServer) handleEndSession(w http.ResponseWriter, r *http.Request) {
	sessionID := extractPathParam(r, "id")
	if sessionID == "" {
		writeEventError(w, http.StatusBadRequest, "session id is required")
		return
	}

	sess := s.sessions.Get(sessionID)
	if sess == nil {
		writeEventError(w, http.StatusNotFound, fmt.Sprintf("session %s not found", sessionID))
		return
	}

	resp := endSessionResponse{
		SessionID:   sess.ID,
		TotalCost:   sess.TotalCost,
		ActionCount: sess.ActionCount,
		Status:      "completed",
	}

	if !sess.StartedAt.IsZero() {
		resp.DurationSeconds = int(time.Since(sess.StartedAt).Seconds())
	}

	if err := s.sessions.End(sessionID); err != nil {
		s.logger.Error("failed to end session",
			"session_id", sessionID,
			"error", err,
		)
		writeEventError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.cost.ResetSession(sessionID)

	s.logger.Info("session ended via HTTP",
		"session_id", sessionID,
		"total_cost", resp.TotalCost,
		"action_count", resp.ActionCount,
	)

	writeEventJSON(w, http.StatusOK, resp)
}

// handleScoreSession stores a quality score on a session.
func (s *HTTPEventsServer) handleScoreSession(w http.ResponseWriter, r *http.Request) {
	sessionID := extractPathParam(r, "id")
	if sessionID == "" {
		writeEventError(w, http.StatusBadRequest, "session id is required")
		return
	}

	var req scoreSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeEventError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	defer func() { _ = r.Body.Close() }()

	scorePayload := map[string]interface{}{
		"task_completed": req.TaskCompleted,
		"quality":        req.Quality,
		"metrics":        req.Metrics,
	}
	scoreJSON, err := json.Marshal(scorePayload)
	if err != nil {
		writeEventError(w, http.StatusInternalServerError, "failed to marshal score")
		return
	}

	if err := s.store.ScoreSession(sessionID, scoreJSON); err != nil {
		s.logger.Error("failed to score session",
			"session_id", sessionID,
			"error", err,
		)
		writeEventError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Info("session scored via HTTP",
		"session_id", sessionID,
		"quality", req.Quality,
		"task_completed", req.TaskCompleted,
	)

	writeEventJSON(w, http.StatusOK, ackResponse{
		OK:      true,
		Message: "score recorded",
	})
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// recordTrace persists an action event and its policy result to the trace store.
func (s *HTTPEventsServer) recordTrace(req evaluateRequest, result policy.PolicyResult, traceID string, latencyMs int32) {
	reqBody, _ := json.Marshal(req.Action)
	metaBytes, _ := json.Marshal(req.Metadata)

	status := trace.StatusAllowed
	switch result.Effect {
	case policy.EffectDeny:
		status = trace.StatusDenied
	case policy.EffectTerminate:
		status = trace.StatusTerminated
	case policy.EffectApprove:
		status = trace.StatusPending
	case policy.EffectThrottle:
		status = trace.StatusThrottled
	}

	t := &trace.Trace{
		ID:           traceID,
		SessionID:    req.SessionID,
		AgentID:      req.AgentID,
		Timestamp:    time.Now().UTC(),
		ActionType:   trace.ActionType(req.Action.Type),
		ActionName:   req.Action.Name,
		RequestBody:  reqBody,
		Status:       status,
		PolicyName:   result.PolicyName,
		PolicyReason: result.Message,
		LatencyMs:    int64(latencyMs),
		Metadata:     metaBytes,
	}

	if err := s.store.InsertTrace(t); err != nil {
		s.logger.Error("failed to insert trace",
			"trace_id", traceID,
			"error", err,
		)
	}

	// Update session action count.
	_ = s.sessions.IncrementActions(req.SessionID, trace.ActionType(req.Action.Type))
}

// runDetection fires the async anomaly detection pipeline.
func (s *HTTPEventsServer) runDetection(req evaluateRequest) {
	if s.detection == nil {
		return
	}

	sessionCost := float64(0)
	if req.Context != nil {
		sessionCost = req.Context.SessionCost
	}

	s.detection.Analyze(detection.ActionEvent{
		SessionID:  req.SessionID,
		AgentID:    req.AgentID,
		ActionType: req.Action.Type,
		ActionName: req.Action.Name,
		Signature:  req.Action.Type + ":" + req.Action.Name + ":" + req.Action.Target,
		CostUSD:    sessionCost,
	})
}

// sendViolationAlert dispatches an alert for policy violations.
func (s *HTTPEventsServer) sendViolationAlert(req evaluateRequest, result policy.PolicyResult) {
	if s.alerts == nil {
		return
	}

	severity := "warning"
	if result.Effect == policy.EffectTerminate {
		severity = "critical"
	}

	s.alerts.Send(alert.Alert{
		Type:      "policy_violation",
		Severity:  severity,
		Title:     fmt.Sprintf("Policy %q triggered: %s", result.PolicyName, result.Effect),
		Message:   result.Message,
		AgentID:   req.AgentID,
		SessionID: req.SessionID,
		Details: map[string]interface{}{
			"action_type": req.Action.Type,
			"action_name": req.Action.Name,
			"target":      req.Action.Target,
			"policy":      result.PolicyName,
			"effect":      result.Effect,
		},
	})
}

// ---------------------------------------------------------------------------
// JSON response helpers
// ---------------------------------------------------------------------------

func writeEventJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeEventError(w http.ResponseWriter, status int, message string) {
	writeEventJSON(w, status, map[string]interface{}{
		"ok":      false,
		"message": message,
	})
}

// extractPathParam extracts a path parameter from the request.
// Uses Go 1.22+ PathValue if available, falls back to URL path parsing.
func extractPathParam(r *http.Request, name string) string {
	// Go 1.22+ ServeMux path parameters
	if v := r.PathValue(name); v != "" {
		return v
	}
	// Fallback: extract from URL path for /v1/sessions/{id}/end pattern
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}
