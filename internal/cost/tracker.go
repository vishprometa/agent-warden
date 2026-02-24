package cost

import (
	"log/slog"
	"sync"
)

// Tracker accumulates cost per session and per agent.
type Tracker struct {
	mu            sync.RWMutex
	sessionCosts  map[string]float64 // sessionID → total USD
	agentCosts    map[string]float64 // agentID → total USD
	tokenCounter  *TokenCounter
	logger        *slog.Logger
}

// NewTracker creates a new cost tracker.
func NewTracker(logger *slog.Logger) *Tracker {
	return &Tracker{
		sessionCosts: make(map[string]float64),
		agentCosts:   make(map[string]float64),
		tokenCounter: NewTokenCounter(),
		logger:       logger,
	}
}

// RecordUsage records token usage and calculates cost for a request.
func (t *Tracker) RecordUsage(sessionID, agentID, model string, inputTokens, outputTokens int) float64 {
	cost := CalculateCost(model, inputTokens, outputTokens)

	t.mu.Lock()
	t.sessionCosts[sessionID] += cost
	t.agentCosts[agentID] += cost
	t.mu.Unlock()

	t.logger.Debug("cost recorded",
		"session_id", sessionID,
		"agent_id", agentID,
		"model", model,
		"input_tokens", inputTokens,
		"output_tokens", outputTokens,
		"cost_usd", cost,
		"session_total", t.GetSessionCost(sessionID),
	)

	return cost
}

// GetSessionCost returns the accumulated cost for a session.
func (t *Tracker) GetSessionCost(sessionID string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.sessionCosts[sessionID]
}

// GetAgentCost returns the accumulated cost for an agent.
func (t *Tracker) GetAgentCost(agentID string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.agentCosts[agentID]
}

// ResetSession clears cost tracking for a completed session.
func (t *Tracker) ResetSession(sessionID string) {
	t.mu.Lock()
	delete(t.sessionCosts, sessionID)
	t.mu.Unlock()
}

// TokenCounter returns the token counter instance.
func (t *Tracker) TokenCounter() *TokenCounter {
	return t.tokenCounter
}
