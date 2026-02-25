// Package spawn implements governance for agent self-replication. OpenClaw
// agents can spawn child agents and fund them with Bitcoin via the Lightning
// Network. This governor enforces limits on spawn depth, child count, budget
// inheritance, and provides cascade kill (killing a parent kills all children).
package spawn

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Config holds spawn governance configuration.
type Config struct {
	Enabled             bool    `yaml:"enabled" json:"enabled"`
	MaxChildrenPerAgent int     `yaml:"max_children_per_agent" json:"max_children_per_agent"`
	MaxDepth            int     `yaml:"max_depth" json:"max_depth"`
	MaxGlobalAgents     int     `yaml:"max_global_agents" json:"max_global_agents"`
	InheritCapabilities bool    `yaml:"inherit_capabilities" json:"inherit_capabilities"`
	RequireApproval     bool    `yaml:"require_approval" json:"require_approval"`
	CascadeKill         bool    `yaml:"cascade_kill" json:"cascade_kill"`
	ChildBudgetMax      float64 `yaml:"child_budget_max" json:"child_budget_max"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:             true,
		MaxChildrenPerAgent: 3,
		MaxDepth:            2,
		MaxGlobalAgents:     20,
		InheritCapabilities: true,
		CascadeKill:         true,
		ChildBudgetMax:      0.5,
	}
}

// AgentNode represents an agent in the spawn tree.
type AgentNode struct {
	AgentID   string    `json:"agent_id"`
	ParentID  string    `json:"parent_id,omitempty"`
	Depth     int       `json:"depth"`
	Children  []string  `json:"children"`
	CreatedAt time.Time `json:"created_at"`
	Budget    float64   `json:"budget"`
}

// SpawnResult is the outcome of a spawn request.
type SpawnResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Governor manages the agent spawn tree and enforces limits.
type Governor struct {
	mu     sync.RWMutex
	config Config
	agents map[string]*AgentNode // agentID → node
	logger *slog.Logger
}

// NewGovernor creates a new spawn governor.
func NewGovernor(cfg Config, logger *slog.Logger) *Governor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Governor{
		config: cfg,
		agents: make(map[string]*AgentNode),
		logger: logger.With("component", "spawn.Governor"),
	}
}

// RegisterRoot registers a top-level (non-spawned) agent.
func (g *Governor) RegisterRoot(agentID string, budget float64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.agents[agentID]; exists {
		return
	}

	g.agents[agentID] = &AgentNode{
		AgentID:   agentID,
		Depth:     0,
		CreatedAt: time.Now(),
		Budget:    budget,
	}
}

// RequestSpawn evaluates whether a parent agent is allowed to spawn a child.
func (g *Governor) RequestSpawn(parentID, childID string, requestedBudget float64) SpawnResult {
	g.mu.Lock()
	defer g.mu.Unlock()

	if !g.config.Enabled {
		return SpawnResult{Allowed: false, Reason: "agent spawning is disabled"}
	}

	// Check global limit.
	if len(g.agents) >= g.config.MaxGlobalAgents {
		return SpawnResult{
			Allowed: false,
			Reason:  fmt.Sprintf("global agent limit reached (%d/%d)", len(g.agents), g.config.MaxGlobalAgents),
		}
	}

	// Find parent.
	parent, ok := g.agents[parentID]
	if !ok {
		// Auto-register parent as root.
		parent = &AgentNode{
			AgentID:   parentID,
			Depth:     0,
			CreatedAt: time.Now(),
		}
		g.agents[parentID] = parent
	}

	// Check depth limit.
	childDepth := parent.Depth + 1
	if childDepth > g.config.MaxDepth {
		return SpawnResult{
			Allowed: false,
			Reason:  fmt.Sprintf("spawn depth limit exceeded (%d/%d)", childDepth, g.config.MaxDepth),
		}
	}

	// Check per-agent child limit.
	if len(parent.Children) >= g.config.MaxChildrenPerAgent {
		return SpawnResult{
			Allowed: false,
			Reason:  fmt.Sprintf("agent %s has reached child limit (%d/%d)", parentID, len(parent.Children), g.config.MaxChildrenPerAgent),
		}
	}

	// Check budget.
	if g.config.ChildBudgetMax > 0 && requestedBudget > parent.Budget*g.config.ChildBudgetMax {
		return SpawnResult{
			Allowed: false,
			Reason:  fmt.Sprintf("requested budget $%.2f exceeds allowed %.0f%% of parent budget $%.2f", requestedBudget, g.config.ChildBudgetMax*100, parent.Budget),
		}
	}

	// Approval required?
	if g.config.RequireApproval {
		return SpawnResult{
			Allowed: false,
			Reason:  "spawn requires human approval",
		}
	}

	// Spawn allowed — register child.
	child := &AgentNode{
		AgentID:   childID,
		ParentID:  parentID,
		Depth:     childDepth,
		CreatedAt: time.Now(),
		Budget:    requestedBudget,
	}
	g.agents[childID] = child
	parent.Children = append(parent.Children, childID)

	g.logger.Info("agent spawn approved",
		"parent_id", parentID,
		"child_id", childID,
		"depth", childDepth,
		"budget", requestedBudget,
	)

	return SpawnResult{Allowed: true}
}

// KillAgent removes an agent and optionally all its descendants.
func (g *Governor) KillAgent(agentID string) []string {
	g.mu.Lock()
	defer g.mu.Unlock()

	var killed []string

	if g.config.CascadeKill {
		killed = g.cascadeKill(agentID)
	} else {
		if _, ok := g.agents[agentID]; ok {
			delete(g.agents, agentID)
			killed = append(killed, agentID)
		}
	}

	return killed
}

// cascadeKill recursively kills an agent and all descendants.
func (g *Governor) cascadeKill(agentID string) []string {
	agent, ok := g.agents[agentID]
	if !ok {
		return nil
	}

	var killed []string

	// Recursively kill children first.
	for _, childID := range agent.Children {
		killed = append(killed, g.cascadeKill(childID)...)
	}

	// Kill this agent.
	delete(g.agents, agentID)
	killed = append(killed, agentID)

	// Remove from parent's children list.
	if agent.ParentID != "" {
		if parent, ok := g.agents[agent.ParentID]; ok {
			for i, id := range parent.Children {
				if id == agentID {
					parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
					break
				}
			}
		}
	}

	return killed
}

// GetTree returns the full spawn tree for visualization.
func (g *Governor) GetTree() map[string]*AgentNode {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make(map[string]*AgentNode, len(g.agents))
	for k, v := range g.agents {
		node := *v
		children := make([]string, len(v.Children))
		copy(children, v.Children)
		node.Children = children
		result[k] = &node
	}
	return result
}

// GetDescendants returns all descendants of an agent (recursive).
func (g *Governor) GetDescendants(agentID string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []string
	g.collectDescendants(agentID, &result)
	return result
}

func (g *Governor) collectDescendants(agentID string, result *[]string) {
	agent, ok := g.agents[agentID]
	if !ok {
		return
	}
	for _, childID := range agent.Children {
		*result = append(*result, childID)
		g.collectDescendants(childID, result)
	}
}

// AgentCount returns the total number of tracked agents.
func (g *Governor) AgentCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.agents)
}
