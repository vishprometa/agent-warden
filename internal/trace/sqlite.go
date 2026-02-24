package trace

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed trace store.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Initialize() error {
	schema := `
	CREATE TABLE IF NOT EXISTS traces (
		id              TEXT PRIMARY KEY,
		session_id      TEXT NOT NULL,
		agent_id        TEXT NOT NULL,
		timestamp       DATETIME NOT NULL,
		action_type     TEXT NOT NULL,
		action_name     TEXT,
		request_body    TEXT,
		response_body   TEXT,
		status          TEXT NOT NULL,
		policy_name     TEXT,
		policy_reason   TEXT,
		latency_ms      INTEGER,
		tokens_in       INTEGER DEFAULT 0,
		tokens_out      INTEGER DEFAULT 0,
		cost_usd        REAL DEFAULT 0,
		model           TEXT,
		metadata        TEXT,
		prev_hash       TEXT,
		hash            TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id              TEXT PRIMARY KEY,
		agent_id        TEXT NOT NULL,
		started_at      DATETIME NOT NULL,
		ended_at        DATETIME,
		status          TEXT NOT NULL DEFAULT 'active',
		total_cost      REAL DEFAULT 0,
		action_count    INTEGER DEFAULT 0,
		metadata        TEXT,
		score           TEXT
	);

	CREATE TABLE IF NOT EXISTS agents (
		id              TEXT PRIMARY KEY,
		name            TEXT,
		created_at      DATETIME NOT NULL,
		current_version TEXT,
		config          TEXT,
		metadata        TEXT
	);

	CREATE TABLE IF NOT EXISTS agent_versions (
		id              TEXT PRIMARY KEY,
		agent_id        TEXT NOT NULL,
		version_number  INTEGER NOT NULL,
		created_at      DATETIME NOT NULL,
		promoted_at     DATETIME,
		rolled_back_at  DATETIME,
		status          TEXT NOT NULL,
		system_prompt   TEXT,
		config          TEXT,
		diff_from_prev  TEXT,
		diff_reason     TEXT,
		shadow_results  TEXT,
		metadata        TEXT
	);

	CREATE TABLE IF NOT EXISTS approvals (
		id              TEXT PRIMARY KEY,
		session_id      TEXT NOT NULL,
		trace_id        TEXT NOT NULL,
		policy_name     TEXT NOT NULL,
		action_summary  TEXT NOT NULL,
		status          TEXT NOT NULL DEFAULT 'pending',
		created_at      DATETIME NOT NULL,
		resolved_at     DATETIME,
		resolved_by     TEXT,
		timeout_at      DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS violations (
		id              TEXT PRIMARY KEY,
		trace_id        TEXT NOT NULL,
		session_id      TEXT NOT NULL,
		agent_id        TEXT NOT NULL,
		policy_name     TEXT NOT NULL,
		effect          TEXT NOT NULL,
		timestamp       DATETIME NOT NULL,
		action_summary  TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_traces_session ON traces(session_id);
	CREATE INDEX IF NOT EXISTS idx_traces_agent ON traces(agent_id);
	CREATE INDEX IF NOT EXISTS idx_traces_timestamp ON traces(timestamp);
	CREATE INDEX IF NOT EXISTS idx_traces_action_type ON traces(action_type);
	CREATE INDEX IF NOT EXISTS idx_sessions_agent ON sessions(agent_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
	CREATE INDEX IF NOT EXISTS idx_violations_agent ON violations(agent_id);
	CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);
	CREATE INDEX IF NOT EXISTS idx_agent_versions_agent ON agent_versions(agent_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// --- Traces ---

func (s *SQLiteStore) InsertTrace(t *Trace) error {
	_, err := s.db.Exec(`INSERT INTO traces (id, session_id, agent_id, timestamp, action_type, action_name,
		request_body, response_body, status, policy_name, policy_reason, latency_ms,
		tokens_in, tokens_out, cost_usd, model, metadata, prev_hash, hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.AgentID, t.Timestamp, t.ActionType, t.ActionName,
		nullableJSON(t.RequestBody), nullableJSON(t.ResponseBody),
		t.Status, nullStr(t.PolicyName), nullStr(t.PolicyReason), t.LatencyMs,
		t.TokensIn, t.TokensOut, t.CostUSD, nullStr(t.Model),
		nullableJSON(t.Metadata), t.PrevHash, t.Hash,
	)
	return err
}

func (s *SQLiteStore) GetTrace(id string) (*Trace, error) {
	t := &Trace{}
	var reqBody, respBody, metadata sql.NullString
	var policyName, policyReason, model, actionName sql.NullString

	err := s.db.QueryRow(`SELECT id, session_id, agent_id, timestamp, action_type, action_name,
		request_body, response_body, status, policy_name, policy_reason, latency_ms,
		tokens_in, tokens_out, cost_usd, model, metadata, prev_hash, hash
		FROM traces WHERE id = ?`, id).Scan(
		&t.ID, &t.SessionID, &t.AgentID, &t.Timestamp, &t.ActionType, &actionName,
		&reqBody, &respBody, &t.Status, &policyName, &policyReason, &t.LatencyMs,
		&t.TokensIn, &t.TokensOut, &t.CostUSD, &model, &metadata, &t.PrevHash, &t.Hash,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.ActionName = policyName.String
	t.RequestBody = jsonOrNil(reqBody)
	t.ResponseBody = jsonOrNil(respBody)
	t.PolicyName = policyName.String
	t.PolicyReason = policyReason.String
	t.Model = model.String
	t.Metadata = jsonOrNil(metadata)
	t.ActionName = actionName.String

	return t, nil
}

func (s *SQLiteStore) ListTraces(filter TraceFilter) ([]*Trace, int, error) {
	where, args := buildTraceWhere(filter)
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	// Count
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM traces"+where, args...).Scan(&count)
	if err != nil {
		return nil, 0, err
	}

	// Rows
	query := "SELECT id, session_id, agent_id, timestamp, action_type, action_name, status, latency_ms, tokens_in, tokens_out, cost_usd, model, policy_name, hash FROM traces" + where + " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, limit, filter.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var traces []*Trace
	for rows.Next() {
		t := &Trace{}
		var actionName, model, policyName sql.NullString
		if err := rows.Scan(&t.ID, &t.SessionID, &t.AgentID, &t.Timestamp, &t.ActionType,
			&actionName, &t.Status, &t.LatencyMs, &t.TokensIn, &t.TokensOut,
			&t.CostUSD, &model, &policyName, &t.Hash); err != nil {
			return nil, 0, err
		}
		t.ActionName = actionName.String
		t.Model = model.String
		t.PolicyName = policyName.String
		traces = append(traces, t)
	}
	return traces, count, nil
}

func (s *SQLiteStore) SearchTraces(query string, limit int) ([]*Trace, error) {
	if limit <= 0 {
		limit = 50
	}
	pattern := "%" + query + "%"
	rows, err := s.db.Query(`SELECT id, session_id, agent_id, timestamp, action_type, action_name, status, latency_ms, cost_usd, model, hash
		FROM traces WHERE request_body LIKE ? OR response_body LIKE ? OR action_name LIKE ?
		ORDER BY timestamp DESC LIMIT ?`, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []*Trace
	for rows.Next() {
		t := &Trace{}
		var actionName, model sql.NullString
		if err := rows.Scan(&t.ID, &t.SessionID, &t.AgentID, &t.Timestamp, &t.ActionType,
			&actionName, &t.Status, &t.LatencyMs, &t.CostUSD, &model, &t.Hash); err != nil {
			return nil, err
		}
		t.ActionName = actionName.String
		t.Model = model.String
		traces = append(traces, t)
	}
	return traces, nil
}

// --- Sessions ---

func (s *SQLiteStore) UpsertSession(sess *Session) error {
	_, err := s.db.Exec(`INSERT INTO sessions (id, agent_id, started_at, ended_at, status, total_cost, action_count, metadata, score)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ended_at = excluded.ended_at,
			status = excluded.status,
			total_cost = excluded.total_cost,
			action_count = excluded.action_count,
			metadata = excluded.metadata,
			score = excluded.score`,
		sess.ID, sess.AgentID, sess.StartedAt, sess.EndedAt, sess.Status,
		sess.TotalCost, sess.ActionCount, nullableJSON(sess.Metadata), nullableJSON(sess.Score),
	)
	return err
}

func (s *SQLiteStore) GetSession(id string) (*Session, error) {
	sess := &Session{}
	var metadata, score sql.NullString

	err := s.db.QueryRow(`SELECT id, agent_id, started_at, ended_at, status, total_cost, action_count, metadata, score
		FROM sessions WHERE id = ?`, id).Scan(
		&sess.ID, &sess.AgentID, &sess.StartedAt, &sess.EndedAt, &sess.Status,
		&sess.TotalCost, &sess.ActionCount, &metadata, &score,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.Metadata = jsonOrNil(metadata)
	sess.Score = jsonOrNil(score)
	return sess, nil
}

func (s *SQLiteStore) ListSessions(filter SessionFilter) ([]*Session, int, error) {
	where, args := buildSessionWhere(filter)
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sessions"+where, args...).Scan(&count)
	if err != nil {
		return nil, 0, err
	}

	query := "SELECT id, agent_id, started_at, ended_at, status, total_cost, action_count FROM sessions" + where + " ORDER BY started_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, filter.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		sess := &Session{}
		if err := rows.Scan(&sess.ID, &sess.AgentID, &sess.StartedAt, &sess.EndedAt,
			&sess.Status, &sess.TotalCost, &sess.ActionCount); err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, count, nil
}

func (s *SQLiteStore) UpdateSessionStatus(id, status string) error {
	now := time.Now()
	var endedAt *time.Time
	if status == "completed" || status == "terminated" {
		endedAt = &now
	}
	_, err := s.db.Exec("UPDATE sessions SET status = ?, ended_at = ? WHERE id = ?", status, endedAt, id)
	return err
}

func (s *SQLiteStore) UpdateSessionCost(id string, cost float64, actionCount int) error {
	_, err := s.db.Exec("UPDATE sessions SET total_cost = ?, action_count = ? WHERE id = ?", cost, actionCount, id)
	return err
}

func (s *SQLiteStore) ScoreSession(id string, score []byte) error {
	_, err := s.db.Exec("UPDATE sessions SET score = ? WHERE id = ?", string(score), id)
	return err
}

// --- Agents ---

func (s *SQLiteStore) UpsertAgent(a *Agent) error {
	_, err := s.db.Exec(`INSERT INTO agents (id, name, created_at, current_version, config, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			current_version = excluded.current_version,
			config = excluded.config,
			metadata = excluded.metadata`,
		a.ID, a.Name, a.CreatedAt, nullStr(a.CurrentVersion),
		nullableJSON(a.Config), nullableJSON(a.Metadata),
	)
	return err
}

func (s *SQLiteStore) GetAgent(id string) (*Agent, error) {
	a := &Agent{}
	var name, currentVersion sql.NullString
	var config, metadata sql.NullString

	err := s.db.QueryRow(`SELECT id, name, created_at, current_version, config, metadata FROM agents WHERE id = ?`, id).Scan(
		&a.ID, &name, &a.CreatedAt, &currentVersion, &config, &metadata,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Name = name.String
	a.CurrentVersion = currentVersion.String
	a.Config = jsonOrNil(config)
	a.Metadata = jsonOrNil(metadata)
	return a, nil
}

func (s *SQLiteStore) ListAgents() ([]*Agent, error) {
	rows, err := s.db.Query("SELECT id, name, created_at, current_version FROM agents ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		a := &Agent{}
		var name, currentVersion sql.NullString
		if err := rows.Scan(&a.ID, &name, &a.CreatedAt, &currentVersion); err != nil {
			return nil, err
		}
		a.Name = name.String
		a.CurrentVersion = currentVersion.String
		agents = append(agents, a)
	}
	return agents, nil
}

func (s *SQLiteStore) GetAgentStats(agentID string) (*AgentStats, error) {
	stats := &AgentStats{AgentID: agentID}

	s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE agent_id = ?", agentID).Scan(&stats.TotalSessions)
	s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE agent_id = ? AND status = 'active'", agentID).Scan(&stats.ActiveSessions)
	s.db.QueryRow("SELECT COALESCE(SUM(total_cost), 0) FROM sessions WHERE agent_id = ?", agentID).Scan(&stats.TotalCost)
	s.db.QueryRow("SELECT COALESCE(SUM(action_count), 0) FROM sessions WHERE agent_id = ?", agentID).Scan(&stats.TotalActions)
	s.db.QueryRow("SELECT COUNT(*) FROM violations WHERE agent_id = ?", agentID).Scan(&stats.TotalViolations)

	if stats.TotalSessions > 0 {
		stats.AvgCostPerSession = stats.TotalCost / float64(stats.TotalSessions)
		var completed int
		s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE agent_id = ? AND status = 'completed'", agentID).Scan(&completed)
		stats.CompletionRate = float64(completed) / float64(stats.TotalSessions)
		var terminated int
		s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE agent_id = ? AND status = 'terminated'", agentID).Scan(&terminated)
		stats.ErrorRate = float64(terminated) / float64(stats.TotalSessions)
	}

	return stats, nil
}

// --- Agent Versions ---

func (s *SQLiteStore) InsertAgentVersion(v *AgentVersion) error {
	_, err := s.db.Exec(`INSERT INTO agent_versions (id, agent_id, version_number, created_at, promoted_at,
		rolled_back_at, status, system_prompt, config, diff_from_prev, diff_reason, shadow_results, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.AgentID, v.VersionNumber, v.CreatedAt, v.PromotedAt, v.RolledBackAt,
		v.Status, nullStr(v.SystemPrompt), nullableJSON(v.Config),
		nullStr(v.DiffFromPrev), nullStr(v.DiffReason),
		nullableJSON(v.ShadowResults), nullableJSON(v.Metadata),
	)
	return err
}

func (s *SQLiteStore) GetAgentVersion(id string) (*AgentVersion, error) {
	v := &AgentVersion{}
	var config, shadowResults, metadata sql.NullString
	err := s.db.QueryRow(`SELECT id, agent_id, version_number, created_at, promoted_at, rolled_back_at,
		status, system_prompt, config, diff_from_prev, diff_reason, shadow_results, metadata
		FROM agent_versions WHERE id = ?`, id).Scan(
		&v.ID, &v.AgentID, &v.VersionNumber, &v.CreatedAt, &v.PromotedAt, &v.RolledBackAt,
		&v.Status, &v.SystemPrompt, &config, &v.DiffFromPrev, &v.DiffReason, &shadowResults, &metadata,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.Config = jsonOrNil(config)
	v.ShadowResults = jsonOrNil(shadowResults)
	v.Metadata = jsonOrNil(metadata)
	return v, nil
}

func (s *SQLiteStore) ListAgentVersions(agentID string) ([]*AgentVersion, error) {
	rows, err := s.db.Query(`SELECT id, agent_id, version_number, created_at, status, diff_reason
		FROM agent_versions WHERE agent_id = ? ORDER BY version_number DESC`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*AgentVersion
	for rows.Next() {
		v := &AgentVersion{}
		if err := rows.Scan(&v.ID, &v.AgentID, &v.VersionNumber, &v.CreatedAt, &v.Status, &v.DiffReason); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, nil
}

// --- Approvals ---

func (s *SQLiteStore) InsertApproval(a *Approval) error {
	_, err := s.db.Exec(`INSERT INTO approvals (id, session_id, trace_id, policy_name, action_summary, status, created_at, timeout_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.SessionID, a.TraceID, a.PolicyName, string(a.ActionSummary), a.Status, a.CreatedAt, a.TimeoutAt,
	)
	return err
}

func (s *SQLiteStore) GetApproval(id string) (*Approval, error) {
	a := &Approval{}
	var actionSummary string
	err := s.db.QueryRow(`SELECT id, session_id, trace_id, policy_name, action_summary, status, created_at, resolved_at, resolved_by, timeout_at
		FROM approvals WHERE id = ?`, id).Scan(
		&a.ID, &a.SessionID, &a.TraceID, &a.PolicyName, &actionSummary, &a.Status,
		&a.CreatedAt, &a.ResolvedAt, &a.ResolvedBy, &a.TimeoutAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.ActionSummary = json.RawMessage(actionSummary)
	return a, nil
}

func (s *SQLiteStore) ListPendingApprovals() ([]*Approval, error) {
	rows, err := s.db.Query(`SELECT id, session_id, trace_id, policy_name, action_summary, status, created_at, timeout_at
		FROM approvals WHERE status = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []*Approval
	for rows.Next() {
		a := &Approval{}
		var actionSummary string
		if err := rows.Scan(&a.ID, &a.SessionID, &a.TraceID, &a.PolicyName, &actionSummary, &a.Status, &a.CreatedAt, &a.TimeoutAt); err != nil {
			return nil, err
		}
		a.ActionSummary = json.RawMessage(actionSummary)
		approvals = append(approvals, a)
	}
	return approvals, nil
}

func (s *SQLiteStore) ResolveApproval(id, status, resolvedBy string) error {
	now := time.Now()
	_, err := s.db.Exec("UPDATE approvals SET status = ?, resolved_at = ?, resolved_by = ? WHERE id = ?",
		status, now, resolvedBy, id)
	return err
}

// --- Violations ---

func (s *SQLiteStore) InsertViolation(v *Violation) error {
	_, err := s.db.Exec(`INSERT INTO violations (id, trace_id, session_id, agent_id, policy_name, effect, timestamp, action_summary)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.TraceID, v.SessionID, v.AgentID, v.PolicyName, v.Effect, v.Timestamp, nullableJSON(v.ActionSummary),
	)
	return err
}

func (s *SQLiteStore) ListViolations(agentID string, limit int) ([]*Violation, error) {
	if limit <= 0 {
		limit = 50
	}
	query := "SELECT id, trace_id, session_id, agent_id, policy_name, effect, timestamp FROM violations"
	var args []interface{}
	if agentID != "" {
		query += " WHERE agent_id = ?"
		args = append(args, agentID)
	}
	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []*Violation
	for rows.Next() {
		v := &Violation{}
		if err := rows.Scan(&v.ID, &v.TraceID, &v.SessionID, &v.AgentID, &v.PolicyName, &v.Effect, &v.Timestamp); err != nil {
			return nil, err
		}
		violations = append(violations, v)
	}
	return violations, nil
}

// --- Maintenance ---

func (s *SQLiteStore) PruneOlderThan(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	result, err := s.db.Exec("DELETE FROM traces WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *SQLiteStore) VerifyHashChain(sessionID string) (bool, int, error) {
	rows, err := s.db.Query(`SELECT id, session_id, action_type, request_body, response_body, prev_hash, hash
		FROM traces WHERE session_id = ? ORDER BY timestamp ASC`, sessionID)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()

	var traces []*Trace
	for rows.Next() {
		t := &Trace{}
		var reqBody, respBody sql.NullString
		if err := rows.Scan(&t.ID, &t.SessionID, &t.ActionType, &reqBody, &respBody, &t.PrevHash, &t.Hash); err != nil {
			return false, 0, err
		}
		t.RequestBody = jsonOrNil(reqBody)
		t.ResponseBody = jsonOrNil(respBody)
		traces = append(traces, t)
	}

	valid, brokenAt := VerifyChain(traces)
	return valid, brokenAt, nil
}

// --- System Stats ---

func (s *SQLiteStore) GetSystemStats() (*SystemStats, error) {
	stats := &SystemStats{}
	s.db.QueryRow("SELECT COUNT(*) FROM traces").Scan(&stats.TotalTraces)
	s.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&stats.TotalSessions)
	s.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE status = 'active'").Scan(&stats.ActiveSessions)
	s.db.QueryRow("SELECT COUNT(*) FROM agents").Scan(&stats.TotalAgents)
	s.db.QueryRow("SELECT COALESCE(SUM(total_cost), 0) FROM sessions").Scan(&stats.TotalCost)
	s.db.QueryRow("SELECT COUNT(*) FROM violations").Scan(&stats.TotalViolations)
	s.db.QueryRow("SELECT COUNT(*) FROM approvals WHERE status = 'pending'").Scan(&stats.PendingApprovals)
	return stats, nil
}

// --- Helpers ---

func buildTraceWhere(f TraceFilter) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if f.SessionID != "" {
		conditions = append(conditions, "session_id = ?")
		args = append(args, f.SessionID)
	}
	if f.AgentID != "" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if f.ActionType != "" {
		conditions = append(conditions, "action_type = ?")
		args = append(args, f.ActionType)
	}
	if f.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, f.Status)
	}
	if f.Since != nil {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, *f.Until)
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func buildSessionWhere(f SessionFilter) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if f.AgentID != "" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, f.AgentID)
	}
	if f.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, f.Status)
	}
	if f.Since != nil {
		conditions = append(conditions, "started_at >= ?")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		conditions = append(conditions, "started_at <= ?")
		args = append(args, *f.Until)
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullableJSON(data json.RawMessage) sql.NullString {
	if data == nil || string(data) == "null" {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func jsonOrNil(ns sql.NullString) json.RawMessage {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	return json.RawMessage(ns.String)
}
