package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/cost"
	"github.com/agentwarden/agentwarden/internal/detection"
	"github.com/agentwarden/agentwarden/internal/policy"
	"github.com/agentwarden/agentwarden/internal/session"
	"github.com/agentwarden/agentwarden/internal/trace"
)

// ---------------------------------------------------------------------------
// Mock implementations for HTTP tests
// ---------------------------------------------------------------------------

type mockPolicyEngineHTTP struct {
	result policy.PolicyResult
}

func (m *mockPolicyEngineHTTP) Evaluate(_ policy.ActionContext) policy.PolicyResult {
	return m.result
}

type mockDetectionEngineHTTP struct {
	lastEvent detection.ActionEvent
}

func (m *mockDetectionEngineHTTP) Analyze(event detection.ActionEvent) {
	m.lastEvent = event
}

type mockAlertManagerHTTP struct {
	lastAlert alert.Alert
	sendCount int
}

func (m *mockAlertManagerHTTP) Send(a alert.Alert) {
	m.sendCount++
	m.lastAlert = a
}

func (m *mockAlertManagerHTTP) HasSenders() bool {
	return true
}

type mockStoreHTTP struct {
	traces        []*trace.Trace
	insertError   error
	scoreError    error
	lastScoreJSON []byte
}

// InsertTrace implements trace.Store
func (m *mockStoreHTTP) InsertTrace(t *trace.Trace) error {
	if m.insertError != nil {
		return m.insertError
	}
	m.traces = append(m.traces, t)
	return nil
}

// ScoreSession implements trace.Store
func (m *mockStoreHTTP) ScoreSession(sessionID string, scoreJSON []byte) error {
	if m.scoreError != nil {
		return m.scoreError
	}
	m.lastScoreJSON = scoreJSON
	return nil
}

// Stub methods for unused trace.Store interface methods
func (m *mockStoreHTTP) Initialize() error                                              { return nil }
func (m *mockStoreHTTP) Close() error                                                   { return nil }
func (m *mockStoreHTTP) GetTrace(id string) (*trace.Trace, error)                       { return nil, nil }
func (m *mockStoreHTTP) ListTraces(filter trace.TraceFilter) ([]*trace.Trace, int, error) { return nil, 0, nil }
func (m *mockStoreHTTP) SearchTraces(query string, limit int) ([]*trace.Trace, error)   { return nil, nil }
func (m *mockStoreHTTP) UpsertSession(s *trace.Session) error                           { return nil }
func (m *mockStoreHTTP) GetSession(id string) (*trace.Session, error)                   { return nil, nil }
func (m *mockStoreHTTP) ListSessions(filter trace.SessionFilter) ([]*trace.Session, int, error) { return nil, 0, nil }
func (m *mockStoreHTTP) UpdateSessionStatus(id, status string) error                    { return nil }
func (m *mockStoreHTTP) UpdateSessionCost(id string, cost float64, actionCount int) error { return nil }
func (m *mockStoreHTTP) UpsertAgent(a *trace.Agent) error                               { return nil }
func (m *mockStoreHTTP) GetAgent(id string) (*trace.Agent, error)                       { return nil, nil }
func (m *mockStoreHTTP) ListAgents() ([]*trace.Agent, error)                            { return nil, nil }
func (m *mockStoreHTTP) GetAgentStats(agentID string) (*trace.AgentStats, error)        { return nil, nil }
func (m *mockStoreHTTP) InsertAgentVersion(v *trace.AgentVersion) error                 { return nil }
func (m *mockStoreHTTP) GetAgentVersion(id string) (*trace.AgentVersion, error)         { return nil, nil }
func (m *mockStoreHTTP) ListAgentVersions(agentID string) ([]*trace.AgentVersion, error) { return nil, nil }
func (m *mockStoreHTTP) InsertApproval(a *trace.Approval) error                         { return nil }
func (m *mockStoreHTTP) GetApproval(id string) (*trace.Approval, error)                 { return nil, nil }
func (m *mockStoreHTTP) ListPendingApprovals() ([]*trace.Approval, error)               { return nil, nil }
func (m *mockStoreHTTP) ResolveApproval(id, status, resolvedBy string) error            { return nil }
func (m *mockStoreHTTP) InsertViolation(v *trace.Violation) error                       { return nil }
func (m *mockStoreHTTP) ListViolations(agentID string, limit int) ([]*trace.Violation, error) { return nil, nil }
func (m *mockStoreHTTP) PruneOlderThan(days int) (int64, error)                         { return 0, nil }
func (m *mockStoreHTTP) VerifyHashChain(sessionID string) (bool, int, error)            { return true, 0, nil }
func (m *mockStoreHTTP) GetSystemStats() (*trace.SystemStats, error)                    { return nil, nil }

// ---------------------------------------------------------------------------
// Test: NewHTTPEventsServer
// ---------------------------------------------------------------------------

func TestNewHTTPEventsServer(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, logger)

		if srv == nil {
			t.Fatal("expected non-nil server")
		}
		if srv.logger == nil {
			t.Fatal("expected logger to be set")
		}
	})

	t.Run("without logger (default)", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		if srv == nil {
			t.Fatal("expected non-nil server")
		}
		if srv.logger == nil {
			t.Fatal("expected default logger to be set")
		}
	})
}

// ---------------------------------------------------------------------------
// Test: RegisterRoutes
// ---------------------------------------------------------------------------

func TestRegisterRoutes(t *testing.T) {
	mockStore := &mockStoreHTTP{}
	mockSessions := session.NewManager(mockStore, nil)
	mockPolicy := &mockPolicyEngineHTTP{
		result: policy.PolicyResult{Effect: policy.EffectAllow},
	}
	mockCost := cost.NewTracker(nil)
	srv := NewHTTPEventsServer(mockPolicy, mockStore, mockSessions, mockCost, nil, nil, nil)
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	// Verify routes are registered by checking they don't 404
	// We test basic route registration, not full handler logic
	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/events/evaluate"},
		{"POST", "/v1/events/trace"},
		{"POST", "/v1/sessions/start"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			// We expect the handlers to be called (even if they return errors for empty bodies)
			// A 404 would mean the route wasn't registered
			if rec.Code == http.StatusNotFound {
				t.Errorf("route not registered: %s %s", route.method, route.path)
			}
		})
	}

	// Test path param routes separately
	t.Run("POST /v1/sessions/{id}/end", func(t *testing.T) {
		// Create a session first so the handler doesn't fail with not found
		mockSessions.GetOrCreate("test-agent", "route-test-id", []byte("{}"))

		req := httptest.NewRequest("POST", "/v1/sessions/route-test-id/end", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Errorf("route not registered: POST /v1/sessions/{id}/end")
		}
	})

	t.Run("POST /v1/sessions/{id}/score", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/sessions/route-test-score/score", strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Errorf("route not registered: POST /v1/sessions/{id}/score")
		}
	})
}

// ---------------------------------------------------------------------------
// Test: handleEvaluate
// ---------------------------------------------------------------------------

func TestHandleEvaluate(t *testing.T) {
	t.Run("successful allow verdict", func(t *testing.T) {
		mockPolicy := &mockPolicyEngineHTTP{
			result: policy.PolicyResult{
				Effect:     policy.EffectAllow,
				PolicyName: "default-allow",
				Message:    "action allowed",
			},
		}
		mockStore := &mockStoreHTTP{}
		mockDetection := &mockDetectionEngineHTTP{}
		mockAlerts := &mockAlertManagerHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(mockPolicy, mockStore, mockSessions, nil, mockDetection, mockAlerts, nil)

		reqBody := evaluateRequest{
			SessionID:    "sess-123",
			AgentID:      "agent-1",
			AgentVersion: "v1",
			Action: actionPayload{
				Type:       "tool.call",
				Name:       "read_file",
				ParamsJSON: `{"path": "/etc/passwd"}`,
				Target:     "/etc/passwd",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/events/evaluate", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleEvaluate(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var resp verdictResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Verdict != policy.EffectAllow {
			t.Errorf("expected verdict allow, got %s", resp.Verdict)
		}
		if resp.TraceID == "" {
			t.Error("expected non-empty trace_id")
		}
		if resp.PolicyName != "default-allow" {
			t.Errorf("expected policy_name default-allow, got %s", resp.PolicyName)
		}

		// Allow async goroutines to complete
		time.Sleep(50 * time.Millisecond)

		// Verify detection was called
		if mockDetection.lastEvent.SessionID != "sess-123" {
			t.Errorf("expected detection to be called for sess-123, got %s", mockDetection.lastEvent.SessionID)
		}

		// Verify no alert was sent (allow verdict)
		if mockAlerts.sendCount != 0 {
			t.Errorf("expected no alerts for allow verdict, got %d", mockAlerts.sendCount)
		}
	})

	t.Run("deny verdict triggers alert", func(t *testing.T) {
		mockPolicy := &mockPolicyEngineHTTP{
			result: policy.PolicyResult{
				Effect:     policy.EffectDeny,
				PolicyName: "block-passwd",
				Message:    "access to /etc/passwd denied",
			},
		}
		mockStore := &mockStoreHTTP{}
		mockAlerts := &mockAlertManagerHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(mockPolicy, mockStore, mockSessions, nil, nil, mockAlerts, nil)

		reqBody := evaluateRequest{
			SessionID: "sess-456",
			AgentID:   "agent-2",
			Action: actionPayload{
				Type:   "tool.call",
				Name:   "read_file",
				Target: "/etc/passwd",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/events/evaluate", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleEvaluate(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var resp verdictResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Verdict != policy.EffectDeny {
			t.Errorf("expected verdict deny, got %s", resp.Verdict)
		}

		// Allow async alert to be sent
		time.Sleep(50 * time.Millisecond)

		if mockAlerts.sendCount != 1 {
			t.Errorf("expected 1 alert, got %d", mockAlerts.sendCount)
		}
		if mockAlerts.lastAlert.Severity != "warning" {
			t.Errorf("expected severity warning, got %s", mockAlerts.lastAlert.Severity)
		}
	})

	t.Run("terminate verdict triggers critical alert", func(t *testing.T) {
		mockPolicy := &mockPolicyEngineHTTP{
			result: policy.PolicyResult{
				Effect:     policy.EffectTerminate,
				PolicyName: "kill-switch",
				Message:    "session terminated",
			},
		}
		mockStore := &mockStoreHTTP{}
		mockAlerts := &mockAlertManagerHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(mockPolicy, mockStore, mockSessions, nil, nil, mockAlerts, nil)

		reqBody := evaluateRequest{
			SessionID: "sess-789",
			AgentID:   "agent-3",
			Action: actionPayload{
				Type: "tool.call",
				Name: "exec",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/events/evaluate", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleEvaluate(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		// Allow async alert to be sent
		time.Sleep(50 * time.Millisecond)

		if mockAlerts.sendCount != 1 {
			t.Errorf("expected 1 alert, got %d", mockAlerts.sendCount)
		}
		if mockAlerts.lastAlert.Severity != "critical" {
			t.Errorf("expected severity critical, got %s", mockAlerts.lastAlert.Severity)
		}
	})

	t.Run("approve verdict returns approval_id", func(t *testing.T) {
		mockPolicy := &mockPolicyEngineHTTP{
			result: policy.PolicyResult{
				Effect:     policy.EffectApprove,
				PolicyName: "require-approval",
				Message:    "action requires approval",
			},
		}
		mockStore := &mockStoreHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(mockPolicy, mockStore, mockSessions, nil, nil, nil, nil)

		reqBody := evaluateRequest{
			SessionID: "sess-approve",
			AgentID:   "agent-4",
			Action: actionPayload{
				Type: "tool.call",
				Name: "delete_file",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/events/evaluate", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleEvaluate(rec, req)

		var resp verdictResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp.Verdict != policy.EffectApprove {
			t.Errorf("expected verdict approve, got %s", resp.Verdict)
		}
		if resp.ApprovalID == "" {
			t.Error("expected non-empty approval_id")
		}
		if resp.TimeoutSeconds != 300 {
			t.Errorf("expected timeout_seconds 300, got %d", resp.TimeoutSeconds)
		}
	})

	t.Run("missing action type returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		reqBody := evaluateRequest{
			SessionID: "sess-bad",
			AgentID:   "agent-5",
			Action: actionPayload{
				Name: "read_file", // missing Type
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/events/evaluate", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleEvaluate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp["ok"] != false {
			t.Error("expected ok=false in error response")
		}
		if !strings.Contains(resp["message"].(string), "action.type is required") {
			t.Errorf("expected error message about action.type, got %s", resp["message"])
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/events/evaluate", strings.NewReader("{invalid json"))
		rec := httptest.NewRecorder()

		srv.handleEvaluate(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})

	t.Run("params JSON parsed correctly", func(t *testing.T) {
		mockPolicy := &mockPolicyEngineHTTP{
			result: policy.PolicyResult{Effect: policy.EffectAllow},
		}
		mockStore := &mockStoreHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(mockPolicy, mockStore, mockSessions, nil, nil, nil, nil)

		reqBody := evaluateRequest{
			SessionID: "sess-ctx",
			AgentID:   "agent-ctx",
			Action: actionPayload{
				Type:       "tool.call",
				Name:       "search",
				ParamsJSON: `{"query": "test"}`,
				Target:     "database",
			},
			Context: &contextPayload{
				SessionCost:        0.15,
				SessionActionCount: 5,
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/events/evaluate", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleEvaluate(rec, req)

		// Just verify the request was processed successfully
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var resp verdictResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp.Verdict != policy.EffectAllow {
			t.Errorf("expected verdict allow, got %s", resp.Verdict)
		}
	})
}

// ---------------------------------------------------------------------------
// Test: handleTrace
// ---------------------------------------------------------------------------

func TestHandleTrace(t *testing.T) {
	t.Run("successful trace ingestion", func(t *testing.T) {
		mockStore := &mockStoreHTTP{}
		mockDetection := &mockDetectionEngineHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(nil, mockStore, mockSessions, nil, mockDetection, nil, nil)

		reqBody := evaluateRequest{
			SessionID: "sess-trace",
			AgentID:   "agent-trace",
			Action: actionPayload{
				Type: "tool.call",
				Name: "write_file",
			},
			Metadata: map[string]string{"source": "sdk"},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/events/trace", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleTrace(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Errorf("expected status 202, got %d", rec.Code)
		}

		var resp ackResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if !resp.OK {
			t.Error("expected ok=true")
		}
		if !strings.Contains(resp.Message, "trace") {
			t.Errorf("expected message to contain 'trace', got %s", resp.Message)
		}

		// Allow async trace insertion
		time.Sleep(50 * time.Millisecond)

		if len(mockStore.traces) != 1 {
			t.Errorf("expected 1 trace, got %d", len(mockStore.traces))
		}
		if mockStore.traces[0].Status != trace.StatusAllowed {
			t.Errorf("expected status allowed, got %s", mockStore.traces[0].Status)
		}

		// Verify detection was triggered
		if mockDetection.lastEvent.SessionID != "sess-trace" {
			t.Error("expected detection to be triggered")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/events/trace", strings.NewReader("not json"))
		rec := httptest.NewRecorder()

		srv.handleTrace(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Test: handleStartSession
// ---------------------------------------------------------------------------

func TestHandleStartSession(t *testing.T) {
	t.Run("successful session start", func(t *testing.T) {
		mockStore := &mockStoreHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(nil, mockStore, mockSessions, nil, nil, nil, nil)

		reqBody := startSessionRequest{
			SessionID:    "sess-start",
			AgentID:      "agent-start",
			AgentVersion: "v1",
			Metadata: map[string]string{
				"user_id": "user-123",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/sessions/start", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleStartSession(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var resp startSessionResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if !resp.OK {
			t.Error("expected ok=true")
		}
		if resp.SessionID != "sess-start" {
			t.Errorf("expected session_id sess-start, got %s", resp.SessionID)
		}
		if resp.Message != "session registered" {
			t.Errorf("expected message 'session registered', got %s", resp.Message)
		}

		// Verify session was created
		sess := mockSessions.Get("sess-start")
		if sess == nil {
			t.Error("expected session to be created")
		}
	})

	t.Run("generated session ID when empty", func(t *testing.T) {
		mockStore := &mockStoreHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(nil, mockStore, mockSessions, nil, nil, nil, nil)

		reqBody := startSessionRequest{
			AgentID:      "agent-autoid",
			AgentVersion: "v1",
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/sessions/start", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleStartSession(rec, req)

		var resp startSessionResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp.SessionID == "" {
			t.Error("expected generated session ID")
		}
		if !resp.OK {
			t.Error("expected ok=true")
		}
	})

	t.Run("missing agent_id returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		reqBody := startSessionRequest{
			SessionID: "sess-noagent",
			// AgentID missing
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/sessions/start", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleStartSession(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		if !strings.Contains(resp["message"].(string), "agent_id is required") {
			t.Errorf("expected error about agent_id, got %s", resp["message"])
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/sessions/start", strings.NewReader("bad json"))
		rec := httptest.NewRecorder()

		srv.handleStartSession(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Test: handleEndSession
// ---------------------------------------------------------------------------

func TestHandleEndSession(t *testing.T) {
	t.Run("successful session end", func(t *testing.T) {
		mockStore := &mockStoreHTTP{}
		mockSessions := session.NewManager(mockStore, nil)
		mockCost := cost.NewTracker(nil)

		// Create a session first
		sess, _ := mockSessions.GetOrCreate("agent-end", "sess-end", []byte("{}"))
		mockSessions.AddCost(sess.ID, 0.25)

		srv := NewHTTPEventsServer(nil, mockStore, mockSessions, mockCost, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/sessions/sess-end/end", nil)
		rec := httptest.NewRecorder()

		srv.handleEndSession(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var resp endSessionResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if resp.SessionID != "sess-end" {
			t.Errorf("expected session_id sess-end, got %s", resp.SessionID)
		}
		if resp.TotalCost != 0.25 {
			t.Errorf("expected total_cost 0.25, got %f", resp.TotalCost)
		}
		if resp.Status != "completed" {
			t.Errorf("expected status completed, got %s", resp.Status)
		}

		// Verify session was ended
		if mockSessions.Get("sess-end") != nil {
			t.Error("expected session to be removed after end")
		}
	})

	t.Run("non-existent session returns error", func(t *testing.T) {
		mockStore := &mockStoreHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(nil, mockStore, mockSessions, nil, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/sessions/sess-notfound/end", nil)
		rec := httptest.NewRecorder()

		srv.handleEndSession(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		if !strings.Contains(resp["message"].(string), "not found") {
			t.Errorf("expected error about not found, got %s", resp["message"])
		}
	})

	t.Run("missing session id returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/sessions//end", nil)
		rec := httptest.NewRecorder()

		srv.handleEndSession(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Test: handleScoreSession
// ---------------------------------------------------------------------------

func TestHandleScoreSession(t *testing.T) {
	t.Run("successful session scoring", func(t *testing.T) {
		mockStore := &mockStoreHTTP{}
		mockSessions := session.NewManager(mockStore, nil)

		srv := NewHTTPEventsServer(nil, mockStore, mockSessions, nil, nil, nil, nil)

		reqBody := scoreSessionRequest{
			TaskCompleted: true,
			Quality:       0.85,
			Metrics: map[string]string{
				"accuracy": "high",
			},
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/sessions/sess-score/score", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleScoreSession(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var resp ackResponse
		json.NewDecoder(rec.Body).Decode(&resp)

		if !resp.OK {
			t.Error("expected ok=true")
		}
		if resp.Message != "score recorded" {
			t.Errorf("expected message 'score recorded', got %s", resp.Message)
		}

		// Verify score was recorded
		if mockStore.lastScoreJSON == nil {
			t.Error("expected score to be recorded")
		}

		var scorePayload map[string]interface{}
		json.Unmarshal(mockStore.lastScoreJSON, &scorePayload)

		if scorePayload["task_completed"] != true {
			t.Error("expected task_completed=true")
		}
		if scorePayload["quality"] != 0.85 {
			t.Errorf("expected quality 0.85, got %v", scorePayload["quality"])
		}
	})

	t.Run("store error returns error", func(t *testing.T) {
		mockStore := &mockStoreHTTP{
			scoreError: errors.New("store error"),
		}

		srv := NewHTTPEventsServer(nil, mockStore, nil, nil, nil, nil, nil)

		reqBody := scoreSessionRequest{
			TaskCompleted: true,
			Quality:       0.5,
		}

		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/v1/sessions/sess-error/score", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		srv.handleScoreSession(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", rec.Code)
		}
	})

	t.Run("missing session id returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/sessions//score", strings.NewReader("{}"))
		rec := httptest.NewRecorder()

		srv.handleScoreSession(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := httptest.NewRequest("POST", "/v1/sessions/sess-bad/score", strings.NewReader("not json"))
		rec := httptest.NewRecorder()

		srv.handleScoreSession(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Test: extractPathParam
// ---------------------------------------------------------------------------

func TestExtractPathParam(t *testing.T) {
	t.Run("extract from PathValue (Go 1.22+)", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/sessions/test-id-123/end", nil)
		req.SetPathValue("id", "test-id-123")

		result := extractPathParam(req, "id")
		if result != "test-id-123" {
			t.Errorf("expected test-id-123, got %s", result)
		}
	})

	t.Run("fallback to URL parsing", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/sessions/fallback-id/end", nil)

		result := extractPathParam(req, "id")
		if result != "fallback-id" {
			t.Errorf("expected fallback-id, got %s", result)
		}
	})

	t.Run("empty when no match", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/sessions/", nil)

		result := extractPathParam(req, "id")
		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})
}

// ---------------------------------------------------------------------------
// Test: recordTrace (internal helper)
// ---------------------------------------------------------------------------

func TestHTTPRecordTrace(t *testing.T) {
	t.Run("maps verdict to trace status correctly", func(t *testing.T) {
		testCases := []struct {
			effect         string
			expectedStatus trace.TraceStatus
		}{
			{policy.EffectAllow, trace.StatusAllowed},
			{policy.EffectDeny, trace.StatusDenied},
			{policy.EffectTerminate, trace.StatusTerminated},
			{policy.EffectApprove, trace.StatusPending},
			{policy.EffectThrottle, trace.StatusThrottled},
		}

		for _, tc := range testCases {
			t.Run(tc.effect, func(t *testing.T) {
				mockStore := &mockStoreHTTP{}
				mockSessions := session.NewManager(mockStore, nil)

				srv := NewHTTPEventsServer(nil, mockStore, mockSessions, nil, nil, nil, nil)

				req := evaluateRequest{
					SessionID: "sess-status",
					AgentID:   "agent-status",
					Action: actionPayload{
						Type: "tool.call",
						Name: "test",
					},
				}

				result := policy.PolicyResult{
					Effect:     tc.effect,
					PolicyName: "test-policy",
					Message:    "test message",
				}

				srv.recordTrace(req, result, "trace-123", 100)

				// Allow async insertion
				time.Sleep(50 * time.Millisecond)

				if len(mockStore.traces) != 1 {
					t.Fatalf("expected 1 trace, got %d", len(mockStore.traces))
				}

				if mockStore.traces[0].Status != tc.expectedStatus {
					t.Errorf("expected status %s, got %s", tc.expectedStatus, mockStore.traces[0].Status)
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// Test: runDetection (internal helper)
// ---------------------------------------------------------------------------

func TestRunDetection(t *testing.T) {
	t.Run("calls detection with correct event", func(t *testing.T) {
		mockDetection := &mockDetectionEngineHTTP{}

		srv := NewHTTPEventsServer(nil, nil, nil, nil, mockDetection, nil, nil)

		req := evaluateRequest{
			SessionID: "sess-detect",
			AgentID:   "agent-detect",
			Action: actionPayload{
				Type:   "tool.call",
				Name:   "search",
				Target: "database",
			},
			Context: &contextPayload{
				SessionCost: 0.10,
			},
		}

		srv.runDetection(req)

		if mockDetection.lastEvent.SessionID != "sess-detect" {
			t.Errorf("expected session_id sess-detect, got %s", mockDetection.lastEvent.SessionID)
		}
		if mockDetection.lastEvent.ActionType != "tool.call" {
			t.Errorf("expected action_type tool.call, got %s", mockDetection.lastEvent.ActionType)
		}
		if mockDetection.lastEvent.Signature != "tool.call:search:database" {
			t.Errorf("unexpected signature: %s", mockDetection.lastEvent.Signature)
		}
	})

	t.Run("handles nil detection engine", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := evaluateRequest{
			SessionID: "sess-nil",
			AgentID:   "agent-nil",
			Action: actionPayload{
				Type: "tool.call",
				Name: "test",
			},
		}

		// Should not panic
		srv.runDetection(req)
	})
}

// ---------------------------------------------------------------------------
// Test: sendViolationAlert (internal helper)
// ---------------------------------------------------------------------------

func TestSendViolationAlert(t *testing.T) {
	t.Run("sends warning alert for deny", func(t *testing.T) {
		mockAlerts := &mockAlertManagerHTTP{}

		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, mockAlerts, nil)

		req := evaluateRequest{
			SessionID: "sess-alert",
			AgentID:   "agent-alert",
			Action: actionPayload{
				Type:   "tool.call",
				Name:   "delete",
				Target: "/data",
			},
		}

		result := policy.PolicyResult{
			Effect:     policy.EffectDeny,
			PolicyName: "protect-data",
			Message:    "deletion denied",
		}

		srv.sendViolationAlert(req, result)

		if mockAlerts.sendCount != 1 {
			t.Errorf("expected 1 alert, got %d", mockAlerts.sendCount)
		}
		if mockAlerts.lastAlert.Severity != "warning" {
			t.Errorf("expected severity warning, got %s", mockAlerts.lastAlert.Severity)
		}
		if mockAlerts.lastAlert.Type != "policy_violation" {
			t.Errorf("expected type policy_violation, got %s", mockAlerts.lastAlert.Type)
		}
	})

	t.Run("sends critical alert for terminate", func(t *testing.T) {
		mockAlerts := &mockAlertManagerHTTP{}

		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, mockAlerts, nil)

		req := evaluateRequest{
			SessionID: "sess-terminate",
			AgentID:   "agent-terminate",
			Action: actionPayload{
				Type: "tool.call",
				Name: "shutdown",
			},
		}

		result := policy.PolicyResult{
			Effect:     policy.EffectTerminate,
			PolicyName: "emergency-stop",
			Message:    "session terminated",
		}

		srv.sendViolationAlert(req, result)

		if mockAlerts.lastAlert.Severity != "critical" {
			t.Errorf("expected severity critical, got %s", mockAlerts.lastAlert.Severity)
		}
	})

	t.Run("handles nil alerts manager", func(t *testing.T) {
		srv := NewHTTPEventsServer(nil, nil, nil, nil, nil, nil, nil)

		req := evaluateRequest{
			SessionID: "sess-nil",
			AgentID:   "agent-nil",
			Action: actionPayload{
				Type: "tool.call",
				Name: "test",
			},
		}

		result := policy.PolicyResult{
			Effect: policy.EffectDeny,
		}

		// Should not panic
		srv.sendViolationAlert(req, result)
	})
}
