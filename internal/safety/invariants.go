// Package safety implements safety invariants that survive LLM context
// compaction. This directly addresses the Summer Yue incident where an
// OpenClaw agent forgot safety constraints after context compaction.
//
// Safety invariants are stored outside the LLM context in AgentWarden's
// own state and are enforced at the proxy level on every action. They
// cannot be lost, overridden, or ignored by the agent.
package safety

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Invariant is a safety rule that cannot be violated regardless of what
// the LLM decides. It is enforced at the proxy layer.
type Invariant struct {
	// Description is a human-readable description (e.g., "NEVER delete
	// more than 5 emails per session without approval").
	Description string `yaml:"description" json:"description"`

	// Condition is an optional CEL expression. If set, the invariant is
	// enforced programmatically. If empty, the invariant is enforced via
	// the description (for AI-judge evaluation or context re-injection).
	Condition string `yaml:"condition,omitempty" json:"condition,omitempty"`

	// Effect is what happens when the invariant is violated.
	// Defaults to "deny".
	Effect string `yaml:"effect,omitempty" json:"effect,omitempty"`

	// Enforcement is how the invariant is enforced:
	//   "proxy"  — enforced at AgentWarden via CEL (cannot be bypassed)
	//   "inject" — re-injected into LLM context on every request
	Enforcement string `yaml:"enforcement,omitempty" json:"enforcement,omitempty"`
}

// AgentInvariants holds the safety invariants for a specific agent.
type AgentInvariants struct {
	AgentID    string      `json:"agent_id"`
	Invariants []Invariant `json:"invariants"`
}

// Engine manages safety invariants for all agents. It provides two
// enforcement mechanisms:
//
//  1. Proxy enforcement (default): The invariant's CEL condition is
//     checked on every action. If the condition matches, the action is
//     blocked. This cannot be bypassed by context compaction.
//
//  2. Context injection: The invariant description is prepended to
//     every LLM request as a system-level safety reminder. This is
//     a belt-and-suspenders approach alongside proxy enforcement.
type Engine struct {
	mu         sync.RWMutex
	invariants map[string][]Invariant // agentID → invariants
	logger     *slog.Logger
}

// NewEngine creates a new safety invariants engine.
func NewEngine(logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		invariants: make(map[string][]Invariant),
		logger:     logger.With("component", "safety.Engine"),
	}
}

// SetInvariants sets the safety invariants for an agent. This replaces
// any existing invariants for that agent.
func (e *Engine) SetInvariants(agentID string, invariants []Invariant) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Apply defaults.
	for i := range invariants {
		if invariants[i].Effect == "" {
			invariants[i].Effect = "deny"
		}
		if invariants[i].Enforcement == "" {
			invariants[i].Enforcement = "proxy"
		}
	}

	e.invariants[agentID] = invariants
	e.logger.Info("safety invariants set",
		"agent_id", agentID,
		"count", len(invariants),
	)
}

// GetInvariants returns the safety invariants for an agent.
func (e *Engine) GetInvariants(agentID string) []Invariant {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.invariants[agentID]
}

// GetProxyConditions returns the CEL conditions that should be enforced
// at the proxy level for a given agent. These are compiled into policy
// rules and checked on every action.
func (e *Engine) GetProxyConditions(agentID string) []Invariant {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []Invariant
	for _, inv := range e.invariants[agentID] {
		if inv.Enforcement == "proxy" && inv.Condition != "" {
			result = append(result, inv)
		}
	}
	return result
}

// GetInjectionText returns the safety text that should be re-injected
// into the LLM context for a given agent. This is used for invariants
// with enforcement="inject".
func (e *Engine) GetInjectionText(agentID string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	invariants := e.invariants[agentID]
	if len(invariants) == 0 {
		return ""
	}

	var lines []string
	for _, inv := range invariants {
		if inv.Enforcement == "inject" || inv.Enforcement == "both" {
			lines = append(lines, fmt.Sprintf("- %s", inv.Description))
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return fmt.Sprintf("[SAFETY INVARIANTS — These rules CANNOT be overridden]\n%s", strings.Join(lines, "\n"))
}

// AllInvariants returns a snapshot of all loaded invariants across all agents.
func (e *Engine) AllInvariants() map[string][]Invariant {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string][]Invariant, len(e.invariants))
	for k, v := range e.invariants {
		copied := make([]Invariant, len(v))
		copy(copied, v)
		result[k] = copied
	}
	return result
}

// Count returns the total number of invariants across all agents.
func (e *Engine) Count() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	total := 0
	for _, v := range e.invariants {
		total += len(v)
	}
	return total
}
