// Package session manages active agent sessions with in-memory state
// backed by persistent storage via the trace store.
package session

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

const (
	sessionIDPrefix = "ses_"
	sessionIDLength = 20

	// Session status constants.
	StatusActive     = "active"
	StatusCompleted  = "completed"
	StatusTerminated = "terminated"
	StatusPaused     = "paused"
)

// sessionState holds the in-memory mutable state for an active session.
// Fields are only accessed while holding the Manager's lock.
type sessionState struct {
	session *trace.Session
	paused  bool

	// actionCounts tracks action counts within sliding windows.
	// Key format: "actionType" -> list of timestamps.
	actionTimestamps map[string][]time.Time
}

// Manager tracks active sessions with thread-safe in-memory state and
// persistent storage via the trace store. It serves as the central point
// for session lifecycle management, cost accumulation, and action counting.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*sessionState
	store    trace.Store
	logger   *slog.Logger
}

// NewManager creates a new session manager backed by the given trace store.
func NewManager(store trace.Store, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		sessions: make(map[string]*sessionState),
		store:    store,
		logger:   logger.With("component", "session.Manager"),
	}
}

// GetOrCreate retrieves an existing session or creates a new one. If sessionID
// is empty, a new session ID is generated. The metadata parameter is stored
// as JSON on the session record.
func (m *Manager) GetOrCreate(agentID, sessionID string, metadata json.RawMessage) (*trace.Session, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agentID is required")
	}

	// If a specific session was requested, try to return it from memory first.
	if sessionID != "" {
		m.mu.RLock()
		if state, ok := m.sessions[sessionID]; ok {
			sess := state.session
			m.mu.RUnlock()
			return sess, nil
		}
		m.mu.RUnlock()

		// Check persistent storage for a previously persisted session.
		existing, err := m.store.GetSession(sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to look up session %s: %w", sessionID, err)
		}
		if existing != nil && existing.Status == StatusActive {
			m.mu.Lock()
			// Double-check after acquiring write lock.
			if state, ok := m.sessions[sessionID]; ok {
				m.mu.Unlock()
				return state.session, nil
			}
			m.sessions[sessionID] = &sessionState{
				session:          existing,
				paused:           false,
				actionTimestamps: make(map[string][]time.Time),
			}
			m.mu.Unlock()
			m.logger.Info("reloaded session from store", "session_id", sessionID, "agent_id", agentID)
			return existing, nil
		}
	}

	// Generate a new session ID if none was provided or the provided one doesn't exist.
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	now := time.Now().UTC()
	sess := &trace.Session{
		ID:        sessionID,
		AgentID:   agentID,
		StartedAt: now,
		Status:    StatusActive,
		Metadata:  metadata,
	}

	// Persist to store.
	if err := m.store.UpsertSession(sess); err != nil {
		return nil, fmt.Errorf("failed to persist new session: %w", err)
	}

	// Also ensure the agent record exists.
	agent := &trace.Agent{
		ID:        agentID,
		Name:      agentID,
		CreatedAt: now,
	}
	if err := m.store.UpsertAgent(agent); err != nil {
		m.logger.Warn("failed to upsert agent record", "agent_id", agentID, "error", err)
	}

	m.mu.Lock()
	m.sessions[sessionID] = &sessionState{
		session:          sess,
		paused:           false,
		actionTimestamps: make(map[string][]time.Time),
	}
	m.mu.Unlock()

	m.logger.Info("created session", "session_id", sessionID, "agent_id", agentID)
	return sess, nil
}

// Get returns the session for the given ID, or nil if not found.
func (m *Manager) Get(sessionID string) *trace.Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, ok := m.sessions[sessionID]; ok {
		return state.session
	}
	return nil
}

// End marks a session as completed and removes it from the active set.
func (m *Manager) End(sessionID string) error {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %s not found", sessionID)
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	now := time.Now().UTC()
	state.session.EndedAt = &now
	state.session.Status = StatusCompleted

	if err := m.store.UpsertSession(state.session); err != nil {
		return fmt.Errorf("failed to persist session end: %w", err)
	}

	m.logger.Info("ended session",
		"session_id", sessionID,
		"agent_id", state.session.AgentID,
		"total_cost", state.session.TotalCost,
		"action_count", state.session.ActionCount,
	)
	return nil
}

// Terminate marks a session as terminated (policy violation, anomaly, etc.)
// and removes it from the active set.
func (m *Manager) Terminate(sessionID string) error {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		// Try updating in store directly for sessions not in memory.
		return m.store.UpdateSessionStatus(sessionID, StatusTerminated)
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	now := time.Now().UTC()
	state.session.EndedAt = &now
	state.session.Status = StatusTerminated

	if err := m.store.UpsertSession(state.session); err != nil {
		return fmt.Errorf("failed to persist session termination: %w", err)
	}

	m.logger.Warn("terminated session",
		"session_id", sessionID,
		"agent_id", state.session.AgentID,
		"total_cost", state.session.TotalCost,
	)
	return nil
}

// AddCost increments the session's accumulated cost and persists the update.
func (m *Manager) AddCost(sessionID string, cost float64) error {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %s not found", sessionID)
	}
	state.session.TotalCost += cost
	totalCost := state.session.TotalCost
	actionCount := state.session.ActionCount
	m.mu.Unlock()

	// Fire-and-forget persistence; log errors but don't block the request path.
	if err := m.store.UpdateSessionCost(sessionID, totalCost, actionCount); err != nil {
		m.logger.Error("failed to persist session cost", "session_id", sessionID, "error", err)
		return err
	}
	return nil
}

// IncrementActions bumps the session action count and records the timestamp
// for the given action type (used by sliding-window rate limiting).
func (m *Manager) IncrementActions(sessionID string, actionType trace.ActionType) error {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %s not found", sessionID)
	}
	state.session.ActionCount++

	at := string(actionType)
	state.actionTimestamps[at] = append(state.actionTimestamps[at], time.Now())

	totalCost := state.session.TotalCost
	actionCount := state.session.ActionCount
	m.mu.Unlock()

	if err := m.store.UpdateSessionCost(sessionID, totalCost, actionCount); err != nil {
		m.logger.Error("failed to persist action count", "session_id", sessionID, "error", err)
		return err
	}
	return nil
}

// GetActionCount returns the number of actions of the given type that occurred
// within the specified sliding window duration.
func (m *Manager) GetActionCount(sessionID string, actionType string, window time.Duration) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.sessions[sessionID]
	if !ok {
		return 0
	}

	timestamps, ok := state.actionTimestamps[actionType]
	if !ok {
		return 0
	}

	cutoff := time.Now().Add(-window)
	count := 0
	// Prune old entries while counting. Since timestamps are appended in order,
	// we find the first index that is within the window.
	startIdx := -1
	for i, ts := range timestamps {
		if ts.After(cutoff) {
			if startIdx == -1 {
				startIdx = i
			}
			count++
		}
	}

	// Compact the slice to remove expired entries (only under write lock).
	// We're under RLock here so we defer compaction to the next write.
	return count
}

// SetPaused sets the paused state for a session. Paused sessions still
// exist but the proxy should hold or reject requests.
func (m *Manager) SetPaused(sessionID string, paused bool) error {
	m.mu.Lock()
	state, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session %s not found", sessionID)
	}
	state.paused = paused

	newStatus := StatusActive
	if paused {
		newStatus = StatusPaused
	}
	state.session.Status = newStatus
	m.mu.Unlock()

	if err := m.store.UpdateSessionStatus(sessionID, newStatus); err != nil {
		m.logger.Error("failed to persist pause state", "session_id", sessionID, "error", err)
		return err
	}

	m.logger.Info("session pause state changed", "session_id", sessionID, "paused", paused)
	return nil
}

// IsPaused returns whether the session is currently paused.
func (m *Manager) IsPaused(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, ok := m.sessions[sessionID]; ok {
		return state.paused
	}
	return false
}

// TotalCost returns the current accumulated cost for a session.
func (m *Manager) TotalCost(sessionID string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, ok := m.sessions[sessionID]; ok {
		return state.session.TotalCost
	}
	return 0
}

// ActiveCount returns the number of currently active sessions.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// generateSessionID creates a session ID with the "ses_" prefix followed
// by random alphanumeric characters.
func generateSessionID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, sessionIDLength)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails (should never happen).
		return fmt.Sprintf("%s%d", sessionIDPrefix, time.Now().UnixNano())
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return sessionIDPrefix + string(b)
}
