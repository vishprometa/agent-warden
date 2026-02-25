package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/alert"
	"github.com/agentwarden/agentwarden/internal/config"
	"github.com/agentwarden/agentwarden/internal/trace"
)

// mockStore implements trace.Store for testing
type mockStore struct {
	mu         sync.RWMutex
	approvals  map[string]*trace.Approval
	insertErr  error
	resolveErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		approvals: make(map[string]*trace.Approval),
	}
}

func (m *mockStore) InsertApproval(a *trace.Approval) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.approvals[a.ID] = a
	return nil
}

func (m *mockStore) ResolveApproval(id, status, resolvedBy string) error {
	if m.resolveErr != nil {
		return m.resolveErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.approvals[id]; ok {
		a.Status = status
		a.ResolvedBy = resolvedBy
		now := time.Now()
		a.ResolvedAt = &now
	}
	return nil
}

func (m *mockStore) getApproval(id string) *trace.Approval {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.approvals[id]
}

// Stub methods for unused Store interface methods
func (m *mockStore) Initialize() error                                           { return nil }
func (m *mockStore) Close() error                                                { return nil }
func (m *mockStore) InsertTrace(t *trace.Trace) error                            { return nil }
func (m *mockStore) GetTrace(id string) (*trace.Trace, error)                    { return nil, nil }
func (m *mockStore) ListTraces(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
	return nil, 0, nil
}
func (m *mockStore) SearchTraces(query string, limit int) ([]*trace.Trace, error) { return nil, nil }
func (m *mockStore) UpsertSession(s *trace.Session) error                           { return nil }
func (m *mockStore) GetSession(id string) (*trace.Session, error)                   { return nil, nil }
func (m *mockStore) ListSessions(filter trace.SessionFilter) ([]*trace.Session, int, error) {
	return nil, 0, nil
}
func (m *mockStore) UpdateSessionStatus(id, status string) error                   { return nil }
func (m *mockStore) UpdateSessionCost(id string, cost float64, actionCount int) error { return nil }
func (m *mockStore) ScoreSession(id string, score []byte) error                      { return nil }
func (m *mockStore) GetApproval(id string) (*trace.Approval, error) { return nil, nil }
func (m *mockStore) ListPendingApprovals() ([]*trace.Approval, error) { return nil, nil }
func (m *mockStore) InsertViolation(v *trace.Violation) error { return nil }
func (m *mockStore) ListViolations(agentID string, limit int) ([]*trace.Violation, error) { return nil, nil }
func (m *mockStore) UpsertAgent(a *trace.Agent) error { return nil }
func (m *mockStore) GetAgent(id string) (*trace.Agent, error) { return nil, nil }
func (m *mockStore) ListAgents() ([]*trace.Agent, error) { return nil, nil }
func (m *mockStore) GetAgentStats(agentID string) (*trace.AgentStats, error) { return nil, nil }
func (m *mockStore) InsertAgentVersion(v *trace.AgentVersion) error { return nil }
func (m *mockStore) GetAgentVersion(id string) (*trace.AgentVersion, error) { return nil, nil }
func (m *mockStore) ListAgentVersions(agentID string) ([]*trace.AgentVersion, error) { return nil, nil }
func (m *mockStore) PruneOlderThan(days int) (int64, error) { return 0, nil }
func (m *mockStore) VerifyHashChain(sessionID string) (bool, int, error) { return true, 0, nil }
func (m *mockStore) GetSystemStats() (*trace.SystemStats, error) { return nil, nil }

// newTestAlertManager creates an alert.Manager with no configured senders
// (alerts will be logged but not actually sent anywhere)
func newTestAlertManager() *alert.Manager {
	// Create alert.Manager with empty config (no actual senders configured)
	return alert.NewManager(config.AlertsConfig{}, testLogger())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewQueue(t *testing.T) {
	store := newMockStore()
	alertMgr := newTestAlertManager()
	logger := testLogger()

	q := NewQueue(store, alertMgr, logger)
	if q == nil {
		t.Fatal("NewQueue returned nil")
	}
	if q.pending == nil {
		t.Error("pending map not initialized")
	}
}

func TestSubmitAndResolve_Approved(t *testing.T) {
	store := newMockStore()
	alertMgr := newTestAlertManager()
	logger := testLogger()
	q := NewQueue(store, alertMgr, logger)

	req := &Request{
		ID:         "approval-1",
		SessionID:  "session-1",
		TraceID:    "trace-1",
		PolicyName: "require-approval",
		ActionSummary: map[string]interface{}{
			"action": "delete_database",
			"target": "production",
		},
		Timeout:       5 * time.Second,
		TimeoutEffect: "deny",
	}

	ctx := context.Background()
	var approved bool
	var submitErr error

	// Submit in goroutine (blocks until resolved)
	done := make(chan struct{})
	go func() {
		approved, submitErr = q.Submit(ctx, req)
		close(done)
	}()

	// Give Submit time to queue the request
	time.Sleep(100 * time.Millisecond)

	// Verify request is pending
	pending := q.ListPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}
	if pending[0].ID != "approval-1" {
		t.Errorf("expected approval-1, got %s", pending[0].ID)
	}

	// Resolve as approved
	err := q.Resolve("approval-1", true, "admin@example.com")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// Wait for Submit to return
	<-done

	if submitErr != nil {
		t.Errorf("Submit returned error: %v", submitErr)
	}
	if !approved {
		t.Error("expected approved=true, got false")
	}

	// Verify no longer pending
	pending = q.ListPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after resolve, got %d", len(pending))
	}

	// Verify persistence
	approval := store.getApproval("approval-1")
	if approval == nil {
		t.Fatal("approval not persisted to store")
	}
	if approval.Status != "approved" {
		t.Errorf("expected status=approved, got %s", approval.Status)
	}
	if approval.ResolvedBy != "admin@example.com" {
		t.Errorf("expected ResolvedBy=admin@example.com, got %s", approval.ResolvedBy)
	}

	// Note: Alert verification is skipped in this test since alert.Manager
	// doesn't expose sent alerts for testing. In production, alerts would
	// be sent to configured channels (Slack, webhook, etc.)
}

func TestSubmitAndResolve_Denied(t *testing.T) {
	store := newMockStore()
	alertMgr := newTestAlertManager()
	logger := testLogger()
	q := NewQueue(store, alertMgr, logger)

	req := &Request{
		ID:            "approval-2",
		SessionID:     "session-2",
		TraceID:       "trace-2",
		PolicyName:    "require-approval",
		ActionSummary: map[string]interface{}{"action": "risky_operation"},
		Timeout:       5 * time.Second,
		TimeoutEffect: "deny",
	}

	ctx := context.Background()
	var approved bool
	var submitErr error

	done := make(chan struct{})
	go func() {
		approved, submitErr = q.Submit(ctx, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	// Resolve as denied
	err := q.Resolve("approval-2", false, "admin@example.com")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	<-done

	if submitErr != nil {
		t.Errorf("Submit returned error: %v", submitErr)
	}
	if approved {
		t.Error("expected approved=false, got true")
	}

	// Verify persistence
	approval := store.getApproval("approval-2")
	if approval == nil {
		t.Fatal("approval not persisted to store")
	}
	if approval.Status != "denied" {
		t.Errorf("expected status=denied, got %s", approval.Status)
	}
}

func TestResolve_NotFound(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	err := q.Resolve("nonexistent-approval", true, "admin")
	if err == nil {
		t.Fatal("expected error for non-existent approval, got nil")
	}
	expectedMsg := "approval nonexistent-approval not found or already resolved"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestResolve_AlreadyResolved(t *testing.T) {
	store := newMockStore()
	alertMgr := newTestAlertManager()
	logger := testLogger()
	q := NewQueue(store, alertMgr, logger)

	req := &Request{
		ID:            "approval-3",
		SessionID:     "session-3",
		TraceID:       "trace-3",
		PolicyName:    "test-policy",
		ActionSummary: map[string]interface{}{"test": true},
		Timeout:       5 * time.Second,
		TimeoutEffect: "deny",
	}

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		_, _ = q.Submit(ctx, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	// Resolve once
	err := q.Resolve("approval-3", true, "admin")
	if err != nil {
		t.Fatalf("first Resolve failed: %v", err)
	}

	<-done

	// Try to resolve again
	err = q.Resolve("approval-3", false, "admin")
	if err == nil {
		t.Fatal("expected error when resolving already-resolved approval, got nil")
	}
}

func TestSubmit_Timeout_DenyEffect(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	req := &Request{
		ID:            "approval-timeout-1",
		SessionID:     "session-timeout",
		TraceID:       "trace-timeout",
		PolicyName:    "test-policy",
		ActionSummary: map[string]interface{}{"test": "timeout"},
		Timeout:       500 * time.Millisecond, // Short timeout for test
		TimeoutEffect: "deny",
	}

	ctx := context.Background()
	approved, err := q.Submit(ctx, req)
	if err != nil {
		t.Errorf("Submit returned error: %v", err)
	}
	if approved {
		t.Error("expected approved=false on timeout with deny effect, got true")
	}

	// Verify persistence (timed_out status)
	approval := store.getApproval("approval-timeout-1")
	if approval == nil {
		t.Fatal("approval not persisted to store")
	}
	if approval.Status != "timed_out" {
		t.Errorf("expected status=timed_out, got %s", approval.Status)
	}
	if approval.ResolvedBy != "timeout" {
		t.Errorf("expected ResolvedBy=timeout, got %s", approval.ResolvedBy)
	}
}

func TestSubmit_Timeout_AllowEffect(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	req := &Request{
		ID:            "approval-timeout-2",
		SessionID:     "session-timeout-allow",
		TraceID:       "trace-timeout-allow",
		PolicyName:    "test-policy",
		ActionSummary: map[string]interface{}{"test": "timeout"},
		Timeout:       500 * time.Millisecond,
		TimeoutEffect: "allow", // Allow on timeout
	}

	ctx := context.Background()
	approved, err := q.Submit(ctx, req)
	if err != nil {
		t.Errorf("Submit returned error: %v", err)
	}
	if !approved {
		t.Error("expected approved=true on timeout with allow effect, got false")
	}

	// Verify persistence
	approval := store.getApproval("approval-timeout-2")
	if approval == nil {
		t.Fatal("approval not persisted to store")
	}
	if approval.Status != "timed_out" {
		t.Errorf("expected status=timed_out, got %s", approval.Status)
	}
}

func TestSubmit_ContextCancelled(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	req := &Request{
		ID:            "approval-cancelled",
		SessionID:     "session-cancelled",
		TraceID:       "trace-cancelled",
		PolicyName:    "test-policy",
		ActionSummary: map[string]interface{}{"test": "cancel"},
		Timeout:       10 * time.Second,
		TimeoutEffect: "deny",
	}

	ctx, cancel := context.WithCancel(context.Background())

	var approved bool
	var submitErr error
	done := make(chan struct{})
	go func() {
		approved, submitErr = q.Submit(ctx, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	<-done

	if submitErr != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", submitErr)
	}
	if approved {
		t.Error("expected approved=false on context cancel, got true")
	}

	// Verify request was cleaned up
	pending := q.ListPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after cancel, got %d", len(pending))
	}
}

func TestListPending_Empty(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	pending := q.ListPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests, got %d", len(pending))
	}
}

func TestListPending_Multiple(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	// Submit 3 requests without resolving
	for i := 1; i <= 3; i++ {
		req := &Request{
			ID:            fmt.Sprintf("approval-%d", i),
			SessionID:     fmt.Sprintf("session-%d", i),
			TraceID:       fmt.Sprintf("trace-%d", i),
			PolicyName:    "test-policy",
			ActionSummary: map[string]interface{}{"index": i},
			Timeout:       10 * time.Second,
			TimeoutEffect: "deny",
		}

		go func() {
			_, _ = q.Submit(context.Background(), req)
		}()
	}

	time.Sleep(200 * time.Millisecond)

	pending := q.ListPending()
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending requests, got %d", len(pending))
	}

	// Verify all IDs are present
	ids := make(map[string]bool)
	for _, req := range pending {
		ids[req.ID] = true
	}
	for i := 1; i <= 3; i++ {
		expectedID := fmt.Sprintf("approval-%d", i)
		if !ids[expectedID] {
			t.Errorf("expected pending request %s not found", expectedID)
		}
	}
}

func TestSubmit_StoreInsertError(t *testing.T) {
	store := newMockStore()
	store.insertErr = fmt.Errorf("database connection lost")
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	req := &Request{
		ID:            "approval-error",
		SessionID:     "session-error",
		TraceID:       "trace-error",
		PolicyName:    "test-policy",
		ActionSummary: map[string]interface{}{"test": "error"},
		Timeout:       5 * time.Second,
		TimeoutEffect: "deny",
	}

	ctx := context.Background()
	_, err := q.Submit(ctx, req)
	if err == nil {
		t.Fatal("expected error from Submit when store fails, got nil")
	}
	if err.Error() != "failed to persist approval: database connection lost" {
		t.Errorf("unexpected error message: %v", err)
	}

	// Verify request was not queued
	pending := q.ListPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after insert error, got %d", len(pending))
	}
}

func TestSubmit_ActionSummaryPersistence(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	actionSummary := map[string]interface{}{
		"tool":   "delete_user",
		"userID": 12345,
		"reason": "account violation",
	}

	req := &Request{
		ID:            "approval-summary",
		SessionID:     "session-summary",
		TraceID:       "trace-summary",
		PolicyName:    "test-policy",
		ActionSummary: actionSummary,
		Timeout:       5 * time.Second,
		TimeoutEffect: "deny",
	}

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		_, _ = q.Submit(ctx, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	// Resolve to unblock
	_ = q.Resolve("approval-summary", true, "admin")
	<-done

	// Verify ActionSummary was serialized and stored
	approval := store.getApproval("approval-summary")
	if approval == nil {
		t.Fatal("approval not persisted to store")
	}
	if len(approval.ActionSummary) == 0 {
		t.Fatal("ActionSummary not persisted")
	}

	var storedSummary map[string]interface{}
	err := json.Unmarshal(approval.ActionSummary, &storedSummary)
	if err != nil {
		t.Fatalf("failed to unmarshal ActionSummary: %v", err)
	}

	if storedSummary["tool"] != "delete_user" {
		t.Errorf("expected tool=delete_user, got %v", storedSummary["tool"])
	}
	if storedSummary["userID"].(float64) != 12345 {
		t.Errorf("expected userID=12345, got %v", storedSummary["userID"])
	}
}

func TestConcurrentSubmissions(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	const numRequests = 20
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(index int) {
			defer wg.Done()
			req := &Request{
				ID:            fmt.Sprintf("approval-concurrent-%d", index),
				SessionID:     fmt.Sprintf("session-%d", index),
				TraceID:       fmt.Sprintf("trace-%d", index),
				PolicyName:    "test-policy",
				ActionSummary: map[string]interface{}{"index": index},
				Timeout:       10 * time.Second,
				TimeoutEffect: "deny",
			}

			go func() {
				_, _ = q.Submit(context.Background(), req)
			}()
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	pending := q.ListPending()
	if len(pending) != numRequests {
		t.Errorf("expected %d pending requests, got %d", numRequests, len(pending))
	}

	// Verify all requests are unique
	ids := make(map[string]bool)
	for _, req := range pending {
		if ids[req.ID] {
			t.Errorf("duplicate request ID found: %s", req.ID)
		}
		ids[req.ID] = true
	}
}

func TestResolve_StoreUpdateError(t *testing.T) {
	store := newMockStore()
	logger := testLogger()
	q := NewQueue(store, nil, logger)

	req := &Request{
		ID:            "approval-resolve-error",
		SessionID:     "session-resolve-error",
		TraceID:       "trace-resolve-error",
		PolicyName:    "test-policy",
		ActionSummary: map[string]interface{}{"test": "resolve-error"},
		Timeout:       10 * time.Second,
		TimeoutEffect: "deny",
	}

	ctx := context.Background()
	var approved bool
	done := make(chan struct{})
	go func() {
		approved, _ = q.Submit(ctx, req)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)

	// Set store to return error on ResolveApproval
	store.resolveErr = fmt.Errorf("database write failed")

	// Resolve should still work (store error is logged but not returned)
	err := q.Resolve("approval-resolve-error", true, "admin")
	if err != nil {
		t.Errorf("Resolve should not fail even if store update fails: %v", err)
	}

	<-done

	// Verify in-memory state was updated despite store error
	if !approved {
		t.Error("expected approved=true, got false")
	}

	pending := q.ListPending()
	if len(pending) != 0 {
		t.Errorf("expected 0 pending requests after resolve, got %d", len(pending))
	}
}
