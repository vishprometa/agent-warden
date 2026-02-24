package trace

// Store defines the interface for trace persistence backends.
type Store interface {
	// Initialize creates tables and indexes.
	Initialize() error

	// Close cleanly shuts down the store.
	Close() error

	// Traces
	InsertTrace(t *Trace) error
	GetTrace(id string) (*Trace, error)
	ListTraces(filter TraceFilter) ([]*Trace, int, error)
	SearchTraces(query string, limit int) ([]*Trace, error)

	// Sessions
	UpsertSession(s *Session) error
	GetSession(id string) (*Session, error)
	ListSessions(filter SessionFilter) ([]*Session, int, error)
	UpdateSessionStatus(id, status string) error
	UpdateSessionCost(id string, cost float64, actionCount int) error
	ScoreSession(id string, score []byte) error

	// Agents
	UpsertAgent(a *Agent) error
	GetAgent(id string) (*Agent, error)
	ListAgents() ([]*Agent, error)
	GetAgentStats(agentID string) (*AgentStats, error)

	// Agent Versions
	InsertAgentVersion(v *AgentVersion) error
	GetAgentVersion(id string) (*AgentVersion, error)
	ListAgentVersions(agentID string) ([]*AgentVersion, error)

	// Approvals
	InsertApproval(a *Approval) error
	GetApproval(id string) (*Approval, error)
	ListPendingApprovals() ([]*Approval, error)
	ResolveApproval(id, status, resolvedBy string) error

	// Violations
	InsertViolation(v *Violation) error
	ListViolations(agentID string, limit int) ([]*Violation, error)

	// Maintenance
	PruneOlderThan(days int) (int64, error)
	VerifyHashChain(sessionID string) (bool, int, error)

	// Metrics
	GetSystemStats() (*SystemStats, error)
}

// SystemStats holds aggregate system metrics.
type SystemStats struct {
	TotalTraces     int64   `json:"total_traces"`
	TotalSessions   int64   `json:"total_sessions"`
	ActiveSessions  int64   `json:"active_sessions"`
	TotalAgents     int64   `json:"total_agents"`
	TotalCost       float64 `json:"total_cost"`
	TotalViolations int64   `json:"total_violations"`
	PendingApprovals int64  `json:"pending_approvals"`
}
