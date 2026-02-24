package trace

import (
	"encoding/json"
	"time"
)

// ActionType categorizes intercepted actions.
type ActionType string

const (
	ActionLLMChat      ActionType = "llm.chat"
	ActionLLMEmbed     ActionType = "llm.embedding"
	ActionToolCall     ActionType = "tool.call"
	ActionAPIRequest   ActionType = "api.request"
	ActionDBQuery      ActionType = "db.query"
	ActionFileWrite    ActionType = "file.write"
	ActionCodeExec     ActionType = "code.exec"
	ActionMCPTool      ActionType = "mcp.tool"
)

// TraceStatus represents the policy evaluation result.
type TraceStatus string

const (
	StatusAllowed    TraceStatus = "allowed"
	StatusDenied     TraceStatus = "denied"
	StatusTerminated TraceStatus = "terminated"
	StatusApproved   TraceStatus = "approved"
	StatusPending    TraceStatus = "pending"
	StatusThrottled  TraceStatus = "throttled"
)

// Trace represents a single intercepted action with full context.
type Trace struct {
	ID           string          `json:"id" db:"id"`
	SessionID    string          `json:"session_id" db:"session_id"`
	AgentID      string          `json:"agent_id" db:"agent_id"`
	Timestamp    time.Time       `json:"timestamp" db:"timestamp"`
	ActionType   ActionType      `json:"action_type" db:"action_type"`
	ActionName   string          `json:"action_name,omitempty" db:"action_name"`
	RequestBody  json.RawMessage `json:"request_body,omitempty" db:"request_body"`
	ResponseBody json.RawMessage `json:"response_body,omitempty" db:"response_body"`
	Status       TraceStatus     `json:"status" db:"status"`
	PolicyName   string          `json:"policy_name,omitempty" db:"policy_name"`
	PolicyReason string          `json:"policy_reason,omitempty" db:"policy_reason"`
	LatencyMs    int64           `json:"latency_ms" db:"latency_ms"`
	TokensIn     int             `json:"tokens_in" db:"tokens_in"`
	TokensOut    int             `json:"tokens_out" db:"tokens_out"`
	CostUSD      float64         `json:"cost_usd" db:"cost_usd"`
	Metadata     json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	PrevHash     string          `json:"prev_hash" db:"prev_hash"`
	Hash         string          `json:"hash" db:"hash"`
	Model        string          `json:"model,omitempty" db:"model"`
}

// Session represents a group of related agent actions.
type Session struct {
	ID          string          `json:"id" db:"id"`
	AgentID     string          `json:"agent_id" db:"agent_id"`
	StartedAt   time.Time       `json:"started_at" db:"started_at"`
	EndedAt     *time.Time      `json:"ended_at,omitempty" db:"ended_at"`
	Status      string          `json:"status" db:"status"` // active, completed, terminated, paused
	TotalCost   float64         `json:"total_cost" db:"total_cost"`
	ActionCount int             `json:"action_count" db:"action_count"`
	Metadata    json.RawMessage `json:"metadata,omitempty" db:"metadata"`
	Score       json.RawMessage `json:"score,omitempty" db:"score"`
}

// Agent represents a registered agent.
type Agent struct {
	ID             string          `json:"id" db:"id"`
	Name           string          `json:"name" db:"name"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	CurrentVersion string          `json:"current_version,omitempty" db:"current_version"`
	Config         json.RawMessage `json:"config,omitempty" db:"config"`
	Metadata       json.RawMessage `json:"metadata,omitempty" db:"metadata"`
}

// AgentVersion stores a snapshot of an agent at a point in time.
type AgentVersion struct {
	ID            string          `json:"id" db:"id"`
	AgentID       string          `json:"agent_id" db:"agent_id"`
	VersionNumber int             `json:"version_number" db:"version_number"`
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
	PromotedAt    *time.Time      `json:"promoted_at,omitempty" db:"promoted_at"`
	RolledBackAt  *time.Time      `json:"rolled_back_at,omitempty" db:"rolled_back_at"`
	Status        string          `json:"status" db:"status"` // active, candidate, shadow, retired, rolled_back
	SystemPrompt  string          `json:"system_prompt,omitempty" db:"system_prompt"`
	Config        json.RawMessage `json:"config,omitempty" db:"config"`
	DiffFromPrev  string          `json:"diff_from_prev,omitempty" db:"diff_from_prev"`
	DiffReason    string          `json:"diff_reason,omitempty" db:"diff_reason"`
	ShadowResults json.RawMessage `json:"shadow_results,omitempty" db:"shadow_results"`
	Metadata      json.RawMessage `json:"metadata,omitempty" db:"metadata"`
}

// Approval represents a pending human approval request.
type Approval struct {
	ID            string          `json:"id" db:"id"`
	SessionID     string          `json:"session_id" db:"session_id"`
	TraceID       string          `json:"trace_id" db:"trace_id"`
	PolicyName    string          `json:"policy_name" db:"policy_name"`
	ActionSummary json.RawMessage `json:"action_summary" db:"action_summary"`
	Status        string          `json:"status" db:"status"` // pending, approved, denied, timed_out
	CreatedAt     time.Time       `json:"created_at" db:"created_at"`
	ResolvedAt    *time.Time      `json:"resolved_at,omitempty" db:"resolved_at"`
	ResolvedBy    string          `json:"resolved_by,omitempty" db:"resolved_by"`
	TimeoutAt     time.Time       `json:"timeout_at" db:"timeout_at"`
}

// Violation records a policy violation event.
type Violation struct {
	ID            string          `json:"id" db:"id"`
	TraceID       string          `json:"trace_id" db:"trace_id"`
	SessionID     string          `json:"session_id" db:"session_id"`
	AgentID       string          `json:"agent_id" db:"agent_id"`
	PolicyName    string          `json:"policy_name" db:"policy_name"`
	Effect        string          `json:"effect" db:"effect"`
	Timestamp     time.Time       `json:"timestamp" db:"timestamp"`
	ActionSummary json.RawMessage `json:"action_summary,omitempty" db:"action_summary"`
}

// TraceFilter defines query parameters for listing traces.
type TraceFilter struct {
	SessionID  string
	AgentID    string
	ActionType ActionType
	Status     TraceStatus
	Since      *time.Time
	Until      *time.Time
	Query      string // full-text search
	Limit      int
	Offset     int
}

// SessionFilter defines query parameters for listing sessions.
type SessionFilter struct {
	AgentID string
	Status  string
	Since   *time.Time
	Until   *time.Time
	Limit   int
	Offset  int
}

// AgentStats holds aggregated metrics for an agent.
type AgentStats struct {
	AgentID         string  `json:"agent_id"`
	TotalSessions   int     `json:"total_sessions"`
	ActiveSessions  int     `json:"active_sessions"`
	TotalCost       float64 `json:"total_cost"`
	TotalActions    int     `json:"total_actions"`
	TotalViolations int     `json:"total_violations"`
	AvgCostPerSession float64 `json:"avg_cost_per_session"`
	CompletionRate  float64 `json:"completion_rate"`
	ErrorRate       float64 `json:"error_rate"`
}
