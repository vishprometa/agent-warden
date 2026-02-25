// Package adapter defines the interface for integrating external agent
// frameworks (OpenClaw, etc.) with AgentWarden's governance pipeline.
// Adapters translate framework-specific events into AgentWarden's
// ActionContext and enforce verdicts back to the framework.
package adapter

import (
	"context"

	"github.com/agentwarden/agentwarden/internal/policy"
)

// Verdict is the governance decision returned to the agent framework.
type Verdict struct {
	Effect  string `json:"effect"`  // allow, deny, terminate, throttle, approve
	Message string `json:"message"` // human-readable explanation
	DelayMS int    `json:"delay_ms,omitempty"`
}

// Adapter is the interface that agent framework integrations implement.
// Each adapter translates between a framework's native protocol and
// AgentWarden's policy evaluation pipeline.
type Adapter interface {
	// Name returns a human-readable adapter name (e.g. "openclaw").
	Name() string

	// Start begins listening for events from the agent framework.
	// The evaluator function is called for each action to get a verdict.
	Start(ctx context.Context, evaluator func(policy.ActionContext) policy.PolicyResult) error

	// Stop gracefully shuts down the adapter.
	Stop() error

	// KillAll immediately terminates all connections managed by this adapter.
	KillAll()

	// KillAgent terminates connections for a specific agent.
	KillAgent(agentID string)

	// KillSession terminates a specific session.
	KillSession(sessionID string)

	// ConnectedAgents returns the number of currently connected agents.
	ConnectedAgents() int
}
