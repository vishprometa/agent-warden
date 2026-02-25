package server

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/cost"
	"github.com/agentwarden/agentwarden/internal/detection"
	"github.com/agentwarden/agentwarden/internal/policy"
	"github.com/agentwarden/agentwarden/internal/session"
	"github.com/agentwarden/agentwarden/internal/trace"
	pb "github.com/agentwarden/agentwarden/proto/agentwarden/v1"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

// mockPolicyEngine implements PolicyEngine for testing.
type mockPolicyEngine struct {
	mu      sync.Mutex
	calls   []policy.ActionContext
	result  policy.PolicyResult
	resultF func(policy.ActionContext) policy.PolicyResult
}

func (m *mockPolicyEngine) Evaluate(ctx policy.ActionContext) policy.PolicyResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, ctx)
	if m.resultF != nil {
		return m.resultF(ctx)
	}
	return m.result
}

func (m *mockPolicyEngine) getCalls() []policy.ActionContext {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]policy.ActionContext{}, m.calls...)
}

// mockDetectionEngine implements DetectionEngine for testing.
type mockDetectionEngine struct {
	mu     sync.Mutex
	events []detection.ActionEvent
}

func (m *mockDetectionEngine) Analyze(event detection.ActionEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

func (m *mockDetectionEngine) getEvents() []detection.ActionEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]detection.ActionEvent{}, m.events...)
}

// mockAlertManager implements AlertManager for testing.
type mockAlertManager struct {
	mu     sync.Mutex
	alerts []alert.Alert
}

func (m *mockAlertManager) Send(a alert.Alert) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.alerts = append(m.alerts, a)
}

func (m *mockAlertManager) getAlerts() []alert.Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]alert.Alert{}, m.alerts...)
}

// mockStore implements trace.Store for testing.
type mockStore struct {
	mu             sync.Mutex
	traces         []*trace.Trace
	sessions       map[string]*trace.Session
	insertTraceErr error
	scoreErr       error
	sessionScores  map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		sessions:      make(map[string]*trace.Session),
		sessionScores: make(map[string][]byte),
	}
}

func (m *mockStore) Initialize() error                                                  { return nil }
func (m *mockStore) Close() error                                                       { return nil }
func (m *mockStore) GetTrace(id string) (*trace.Trace, error)                           { return nil, nil }
func (m *mockStore) ListTraces(filter trace.TraceFilter) ([]*trace.Trace, int, error)   { return nil, 0, nil }
func (m *mockStore) SearchTraces(query string, limit int) ([]*trace.Trace, error)       { return nil, nil }
func (m *mockStore) GetSession(id string) (*trace.Session, error)                       { return nil, nil }
func (m *mockStore) ListSessions(filter trace.SessionFilter) ([]*trace.Session, int, error) {
	return nil, 0, nil
}
func (m *mockStore) UpdateSessionStatus(id, status string) error        { return nil }
func (m *mockStore) UpdateSessionCost(id string, cost float64, actionCount int) error {
	return nil
}
func (m *mockStore) InsertApproval(a *trace.Approval) error                       { return nil }
func (m *mockStore) GetApproval(id string) (*trace.Approval, error)               { return nil, nil }
func (m *mockStore) ListPendingApprovals() ([]*trace.Approval, error)             { return nil, nil }
func (m *mockStore) ResolveApproval(id, status, resolvedBy string) error          { return nil }
func (m *mockStore) InsertViolation(v *trace.Violation) error                     { return nil }
func (m *mockStore) ListViolations(agentID string, limit int) ([]*trace.Violation, error) {
	return nil, nil
}
func (m *mockStore) UpsertAgent(a *trace.Agent) error                             { return nil }
func (m *mockStore) GetAgent(id string) (*trace.Agent, error)                     { return nil, nil }
func (m *mockStore) ListAgents() ([]*trace.Agent, error)                          { return nil, nil }
func (m *mockStore) GetAgentStats(agentID string) (*trace.AgentStats, error)      { return nil, nil }
func (m *mockStore) InsertAgentVersion(v *trace.AgentVersion) error               { return nil }
func (m *mockStore) GetAgentVersion(id string) (*trace.AgentVersion, error)       { return nil, nil }
func (m *mockStore) ListAgentVersions(agentID string) ([]*trace.AgentVersion, error) {
	return nil, nil
}
func (m *mockStore) PruneOlderThan(days int) (int64, error)                       { return 0, nil }
func (m *mockStore) VerifyHashChain(sessionID string) (bool, int, error)          { return true, 0, nil }
func (m *mockStore) GetSystemStats() (*trace.SystemStats, error)                  { return nil, nil }

func (m *mockStore) InsertTrace(t *trace.Trace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.insertTraceErr != nil {
		return m.insertTraceErr
	}
	m.traces = append(m.traces, t)
	return nil
}

func (m *mockStore) UpsertSession(s *trace.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}

func (m *mockStore) ScoreSession(id string, score []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.scoreErr != nil {
		return m.scoreErr
	}
	m.sessionScores[id] = score
	return nil
}

func (m *mockStore) getTraces() []*trace.Trace {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*trace.Trace, len(m.traces))
	copy(result, m.traces)
	return result
}

func (m *mockStore) getScores() map[string][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string][]byte)
	for k, v := range m.sessionScores {
		result[k] = v
	}
	return result
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestNewGRPCServer(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)
		detection := &mockDetectionEngine{}
		alerts := &mockAlertManager{}

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, detection, alerts, nil)

		if server.policy != policyEngine {
			t.Error("policy engine not set")
		}
		if server.logger == nil {
			t.Error("logger should be set to default")
		}
	})
}

func TestEvaluateAction(t *testing.T) {
	t.Run("successful evaluation allow", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{
			result: policy.PolicyResult{
				Effect:     policy.EffectAllow,
				PolicyName: "allow-all",
				Message:    "action allowed",
			},
		}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)
		detection := &mockDetectionEngine{}
		alerts := &mockAlertManager{}

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, detection, alerts, nil)

		req := &pb.ActionEvent{
			SessionId: "test-session",
			AgentId:   "test-agent",
			Action: &pb.Action{
				Type:   "tool.call",
				Name:   "search",
				Target: "google",
			},
		}

		verdict, err := server.EvaluateAction(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if verdict.Verdict != policy.EffectAllow {
			t.Errorf("expected allow verdict, got %s", verdict.Verdict)
		}
		if verdict.PolicyName != "allow-all" {
			t.Errorf("expected policy name 'allow-all', got %s", verdict.PolicyName)
		}
		if verdict.TraceId == "" {
			t.Error("trace ID should be set")
		}

		// Verify policy was called
		calls := policyEngine.getCalls()
		if len(calls) != 1 {
			t.Errorf("expected 1 policy call, got %d", len(calls))
		}

		// Verify trace was recorded (async, need small wait)
		time.Sleep(50 * time.Millisecond)
		traces := store.getTraces()
		if len(traces) != 1 {
			t.Errorf("expected 1 trace, got %d", len(traces))
		}
		if len(traces) > 0 && traces[0].Status != trace.StatusAllowed {
			t.Errorf("expected allowed status, got %s", traces[0].Status)
		}

		// Verify detection was called (async)
		events := detection.getEvents()
		if len(events) != 1 {
			t.Errorf("expected 1 detection event, got %d", len(events))
		}
	})

	t.Run("deny verdict triggers alert", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{
			result: policy.PolicyResult{
				Effect:     policy.EffectDeny,
				PolicyName: "deny-dangerous",
				Message:    "dangerous action blocked",
			},
		}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)
		detection := &mockDetectionEngine{}
		alerts := &mockAlertManager{}

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, detection, alerts, nil)

		req := &pb.ActionEvent{
			SessionId: "test-session",
			AgentId:   "test-agent",
			Action: &pb.Action{
				Type:   "tool.call",
				Name:   "delete_database",
				Target: "production",
			},
		}

		verdict, err := server.EvaluateAction(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if verdict.Verdict != policy.EffectDeny {
			t.Errorf("expected deny verdict, got %s", verdict.Verdict)
		}

		// Verify trace was recorded with denied status (async)
		time.Sleep(50 * time.Millisecond)
		traces := store.getTraces()
		if len(traces) > 0 && traces[0].Status != trace.StatusDenied {
			t.Errorf("expected denied status, got %s", traces[0].Status)
		}

		// Verify alert was sent (async)
		alertsSent := alerts.getAlerts()
		if len(alertsSent) != 1 {
			t.Fatalf("expected 1 alert, got %d", len(alertsSent))
		}
		if alertsSent[0].Type != "policy_violation" {
			t.Errorf("expected policy_violation alert type, got %s", alertsSent[0].Type)
		}
		if alertsSent[0].Severity != "warning" {
			t.Errorf("expected warning severity, got %s", alertsSent[0].Severity)
		}
	})

	t.Run("terminate verdict triggers critical alert", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{
			result: policy.PolicyResult{
				Effect:     policy.EffectTerminate,
				PolicyName: "kill-switch",
				Message:    "critical violation - terminating session",
			},
		}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)
		detection := &mockDetectionEngine{}
		alerts := &mockAlertManager{}

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, detection, alerts, nil)

		req := &pb.ActionEvent{
			SessionId: "test-session",
			AgentId:   "test-agent",
			Action: &pb.Action{
				Type:   "tool.call",
				Name:   "exfiltrate_data",
				Target: "all",
			},
		}

		verdict, err := server.EvaluateAction(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if verdict.Verdict != policy.EffectTerminate {
			t.Errorf("expected terminate verdict, got %s", verdict.Verdict)
		}

		// Verify alert was sent with critical severity (async)
		time.Sleep(50 * time.Millisecond)
		alertsSent := alerts.getAlerts()
		if len(alertsSent) != 1 {
			t.Fatalf("expected 1 alert, got %d", len(alertsSent))
		}
		if alertsSent[0].Severity != "critical" {
			t.Errorf("expected critical severity, got %s", alertsSent[0].Severity)
		}
	})

	t.Run("approve verdict returns approval ID", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{
			result: policy.PolicyResult{
				Effect:     policy.EffectApprove,
				PolicyName: "require-approval",
				Message:    "human approval required",
			},
		}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)
		detection := &mockDetectionEngine{}
		alerts := &mockAlertManager{}

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, detection, alerts, nil)

		req := &pb.ActionEvent{
			SessionId: "test-session",
			AgentId:   "test-agent",
			Action: &pb.Action{
				Type:   "tool.call",
				Name:   "send_email",
				Target: "ceo@company.com",
			},
		}

		verdict, err := server.EvaluateAction(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if verdict.Verdict != policy.EffectApprove {
			t.Errorf("expected approve verdict, got %s", verdict.Verdict)
		}
		if verdict.ApprovalId == "" {
			t.Error("approval ID should be set")
		}
		if verdict.TimeoutSeconds == 0 {
			t.Error("timeout seconds should be set")
		}

		// Verify trace was recorded with pending status
		time.Sleep(50 * time.Millisecond)
		traces := store.getTraces()
		if len(traces) > 0 && traces[0].Status != trace.StatusPending {
			t.Errorf("expected pending status, got %s", traces[0].Status)
		}
	})

	t.Run("throttle verdict recorded correctly", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{
			result: policy.PolicyResult{
				Effect:     policy.EffectThrottle,
				PolicyName: "rate-limit",
				Message:    "rate limit exceeded",
			},
		}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, nil, nil, nil)

		req := &pb.ActionEvent{
			SessionId: "test-session",
			AgentId:   "test-agent",
			Action: &pb.Action{
				Type: "tool.call",
				Name: "api_call",
			},
		}

		verdict, err := server.EvaluateAction(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if verdict.Verdict != policy.EffectThrottle {
			t.Errorf("expected throttle verdict, got %s", verdict.Verdict)
		}

		// Verify trace was recorded with throttled status
		time.Sleep(50 * time.Millisecond)
		traces := store.getTraces()
		if len(traces) > 0 && traces[0].Status != trace.StatusThrottled {
			t.Errorf("expected throttled status, got %s", traces[0].Status)
		}
	})

	t.Run("missing action returns error", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, nil, nil, nil)

		req := &pb.ActionEvent{
			SessionId: "test-session",
			AgentId:   "test-agent",
			Action:    nil, // missing action
		}

		_, err := server.EvaluateAction(context.Background(), req)

		if err == nil {
			t.Fatal("expected error for missing action, got nil")
		}
		if err.Error() != "action is required" {
			t.Errorf("expected 'action is required' error, got %v", err)
		}
	})

	t.Run("builds policy context correctly", func(t *testing.T) {
		policyEngine := &mockPolicyEngine{
			result: policy.PolicyResult{
				Effect: policy.EffectAllow,
			},
		}
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)

		server := NewGRPCServer(policyEngine, store, sessions, costTracker, nil, nil, nil)

		paramsJSON := `{"key": "value", "count": 42}`
		req := &pb.ActionEvent{
			SessionId: "test-session",
			AgentId:   "test-agent",
			Action: &pb.Action{
				Type:       "tool.call",
				Name:       "search",
				Target:     "google",
				ParamsJson: paramsJSON,
			},
			Context: &pb.ActionContext{
				SessionCost:        1.25,
				SessionActionCount: 10,
			},
		}

		_, err := server.EvaluateAction(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify policy context was built correctly
		calls := policyEngine.getCalls()
		if len(calls) != 1 {
			t.Fatalf("expected 1 policy call, got %d", len(calls))
		}

		ctx := calls[0]
		if ctx.Action.Type != "tool.call" {
			t.Errorf("expected action type 'tool.call', got %s", ctx.Action.Type)
		}
		if ctx.Session.Cost != 1.25 {
			t.Errorf("expected session cost 1.25, got %f", ctx.Session.Cost)
		}
		if ctx.Session.ActionCount != 10 {
			t.Errorf("expected action count 10, got %d", ctx.Session.ActionCount)
		}
		if ctx.Action.Params["key"] != "value" {
			t.Errorf("expected params to be parsed, got %v", ctx.Action.Params)
		}
	})
}

func TestStartSession(t *testing.T) {
	t.Run("successful session start", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)

		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.SessionStart{
			SessionId: "test-session-1",
			AgentId:   "test-agent",
			Metadata:  map[string]string{"user": "alice", "task": "search"},
		}

		ack, err := server.StartSession(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !ack.Ok {
			t.Errorf("expected ok=true, got false with message: %s", ack.Message)
		}
		if ack.SessionId != "test-session-1" {
			t.Errorf("expected session ID 'test-session-1', got %s", ack.SessionId)
		}

		// Verify session was created
		sess := sessions.Get("test-session-1")
		if sess == nil {
			t.Fatal("session should exist in manager")
		}
		if sess.AgentID != "test-agent" {
			t.Errorf("expected agent ID 'test-agent', got %s", sess.AgentID)
		}
	})

	t.Run("generated session ID when not provided", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)

		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.SessionStart{
			SessionId: "", // empty session ID
			AgentId:   "test-agent",
		}

		ack, err := server.StartSession(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !ack.Ok {
			t.Errorf("expected ok=true, got false")
		}
		if ack.SessionId == "" {
			t.Error("session ID should be generated")
		}

		// Verify session was created with generated ID
		sess := sessions.Get(ack.SessionId)
		if sess == nil {
			t.Fatal("session should exist in manager")
		}
	})

	t.Run("metadata serialization", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)

		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.SessionStart{
			SessionId: "test-session-meta",
			AgentId:   "test-agent",
			Metadata: map[string]string{
				"environment": "production",
				"version":     "1.0.0",
			},
		}

		ack, err := server.StartSession(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !ack.Ok {
			t.Errorf("expected ok=true, got false")
		}

		// Verify metadata was serialized and stored
		sess := sessions.Get("test-session-meta")
		if sess == nil {
			t.Fatal("session should exist")
		}

		var metadata map[string]string
		if err := json.Unmarshal(sess.Metadata, &metadata); err != nil {
			t.Fatalf("failed to unmarshal metadata: %v", err)
		}
		if metadata["environment"] != "production" {
			t.Errorf("expected environment 'production', got %s", metadata["environment"])
		}
	})
}

func TestEndSession(t *testing.T) {
	t.Run("successful session end", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)

		server := NewGRPCServer(nil, store, sessions, costTracker, nil, nil, nil)

		// Create a session first
		sess, _ := sessions.GetOrCreate("test-agent", "test-session-end", []byte("{}"))
		_ = sessions.AddCost("test-session-end", 0.5)
		_ = sessions.IncrementActions("test-session-end", trace.ActionToolCall)
		_ = sessions.IncrementActions("test-session-end", trace.ActionToolCall)

		// Wait a bit to ensure duration > 0
		time.Sleep(1100 * time.Millisecond)

		req := &pb.SessionEnd{
			SessionId: "test-session-end",
		}

		summary, err := server.EndSession(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if summary.SessionId != "test-session-end" {
			t.Errorf("expected session ID 'test-session-end', got %s", summary.SessionId)
		}
		if summary.TotalCost != 0.5 {
			t.Errorf("expected total cost 0.5, got %f", summary.TotalCost)
		}
		if summary.ActionCount != 2 {
			t.Errorf("expected action count 2, got %d", summary.ActionCount)
		}
		if summary.Status != "completed" {
			t.Errorf("expected status 'completed', got %s", summary.Status)
		}
		if summary.DurationSeconds == 0 {
			t.Error("duration should be > 0")
		}

		// Verify session no longer exists in manager
		if sessions.Get("test-session-end") != nil {
			t.Error("session should be removed from manager after end")
		}

		// Verify session was persisted with completed status
		if store.sessions[sess.ID].Status != session.StatusCompleted {
			t.Errorf("expected completed status in store, got %s", store.sessions[sess.ID].Status)
		}
	})

	t.Run("non-existent session returns error", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		costTracker := cost.NewTracker(nil)

		server := NewGRPCServer(nil, store, sessions, costTracker, nil, nil, nil)

		req := &pb.SessionEnd{
			SessionId: "non-existent-session",
		}

		_, err := server.EndSession(context.Background(), req)

		if err == nil {
			t.Fatal("expected error for non-existent session, got nil")
		}
		if err.Error() != "session non-existent-session not found" {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})
}

func TestScoreSession(t *testing.T) {
	t.Run("successful session scoring", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)

		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.SessionScore{
			SessionId:     "test-session-score",
			TaskCompleted: true,
			Quality:       0.85,
			Metrics: map[string]string{
				"accuracy": "95%",
				"latency":  "200ms",
			},
		}

		ack, err := server.ScoreSession(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if !ack.Ok {
			t.Errorf("expected ok=true, got false with message: %s", ack.Message)
		}

		// Verify score was stored
		scores := store.getScores()
		scoreJSON, exists := scores["test-session-score"]
		if !exists {
			t.Fatal("score should be stored")
		}

		var scoreData map[string]interface{}
		if err := json.Unmarshal(scoreJSON, &scoreData); err != nil {
			t.Fatalf("failed to unmarshal score: %v", err)
		}
		if scoreData["task_completed"] != true {
			t.Errorf("expected task_completed=true, got %v", scoreData["task_completed"])
		}
		if scoreData["quality"] != 0.85 {
			t.Errorf("expected quality=0.85, got %v", scoreData["quality"])
		}
	})

	t.Run("store error returns failure ack", func(t *testing.T) {
		store := newMockStore()
		store.scoreErr = errors.New("database error")
		sessions := session.NewManager(store, nil)

		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.SessionScore{
			SessionId:     "test-session-error",
			TaskCompleted: true,
			Quality:       0.9,
		}

		ack, err := server.ScoreSession(context.Background(), req)

		if err != nil {
			t.Fatalf("expected no error (should return ack), got %v", err)
		}
		if ack.Ok {
			t.Error("expected ok=false for store error")
		}
		if ack.Message != "database error" {
			t.Errorf("expected error message, got %s", ack.Message)
		}
	})
}

func TestBuildPolicyContext(t *testing.T) {
	t.Run("complete context", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.ActionEvent{
			SessionId: "sess-123",
			AgentId:   "agent-456",
			Action: &pb.Action{
				Type:       "tool.call",
				Name:       "search",
				Target:     "google",
				ParamsJson: `{"query": "test", "limit": 10}`,
			},
			Context: &pb.ActionContext{
				SessionCost:        2.5,
				SessionActionCount: 15,
			},
		}

		ctx := server.buildPolicyContext(req)

		if ctx.Action.Type != "tool.call" {
			t.Errorf("expected action type 'tool.call', got %s", ctx.Action.Type)
		}
		if ctx.Action.Name != "search" {
			t.Errorf("expected action name 'search', got %s", ctx.Action.Name)
		}
		if ctx.Action.Target != "google" {
			t.Errorf("expected target 'google', got %s", ctx.Action.Target)
		}
		if ctx.Session.ID != "sess-123" {
			t.Errorf("expected session ID 'sess-123', got %s", ctx.Session.ID)
		}
		if ctx.Session.AgentID != "agent-456" {
			t.Errorf("expected agent ID 'agent-456', got %s", ctx.Session.AgentID)
		}
		if ctx.Session.Cost != 2.5 {
			t.Errorf("expected cost 2.5, got %f", ctx.Session.Cost)
		}
		if ctx.Session.ActionCount != 15 {
			t.Errorf("expected action count 15, got %d", ctx.Session.ActionCount)
		}
		if ctx.Action.Params["query"] != "test" {
			t.Errorf("expected query param 'test', got %v", ctx.Action.Params)
		}
		if ctx.Action.Params["limit"] != float64(10) {
			t.Errorf("expected limit param 10, got %v", ctx.Action.Params)
		}
	})

	t.Run("missing context fields", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.ActionEvent{
			SessionId: "sess-123",
			AgentId:   "agent-456",
			Action: &pb.Action{
				Type: "tool.call",
				Name: "search",
			},
			Context: nil, // no context
		}

		ctx := server.buildPolicyContext(req)

		// Should have zero values for missing context
		if ctx.Session.Cost != 0 {
			t.Errorf("expected zero cost, got %f", ctx.Session.Cost)
		}
		if ctx.Session.ActionCount != 0 {
			t.Errorf("expected zero action count, got %d", ctx.Session.ActionCount)
		}
	})

	t.Run("invalid params JSON", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		server := NewGRPCServer(nil, store, sessions, nil, nil, nil, nil)

		req := &pb.ActionEvent{
			SessionId: "sess-123",
			AgentId:   "agent-456",
			Action: &pb.Action{
				Type:       "tool.call",
				Name:       "search",
				ParamsJson: `{invalid json}`, // malformed JSON
			},
		}

		ctx := server.buildPolicyContext(req)

		// Should have empty params map for invalid JSON
		if len(ctx.Action.Params) != 0 {
			t.Errorf("expected empty params for invalid JSON, got %v", ctx.Action.Params)
		}
	})
}

func TestRecordTrace(t *testing.T) {
	// recordTrace is called asynchronously in EvaluateAction, so we test it indirectly
	t.Run("trace recorded with correct status", func(t *testing.T) {
		store := newMockStore()
		sessions := session.NewManager(store, nil)
		policyEngine := &mockPolicyEngine{
			result: policy.PolicyResult{
				Effect:     policy.EffectDeny,
				PolicyName: "test-policy",
				Message:    "test reason",
			},
		}

		server := NewGRPCServer(policyEngine, store, sessions, nil, nil, nil, nil)

		req := &pb.ActionEvent{
			SessionId: "sess-123",
			AgentId:   "agent-456",
			Action: &pb.Action{
				Type:   "tool.call",
				Name:   "test",
				Target: "target",
			},
			Metadata: map[string]string{"key": "value"},
		}

		_, err := server.EvaluateAction(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Wait for async trace recording
		time.Sleep(50 * time.Millisecond)

		traces := store.getTraces()
		if len(traces) != 1 {
			t.Fatalf("expected 1 trace, got %d", len(traces))
		}

		tr := traces[0]
		if tr.SessionID != "sess-123" {
			t.Errorf("expected session ID 'sess-123', got %s", tr.SessionID)
		}
		if tr.AgentID != "agent-456" {
			t.Errorf("expected agent ID 'agent-456', got %s", tr.AgentID)
		}
		if tr.ActionType != trace.ActionToolCall {
			t.Errorf("expected action type %s, got %s", trace.ActionToolCall, tr.ActionType)
		}
		if tr.ActionName != "test" {
			t.Errorf("expected action name 'test', got %s", tr.ActionName)
		}
		if tr.Status != trace.StatusDenied {
			t.Errorf("expected denied status, got %s", tr.Status)
		}
		if tr.PolicyName != "test-policy" {
			t.Errorf("expected policy name 'test-policy', got %s", tr.PolicyName)
		}
		if tr.PolicyReason != "test reason" {
			t.Errorf("expected reason 'test reason', got %s", tr.PolicyReason)
		}
	})
}
