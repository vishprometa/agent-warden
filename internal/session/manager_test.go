package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// mockStore is a simple in-memory trace.Store for testing.
type mockStore struct {
	mu        sync.RWMutex
	sessions  map[string]*trace.Session
	agents    map[string]*trace.Agent
	failUpsert bool // simulate failures
	failUpdate bool
}

func newMockStore() *mockStore {
	return &mockStore{
		sessions: make(map[string]*trace.Session),
		agents:   make(map[string]*trace.Agent),
	}
}

func (m *mockStore) Initialize() error { return nil }
func (m *mockStore) Close() error      { return nil }

func (m *mockStore) UpsertSession(s *trace.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failUpsert {
		return fmt.Errorf("mock upsert failure")
	}
	// Make a copy to simulate persistence.
	copy := *s
	m.sessions[s.ID] = &copy
	return nil
}

func (m *mockStore) GetSession(id string) (*trace.Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, nil
	}
	// Return a copy to avoid race conditions in tests.
	copy := *s
	return &copy, nil
}

func (m *mockStore) UpdateSessionStatus(id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failUpdate {
		return fmt.Errorf("mock update failure")
	}
	s, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	s.Status = status
	return nil
}

func (m *mockStore) UpdateSessionCost(id string, cost float64, actionCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failUpdate {
		return fmt.Errorf("mock update failure")
	}
	s, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	s.TotalCost = cost
	s.ActionCount = actionCount
	return nil
}

func (m *mockStore) UpsertAgent(a *trace.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[a.ID] = a
	return nil
}

// Stub implementations for other Store methods.
func (m *mockStore) ListSessions(trace.SessionFilter) ([]*trace.Session, int, error) {
	return nil, 0, nil
}
func (m *mockStore) ScoreSession(string, []byte) error                     { return nil }
func (m *mockStore) GetAgent(string) (*trace.Agent, error)                 { return nil, nil }
func (m *mockStore) ListAgents() ([]*trace.Agent, error)                   { return nil, nil }
func (m *mockStore) GetAgentStats(string) (*trace.AgentStats, error)       { return nil, nil }
func (m *mockStore) InsertTrace(*trace.Trace) error                        { return nil }
func (m *mockStore) GetTrace(string) (*trace.Trace, error)                 { return nil, nil }
func (m *mockStore) ListTraces(trace.TraceFilter) ([]*trace.Trace, int, error) {
	return nil, 0, nil
}
func (m *mockStore) SearchTraces(string, int) ([]*trace.Trace, error) { return nil, nil }
func (m *mockStore) InsertAgentVersion(*trace.AgentVersion) error      { return nil }
func (m *mockStore) GetAgentVersion(string) (*trace.AgentVersion, error) {
	return nil, nil
}
func (m *mockStore) ListAgentVersions(string) ([]*trace.AgentVersion, error) { return nil, nil }
func (m *mockStore) InsertApproval(*trace.Approval) error                    { return nil }
func (m *mockStore) GetApproval(string) (*trace.Approval, error)             { return nil, nil }
func (m *mockStore) ListPendingApprovals() ([]*trace.Approval, error)        { return nil, nil }
func (m *mockStore) ResolveApproval(string, string, string) error            { return nil }
func (m *mockStore) InsertViolation(*trace.Violation) error                  { return nil }
func (m *mockStore) ListViolations(string, int) ([]*trace.Violation, error) { return nil, nil }
func (m *mockStore) PruneOlderThan(int) (int64, error)                       { return 0, nil }
func (m *mockStore) VerifyHashChain(string) (bool, int, error)               { return true, 0, nil }
func (m *mockStore) GetSystemStats() (*trace.SystemStats, error)             { return nil, nil }

func TestNewManager(t *testing.T) {
	store := newMockStore()
	logger := slog.Default()

	t.Run("with logger", func(t *testing.T) {
		m := NewManager(store, logger)
		if m == nil {
			t.Fatal("expected non-nil manager")
		}
		if m.store != store {
			t.Error("store not set correctly")
		}
		if m.logger == nil {
			t.Error("logger should not be nil")
		}
		if m.sessions == nil {
			t.Error("sessions map should be initialized")
		}
	})

	t.Run("without logger", func(t *testing.T) {
		m := NewManager(store, nil)
		if m == nil {
			t.Fatal("expected non-nil manager")
		}
		if m.logger == nil {
			t.Error("logger should be initialized to default")
		}
	})
}

func TestGetOrCreate(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("create new session with auto-generated ID", func(t *testing.T) {
		metadata := json.RawMessage(`{"user":"alice"}`)
		sess, err := m.GetOrCreate("agent1", "", metadata)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sess == nil {
			t.Fatal("expected session to be created")
		}
		if sess.ID == "" {
			t.Error("expected session ID to be generated")
		}
		if sess.AgentID != "agent1" {
			t.Errorf("expected agent_id=agent1, got %s", sess.AgentID)
		}
		if sess.Status != StatusActive {
			t.Errorf("expected status=active, got %s", sess.Status)
		}
		if string(sess.Metadata) != string(metadata) {
			t.Errorf("metadata mismatch")
		}

		// Verify persisted in store.
		stored, err := store.GetSession(sess.ID)
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if stored == nil {
			t.Error("session not found in store")
		}
	})

	t.Run("create new session with explicit ID", func(t *testing.T) {
		sess, err := m.GetOrCreate("agent2", "ses_explicit123", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sess.ID != "ses_explicit123" {
			t.Errorf("expected session ID=ses_explicit123, got %s", sess.ID)
		}
	})

	t.Run("get existing session from memory", func(t *testing.T) {
		// Create first.
		sess1, err := m.GetOrCreate("agent3", "ses_mem", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Retrieve again.
		sess2, err := m.GetOrCreate("agent3", "ses_mem", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should be the same in-memory object.
		if sess1.ID != sess2.ID {
			t.Errorf("expected same session, got different IDs: %s vs %s", sess1.ID, sess2.ID)
		}
	})

	t.Run("reload session from store", func(t *testing.T) {
		// Simulate a session that exists in store but not in memory.
		store.sessions["ses_store123"] = &trace.Session{
			ID:        "ses_store123",
			AgentID:   "agent4",
			StartedAt: time.Now().UTC(),
			Status:    StatusActive,
		}

		sess, err := m.GetOrCreate("agent4", "ses_store123", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sess.ID != "ses_store123" {
			t.Errorf("expected session ID=ses_store123, got %s", sess.ID)
		}

		// Should now be in memory.
		if m.Get("ses_store123") == nil {
			t.Error("session should be loaded into memory")
		}
	})

	t.Run("empty agentID returns error", func(t *testing.T) {
		_, err := m.GetOrCreate("", "", nil)
		if err == nil {
			t.Fatal("expected error for empty agentID")
		}
	})

	t.Run("store upsert failure", func(t *testing.T) {
		failStore := newMockStore()
		failStore.failUpsert = true
		failM := NewManager(failStore, slog.Default())

		_, err := failM.GetOrCreate("agent5", "", nil)
		if err == nil {
			t.Fatal("expected error when store upsert fails")
		}
	})
}

func TestGet(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("get existing session", func(t *testing.T) {
		sess, err := m.GetOrCreate("agent1", "ses_get1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := m.Get("ses_get1")
		if got == nil {
			t.Fatal("expected session to be found")
		}
		if got.ID != sess.ID {
			t.Errorf("ID mismatch: expected %s, got %s", sess.ID, got.ID)
		}
	})

	t.Run("get non-existent session", func(t *testing.T) {
		got := m.Get("ses_nonexistent")
		if got != nil {
			t.Error("expected nil for non-existent session")
		}
	})
}

func TestEnd(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("end active session", func(t *testing.T) {
		sess, err := m.GetOrCreate("agent1", "ses_end1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = m.End("ses_end1")
		if err != nil {
			t.Fatalf("unexpected error ending session: %v", err)
		}

		// Should be removed from memory.
		if m.Get("ses_end1") != nil {
			t.Error("session should be removed from memory after ending")
		}

		// Should be persisted in store with completed status.
		stored, err := store.GetSession("ses_end1")
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if stored.Status != StatusCompleted {
			t.Errorf("expected status=completed, got %s", stored.Status)
		}
		if stored.EndedAt == nil {
			t.Error("expected EndedAt to be set")
		}

		// Verify total cost and action count are persisted.
		if stored.TotalCost != sess.TotalCost {
			t.Errorf("TotalCost mismatch: expected %f, got %f", sess.TotalCost, stored.TotalCost)
		}
	})

	t.Run("end non-existent session", func(t *testing.T) {
		err := m.End("ses_nonexistent")
		if err == nil {
			t.Fatal("expected error when ending non-existent session")
		}
	})
}

func TestTerminate(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("terminate active session", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_term1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = m.Terminate("ses_term1")
		if err != nil {
			t.Fatalf("unexpected error terminating session: %v", err)
		}

		// Should be removed from memory.
		if m.Get("ses_term1") != nil {
			t.Error("session should be removed from memory after termination")
		}

		// Should be persisted in store with terminated status.
		stored, err := store.GetSession("ses_term1")
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if stored.Status != StatusTerminated {
			t.Errorf("expected status=terminated, got %s", stored.Status)
		}
		if stored.EndedAt == nil {
			t.Error("expected EndedAt to be set")
		}
	})

	t.Run("terminate session not in memory but in store", func(t *testing.T) {
		// Simulate a session in store but not in memory.
		store.sessions["ses_term_store"] = &trace.Session{
			ID:        "ses_term_store",
			AgentID:   "agent2",
			StartedAt: time.Now().UTC(),
			Status:    StatusActive,
		}

		err := m.Terminate("ses_term_store")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should update status in store.
		stored, err := store.GetSession("ses_term_store")
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if stored.Status != StatusTerminated {
			t.Errorf("expected status=terminated, got %s", stored.Status)
		}
	})
}

func TestAddCost(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("add cost to session", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_cost1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = m.AddCost("ses_cost1", 0.05)
		if err != nil {
			t.Fatalf("unexpected error adding cost: %v", err)
		}

		sess := m.Get("ses_cost1")
		if sess.TotalCost != 0.05 {
			t.Errorf("expected cost=0.05, got %f", sess.TotalCost)
		}

		// Add more cost.
		err = m.AddCost("ses_cost1", 0.03)
		if err != nil {
			t.Fatalf("unexpected error adding cost: %v", err)
		}

		sess = m.Get("ses_cost1")
		if sess.TotalCost != 0.08 {
			t.Errorf("expected cost=0.08, got %f", sess.TotalCost)
		}

		// Verify persisted.
		stored, err := store.GetSession("ses_cost1")
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if stored.TotalCost != 0.08 {
			t.Errorf("expected stored cost=0.08, got %f", stored.TotalCost)
		}
	})

	t.Run("add cost to non-existent session", func(t *testing.T) {
		err := m.AddCost("ses_nonexistent", 1.0)
		if err == nil {
			t.Fatal("expected error when adding cost to non-existent session")
		}
	})
}

func TestIncrementActions(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("increment action count", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_action1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = m.IncrementActions("ses_action1", trace.ActionLLMChat)
		if err != nil {
			t.Fatalf("unexpected error incrementing actions: %v", err)
		}

		sess := m.Get("ses_action1")
		if sess.ActionCount != 1 {
			t.Errorf("expected action count=1, got %d", sess.ActionCount)
		}

		// Increment again.
		err = m.IncrementActions("ses_action1", trace.ActionToolCall)
		if err != nil {
			t.Fatalf("unexpected error incrementing actions: %v", err)
		}

		sess = m.Get("ses_action1")
		if sess.ActionCount != 2 {
			t.Errorf("expected action count=2, got %d", sess.ActionCount)
		}

		// Verify persisted.
		stored, err := store.GetSession("ses_action1")
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if stored.ActionCount != 2 {
			t.Errorf("expected stored action count=2, got %d", stored.ActionCount)
		}
	})

	t.Run("increment actions for non-existent session", func(t *testing.T) {
		err := m.IncrementActions("ses_nonexistent", trace.ActionLLMChat)
		if err == nil {
			t.Fatal("expected error when incrementing actions for non-existent session")
		}
	})
}

func TestGetActionCount(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("get action count within window", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_window1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Add actions.
		err = m.IncrementActions("ses_window1", trace.ActionLLMChat)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		err = m.IncrementActions("ses_window1", trace.ActionLLMChat)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		err = m.IncrementActions("ses_window1", trace.ActionToolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check counts within a 1-minute window.
		chatCount := m.GetActionCount("ses_window1", string(trace.ActionLLMChat), 1*time.Minute)
		if chatCount != 2 {
			t.Errorf("expected chat count=2, got %d", chatCount)
		}

		toolCount := m.GetActionCount("ses_window1", string(trace.ActionToolCall), 1*time.Minute)
		if toolCount != 1 {
			t.Errorf("expected tool count=1, got %d", toolCount)
		}
	})

	t.Run("get action count for non-existent session", func(t *testing.T) {
		count := m.GetActionCount("ses_nonexistent", string(trace.ActionLLMChat), 1*time.Minute)
		if count != 0 {
			t.Errorf("expected count=0 for non-existent session, got %d", count)
		}
	})

	t.Run("get action count for unknown action type", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_unknown", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		count := m.GetActionCount("ses_unknown", "unknown.action", 1*time.Minute)
		if count != 0 {
			t.Errorf("expected count=0 for unknown action type, got %d", count)
		}
	})
}

func TestSetPaused(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("pause and unpause session", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_pause1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Pause session.
		err = m.SetPaused("ses_pause1", true)
		if err != nil {
			t.Fatalf("unexpected error pausing session: %v", err)
		}

		if !m.IsPaused("ses_pause1") {
			t.Error("expected session to be paused")
		}

		sess := m.Get("ses_pause1")
		if sess.Status != StatusPaused {
			t.Errorf("expected status=paused, got %s", sess.Status)
		}

		// Verify persisted.
		stored, err := store.GetSession("ses_pause1")
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if stored.Status != StatusPaused {
			t.Errorf("expected stored status=paused, got %s", stored.Status)
		}

		// Unpause session.
		err = m.SetPaused("ses_pause1", false)
		if err != nil {
			t.Fatalf("unexpected error unpausing session: %v", err)
		}

		if m.IsPaused("ses_pause1") {
			t.Error("expected session to not be paused")
		}

		sess = m.Get("ses_pause1")
		if sess.Status != StatusActive {
			t.Errorf("expected status=active, got %s", sess.Status)
		}
	})

	t.Run("pause non-existent session", func(t *testing.T) {
		err := m.SetPaused("ses_nonexistent", true)
		if err == nil {
			t.Fatal("expected error when pausing non-existent session")
		}
	})
}

func TestIsPaused(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("check paused state", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_check1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if m.IsPaused("ses_check1") {
			t.Error("new session should not be paused")
		}

		err = m.SetPaused("ses_check1", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !m.IsPaused("ses_check1") {
			t.Error("session should be paused after SetPaused(true)")
		}
	})

	t.Run("check paused for non-existent session", func(t *testing.T) {
		if m.IsPaused("ses_nonexistent") {
			t.Error("non-existent session should return false for IsPaused")
		}
	})
}

func TestTotalCost(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("get total cost", func(t *testing.T) {
		_, err := m.GetOrCreate("agent1", "ses_totalcost1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cost := m.TotalCost("ses_totalcost1")
		if cost != 0 {
			t.Errorf("expected initial cost=0, got %f", cost)
		}

		err = m.AddCost("ses_totalcost1", 0.15)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		cost = m.TotalCost("ses_totalcost1")
		if cost != 0.15 {
			t.Errorf("expected cost=0.15, got %f", cost)
		}
	})

	t.Run("get total cost for non-existent session", func(t *testing.T) {
		cost := m.TotalCost("ses_nonexistent")
		if cost != 0 {
			t.Errorf("expected cost=0 for non-existent session, got %f", cost)
		}
	})
}

func TestActiveCount(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("track active session count", func(t *testing.T) {
		if m.ActiveCount() != 0 {
			t.Errorf("expected initial count=0, got %d", m.ActiveCount())
		}

		_, err := m.GetOrCreate("agent1", "ses_active1", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.ActiveCount() != 1 {
			t.Errorf("expected count=1, got %d", m.ActiveCount())
		}

		_, err = m.GetOrCreate("agent2", "ses_active2", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.ActiveCount() != 2 {
			t.Errorf("expected count=2, got %d", m.ActiveCount())
		}

		err = m.End("ses_active1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m.ActiveCount() != 1 {
			t.Errorf("expected count=1 after ending session, got %d", m.ActiveCount())
		}
	})
}

func TestConcurrentAccess(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("concurrent GetOrCreate", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				agentID := fmt.Sprintf("agent%d", i)
				_, err := m.GetOrCreate(agentID, "", nil)
				if err != nil {
					t.Errorf("unexpected error in goroutine: %v", err)
				}
			}(i)
		}
		wg.Wait()

		if m.ActiveCount() != 10 {
			t.Errorf("expected 10 active sessions, got %d", m.ActiveCount())
		}
	})

	t.Run("concurrent AddCost and Get", func(t *testing.T) {
		_, err := m.GetOrCreate("agent_concurrent", "ses_concurrent", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = m.AddCost("ses_concurrent", 0.01)
			}()
		}

		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = m.Get("ses_concurrent")
			}()
		}

		wg.Wait()

		cost := m.TotalCost("ses_concurrent")
		expected := 1.0
		// Use a small epsilon for floating-point comparison.
		epsilon := 0.0001
		if cost < expected-epsilon || cost > expected+epsilon {
			t.Errorf("expected cost ~= 1.0 after 100 concurrent additions of 0.01, got %f", cost)
		}
	})
}

func TestMetadataHandling(t *testing.T) {
	store := newMockStore()
	m := NewManager(store, slog.Default())

	t.Run("create session with metadata", func(t *testing.T) {
		metadata := json.RawMessage(`{"env":"production","region":"us-east-1"}`)
		sess, err := m.GetOrCreate("agent1", "ses_meta1", metadata)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if string(sess.Metadata) != string(metadata) {
			t.Errorf("metadata mismatch: expected %s, got %s", string(metadata), string(sess.Metadata))
		}

		// Verify persisted.
		stored, err := store.GetSession("ses_meta1")
		if err != nil {
			t.Fatalf("failed to get session from store: %v", err)
		}
		if string(stored.Metadata) != string(metadata) {
			t.Errorf("stored metadata mismatch: expected %s, got %s", string(metadata), string(stored.Metadata))
		}
	})

	t.Run("create session with nil metadata", func(t *testing.T) {
		sess, err := m.GetOrCreate("agent2", "ses_meta2", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if sess.Metadata != nil {
			t.Errorf("expected nil metadata, got %s", string(sess.Metadata))
		}
	})
}

func TestGenerateSessionID(t *testing.T) {
	t.Run("generates unique IDs", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := generateSessionID()
			if ids[id] {
				t.Errorf("duplicate session ID generated: %s", id)
			}
			ids[id] = true

			if len(id) != len(sessionIDPrefix)+sessionIDLength {
				t.Errorf("unexpected ID length: %d", len(id))
			}
			if id[:len(sessionIDPrefix)] != sessionIDPrefix {
				t.Errorf("ID missing prefix: %s", id)
			}
		}
	})
}
