package evolution

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// mockStore implements trace.Store for testing analyzer methods.
type mockStore struct {
	traces   []*trace.Trace
	sessions []*trace.Session
	listTracesErr   error
	listSessionsErr error
}

func (m *mockStore) ListTraces(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
	if m.listTracesErr != nil {
		return nil, 0, m.listTracesErr
	}

	var filtered []*trace.Trace
	for _, t := range m.traces {
		if filter.AgentID != "" && t.AgentID != filter.AgentID {
			continue
		}
		if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		if filter.Since != nil && t.Timestamp.Before(*filter.Since) {
			continue
		}
		filtered = append(filtered, t)
	}

	if filter.Limit > 0 && len(filtered) > filter.Limit {
		filtered = filtered[:filter.Limit]
	}

	return filtered, len(filtered), nil
}

func (m *mockStore) ListSessions(filter trace.SessionFilter) ([]*trace.Session, int, error) {
	if m.listSessionsErr != nil {
		return nil, 0, m.listSessionsErr
	}

	var filtered []*trace.Session
	for _, s := range m.sessions {
		if filter.AgentID != "" && s.AgentID != filter.AgentID {
			continue
		}
		if filter.Status != "" && s.Status != filter.Status {
			continue
		}
		if filter.Since != nil && s.StartedAt.Before(*filter.Since) {
			continue
		}
		filtered = append(filtered, s)
	}

	if filter.Limit > 0 && len(filtered) > filter.Limit {
		filtered = filtered[:filter.Limit]
	}

	return filtered, len(filtered), nil
}

// Stub methods for unused Store interface methods.
func (m *mockStore) Initialize() error                                    { return nil }
func (m *mockStore) Close() error                                         { return nil }
func (m *mockStore) InsertTrace(t *trace.Trace) error                     { return nil }
func (m *mockStore) GetTrace(id string) (*trace.Trace, error)             { return nil, nil }
func (m *mockStore) SearchTraces(query string, limit int) ([]*trace.Trace, error) { return nil, nil }
func (m *mockStore) UpsertSession(s *trace.Session) error                 { return nil }
func (m *mockStore) GetSession(id string) (*trace.Session, error)         { return nil, nil }
func (m *mockStore) UpdateSessionStatus(id, status string) error          { return nil }
func (m *mockStore) UpdateSessionCost(id string, cost float64, actionCount int) error { return nil }
func (m *mockStore) ScoreSession(id string, score []byte) error           { return nil }
func (m *mockStore) UpsertAgent(a *trace.Agent) error                     { return nil }
func (m *mockStore) GetAgent(id string) (*trace.Agent, error)             { return nil, nil }
func (m *mockStore) ListAgents() ([]*trace.Agent, error)                  { return nil, nil }
func (m *mockStore) GetAgentStats(agentID string) (*trace.AgentStats, error) { return nil, nil }
func (m *mockStore) InsertAgentVersion(v *trace.AgentVersion) error       { return nil }
func (m *mockStore) GetAgentVersion(id string) (*trace.AgentVersion, error) { return nil, nil }
func (m *mockStore) ListAgentVersions(agentID string) ([]*trace.AgentVersion, error) { return nil, nil }
func (m *mockStore) InsertApproval(a *trace.Approval) error               { return nil }
func (m *mockStore) GetApproval(id string) (*trace.Approval, error)       { return nil, nil }
func (m *mockStore) ListPendingApprovals() ([]*trace.Approval, error)     { return nil, nil }
func (m *mockStore) ResolveApproval(id, status, resolvedBy string) error  { return nil }
func (m *mockStore) InsertViolation(v *trace.Violation) error             { return nil }
func (m *mockStore) ListViolations(agentID string, limit int) ([]*trace.Violation, error) { return nil, nil }
func (m *mockStore) PruneOlderThan(days int) (int64, error)               { return 0, nil }
func (m *mockStore) VerifyHashChain(sessionID string) (bool, int, error)  { return true, 0, nil }
func (m *mockStore) GetSystemStats() (*trace.SystemStats, error)          { return nil, nil }

// mockLLM implements a test LLM client that returns canned responses.
type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestNewAnalyzer(t *testing.T) {
	store := &mockStore{}
	llm := &mockLLM{}

	analyzer := NewAnalyzer(store, llm)
	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
	if analyzer.store != store {
		t.Error("analyzer.store not set correctly")
	}
	if analyzer.llm == nil {
		t.Error("analyzer.llm not set")
	}
}

func TestGetMetrics_NoData(t *testing.T) {
	store := &mockStore{
		traces:   []*trace.Trace{},
		sessions: []*trace.Session{},
	}
	analyzer := NewAnalyzer(store, nil)

	metrics, err := analyzer.GetMetrics("agent1", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.TotalSessions != 0 {
		t.Errorf("expected TotalSessions=0, got %d", metrics.TotalSessions)
	}
	if metrics.CompletionRate != 0 {
		t.Errorf("expected CompletionRate=0, got %f", metrics.CompletionRate)
	}
	if metrics.ErrorRate != 0 {
		t.Errorf("expected ErrorRate=0, got %f", metrics.ErrorRate)
	}
	if metrics.Window != 24*time.Hour {
		t.Errorf("expected Window=24h, got %s", metrics.Window)
	}
}

func TestGetMetrics_WithData(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		traces: []*trace.Trace{
			{ID: "t1", AgentID: "agent1", Status: trace.StatusAllowed, LatencyMs: 100, CostUSD: 0.01, Timestamp: now.Add(-1 * time.Hour)},
			{ID: "t2", AgentID: "agent1", Status: trace.StatusDenied, LatencyMs: 50, CostUSD: 0.005, Timestamp: now.Add(-2 * time.Hour)},
			{ID: "t3", AgentID: "agent1", Status: trace.StatusApproved, LatencyMs: 200, CostUSD: 0.02, Timestamp: now.Add(-3 * time.Hour)},
			{ID: "t4", AgentID: "agent1", Status: trace.StatusTerminated, LatencyMs: 75, CostUSD: 0.01, Timestamp: now.Add(-4 * time.Hour)},
			{ID: "t5", AgentID: "agent1", Status: trace.StatusAllowed, LatencyMs: 150, CostUSD: 0.015, Timestamp: now.Add(-5 * time.Hour)},
		},
		sessions: []*trace.Session{
			{ID: "s1", AgentID: "agent1", Status: "completed", StartedAt: now.Add(-1 * time.Hour)},
			{ID: "s2", AgentID: "agent1", Status: "active", StartedAt: now.Add(-2 * time.Hour)},
			{ID: "s3", AgentID: "agent1", Status: "completed", StartedAt: now.Add(-3 * time.Hour)},
		},
	}
	analyzer := NewAnalyzer(store, nil)

	metrics, err := analyzer.GetMetrics("agent1", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	// TotalSessions = 3
	if metrics.TotalSessions != 3 {
		t.Errorf("expected TotalSessions=3, got %d", metrics.TotalSessions)
	}

	// CompletionRate = 2/3 = 0.6666...
	expectedCompletionRate := 2.0 / 3.0
	if fmt.Sprintf("%.2f", metrics.CompletionRate) != fmt.Sprintf("%.2f", expectedCompletionRate) {
		t.Errorf("expected CompletionRate=%.2f, got %.2f", expectedCompletionRate, metrics.CompletionRate)
	}

	// ErrorRate = (1 denied + 1 terminated) / 5 traces = 2/5 = 0.4
	expectedErrorRate := 2.0 / 5.0
	if fmt.Sprintf("%.2f", metrics.ErrorRate) != fmt.Sprintf("%.2f", expectedErrorRate) {
		t.Errorf("expected ErrorRate=%.2f, got %.2f", expectedErrorRate, metrics.ErrorRate)
	}

	// HumanOverrideRate = 1 approved / 5 traces = 0.2
	expectedOverrideRate := 1.0 / 5.0
	if fmt.Sprintf("%.2f", metrics.HumanOverrideRate) != fmt.Sprintf("%.2f", expectedOverrideRate) {
		t.Errorf("expected HumanOverrideRate=%.2f, got %.2f", expectedOverrideRate, metrics.HumanOverrideRate)
	}

	// AvgLatency = (100+50+200+75+150) / 5 = 575/5 = 115
	expectedLatency := 115.0
	if metrics.AvgLatency != expectedLatency {
		t.Errorf("expected AvgLatency=%.0f, got %.0f", expectedLatency, metrics.AvgLatency)
	}

	// CostPerTask = (0.01+0.005+0.02+0.01+0.015) / 3 sessions = 0.06 / 3 = 0.02
	expectedCost := 0.02
	if fmt.Sprintf("%.4f", metrics.CostPerTask) != fmt.Sprintf("%.4f", expectedCost) {
		t.Errorf("expected CostPerTask=%.4f, got %.4f", expectedCost, metrics.CostPerTask)
	}
}

func TestGetMetrics_FiltersByAgentID(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		traces: []*trace.Trace{
			{ID: "t1", AgentID: "agent1", Status: trace.StatusAllowed, Timestamp: now.Add(-1 * time.Hour)},
			{ID: "t2", AgentID: "agent2", Status: trace.StatusDenied, Timestamp: now.Add(-2 * time.Hour)},
		},
		sessions: []*trace.Session{
			{ID: "s1", AgentID: "agent1", Status: "completed", StartedAt: now.Add(-1 * time.Hour)},
			{ID: "s2", AgentID: "agent2", Status: "active", StartedAt: now.Add(-2 * time.Hour)},
		},
	}
	analyzer := NewAnalyzer(store, nil)

	metrics, err := analyzer.GetMetrics("agent1", 24*time.Hour)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	// Should only include agent1 data (1 trace, 1 session).
	if metrics.TotalSessions != 1 {
		t.Errorf("expected TotalSessions=1 for agent1, got %d", metrics.TotalSessions)
	}
}

func TestGetMetrics_StoreError(t *testing.T) {
	store := &mockStore{
		listTracesErr: fmt.Errorf("database connection lost"),
	}
	analyzer := NewAnalyzer(store, nil)

	_, err := analyzer.GetMetrics("agent1", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error when store.ListTraces fails, got nil")
	}
	if err.Error() != "list traces: database connection lost" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetMetrics_SessionStoreError(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		traces: []*trace.Trace{
			{ID: "t1", AgentID: "agent1", Status: trace.StatusAllowed, Timestamp: now.Add(-1 * time.Hour)},
		},
		listSessionsErr: fmt.Errorf("session query failed"),
	}
	analyzer := NewAnalyzer(store, nil)

	_, err := analyzer.GetMetrics("agent1", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error when store.ListSessions fails, got nil")
	}
	if err.Error() != "list sessions: session query failed" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGetRecentFailures_NoFailures(t *testing.T) {
	store := &mockStore{
		traces: []*trace.Trace{},
	}
	analyzer := NewAnalyzer(store, nil)

	failures, err := analyzer.GetRecentFailures("agent1", 10)
	if err != nil {
		t.Fatalf("GetRecentFailures failed: %v", err)
	}

	if len(failures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(failures))
	}
}

func TestGetRecentFailures_WithFailures(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		traces: []*trace.Trace{
			{ID: "t1", AgentID: "agent1", Status: trace.StatusDenied, Timestamp: now.Add(-1 * time.Hour)},
			{ID: "t2", AgentID: "agent1", Status: trace.StatusTerminated, Timestamp: now.Add(-2 * time.Hour)},
			{ID: "t3", AgentID: "agent1", Status: trace.StatusThrottled, Timestamp: now.Add(-3 * time.Hour)},
			{ID: "t4", AgentID: "agent1", Status: trace.StatusAllowed, Timestamp: now.Add(-4 * time.Hour)}, // Not a failure.
		},
	}
	analyzer := NewAnalyzer(store, nil)

	failures, err := analyzer.GetRecentFailures("agent1", 10)
	if err != nil {
		t.Fatalf("GetRecentFailures failed: %v", err)
	}

	if len(failures) != 3 {
		t.Errorf("expected 3 failures, got %d", len(failures))
	}

	// Should be sorted newest-first.
	if failures[0].ID != "t1" {
		t.Errorf("expected first failure ID=t1, got %s", failures[0].ID)
	}
	if failures[1].ID != "t2" {
		t.Errorf("expected second failure ID=t2, got %s", failures[1].ID)
	}
	if failures[2].ID != "t3" {
		t.Errorf("expected third failure ID=t3, got %s", failures[2].ID)
	}
}

func TestGetRecentFailures_EnforcesLimit(t *testing.T) {
	now := time.Now()
	store := &mockStore{
		traces: []*trace.Trace{
			{ID: "t1", AgentID: "agent1", Status: trace.StatusDenied, Timestamp: now.Add(-1 * time.Hour)},
			{ID: "t2", AgentID: "agent1", Status: trace.StatusDenied, Timestamp: now.Add(-2 * time.Hour)},
			{ID: "t3", AgentID: "agent1", Status: trace.StatusDenied, Timestamp: now.Add(-3 * time.Hour)},
			{ID: "t4", AgentID: "agent1", Status: trace.StatusDenied, Timestamp: now.Add(-4 * time.Hour)},
			{ID: "t5", AgentID: "agent1", Status: trace.StatusDenied, Timestamp: now.Add(-5 * time.Hour)},
		},
	}
	analyzer := NewAnalyzer(store, nil)

	failures, err := analyzer.GetRecentFailures("agent1", 3)
	if err != nil {
		t.Fatalf("GetRecentFailures failed: %v", err)
	}

	if len(failures) != 3 {
		t.Errorf("expected limit of 3 failures, got %d", len(failures))
	}

	// Should be the 3 most recent (t1, t2, t3).
	if failures[0].ID != "t1" || failures[1].ID != "t2" || failures[2].ID != "t3" {
		t.Errorf("expected failures t1, t2, t3 in order, got %s, %s, %s",
			failures[0].ID, failures[1].ID, failures[2].ID)
	}
}

func TestGetRecentFailures_StoreError(t *testing.T) {
	store := &mockStore{
		listTracesErr: fmt.Errorf("database connection lost"),
	}
	analyzer := NewAnalyzer(store, nil)

	_, err := analyzer.GetRecentFailures("agent1", 10)
	if err == nil {
		t.Fatal("expected error when store.ListTraces fails, got nil")
	}
}

func TestSortTracesByTimestamp(t *testing.T) {
	now := time.Now()
	traces := []trace.Trace{
		{ID: "t1", Timestamp: now.Add(-5 * time.Hour)},
		{ID: "t2", Timestamp: now.Add(-1 * time.Hour)},
		{ID: "t3", Timestamp: now.Add(-3 * time.Hour)},
		{ID: "t4", Timestamp: now.Add(-2 * time.Hour)},
	}

	sortTracesByTimestamp(traces)

	// Should be sorted newest-first (t2, t4, t3, t1).
	expected := []string{"t2", "t4", "t3", "t1"}
	for i, exp := range expected {
		if traces[i].ID != exp {
			t.Errorf("position %d: expected ID=%s, got %s", i, exp, traces[i].ID)
		}
	}
}

func TestSortTracesByTimestamp_EmptySlice(t *testing.T) {
	traces := []trace.Trace{}
	sortTracesByTimestamp(traces) // Should not panic.
	if len(traces) != 0 {
		t.Error("empty slice should remain empty")
	}
}

func TestSortTracesByTimestamp_SingleElement(t *testing.T) {
	traces := []trace.Trace{{ID: "t1", Timestamp: time.Now()}}
	sortTracesByTimestamp(traces) // Should not panic.
	if traces[0].ID != "t1" {
		t.Error("single element slice should remain unchanged")
	}
}

func TestParseAnalysisResponse_FullResponse(t *testing.T) {
	response := `FAILURE_PATTERNS:
- Pattern 1
- Pattern 2

RECOMMENDATIONS:
- Recommendation 1
- Recommendation 2

PRIORITY: High priority item

ANALYSIS:
Detailed reasoning here.
More analysis.
`

	result := parseAnalysisResponse(response)

	if result.RawAnalysis != response {
		t.Error("RawAnalysis not preserved")
	}

	if len(result.FailurePatterns) != 2 {
		t.Errorf("expected 2 failure patterns, got %d", len(result.FailurePatterns))
	}
	if result.FailurePatterns[0] != "Pattern 1" || result.FailurePatterns[1] != "Pattern 2" {
		t.Errorf("failure patterns not parsed correctly: %v", result.FailurePatterns)
	}

	if len(result.Recommendations) != 2 {
		t.Errorf("expected 2 recommendations, got %d", len(result.Recommendations))
	}
	if result.Recommendations[0] != "Recommendation 1" || result.Recommendations[1] != "Recommendation 2" {
		t.Errorf("recommendations not parsed correctly: %v", result.Recommendations)
	}

	if result.Priority != "High priority item" {
		t.Errorf("expected Priority='High priority item', got '%s'", result.Priority)
	}
}

func TestParseAnalysisResponse_EmptyResponse(t *testing.T) {
	response := ""
	result := parseAnalysisResponse(response)

	if len(result.FailurePatterns) != 0 {
		t.Errorf("expected 0 failure patterns, got %d", len(result.FailurePatterns))
	}
	if len(result.Recommendations) != 0 {
		t.Errorf("expected 0 recommendations, got %d", len(result.Recommendations))
	}
	if result.Priority != "" {
		t.Errorf("expected empty Priority, got '%s'", result.Priority)
	}
}

func TestParseAnalysisResponse_NoSections(t *testing.T) {
	response := "Just some random text without any sections."
	result := parseAnalysisResponse(response)

	if len(result.FailurePatterns) != 0 {
		t.Errorf("expected 0 failure patterns, got %d", len(result.FailurePatterns))
	}
	if len(result.Recommendations) != 0 {
		t.Errorf("expected 0 recommendations, got %d", len(result.Recommendations))
	}
	if result.Priority != "" {
		t.Errorf("expected empty Priority, got '%s'", result.Priority)
	}
}

func TestAnalyze_Success(t *testing.T) {
	llmResponse := `FAILURE_PATTERNS:
- Loop detected in tool calls
- Cost exceeded budget

RECOMMENDATIONS:
- Add explicit loop breaking logic
- Reduce context window size

PRIORITY: Fix loop detection first

ANALYSIS:
The agent is repeatedly calling the same tool without making progress.
`

	store := &mockStore{}
	llm := &mockLLM{response: llmResponse}
	analyzer := NewAnalyzer(store, llm)

	now := time.Now()
	input := AnalysisInput{
		AgentID:  "test-agent",
		AgentMD:  "# Test Agent\nA test agent for testing.",
		EvolveMD: "# Evolution Rules\nImprove gradually.",
		PromptMD: "You are a helpful assistant.",
		Metrics: &AgentMetrics{
			CompletionRate:    0.8,
			ErrorRate:         0.1,
			HumanOverrideRate: 0.05,
			CostPerTask:       0.02,
			AvgLatency:        150,
			TotalSessions:     100,
			Window:            24 * time.Hour,
		},
		RecentFailures: []trace.Trace{
			{
				ID:           "fail1",
				SessionID:    "session1",
				AgentID:      "test-agent",
				ActionType:   trace.ActionToolCall,
				ActionName:   "search",
				Status:       trace.StatusDenied,
				PolicyName:   "loop-detector",
				PolicyReason: "Too many identical tool calls",
				Timestamp:    now.Add(-1 * time.Hour),
				RequestBody:  json.RawMessage(`{"query": "test"}`),
			},
		},
	}

	result, err := analyzer.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(result.FailurePatterns) != 2 {
		t.Errorf("expected 2 failure patterns, got %d", len(result.FailurePatterns))
	}
	if result.FailurePatterns[0] != "Loop detected in tool calls" {
		t.Errorf("unexpected first failure pattern: %s", result.FailurePatterns[0])
	}

	if len(result.Recommendations) != 2 {
		t.Errorf("expected 2 recommendations, got %d", len(result.Recommendations))
	}

	if result.Priority != "Fix loop detection first" {
		t.Errorf("unexpected priority: %s", result.Priority)
	}

	if result.RawAnalysis != llmResponse {
		t.Error("RawAnalysis not preserved")
	}
}

func TestAnalyze_LLMError(t *testing.T) {
	store := &mockStore{}
	llm := &mockLLM{err: fmt.Errorf("API rate limit exceeded")}
	analyzer := NewAnalyzer(store, llm)

	input := AnalysisInput{
		AgentID:  "test-agent",
		AgentMD:  "# Test Agent",
		EvolveMD: "# Evolution Rules",
		PromptMD: "You are helpful.",
	}

	_, err := analyzer.Analyze(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when LLM fails, got nil")
	}
	if err.Error() != "llm analysis failed: API rate limit exceeded" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAnalyze_NoMetrics(t *testing.T) {
	llmResponse := `FAILURE_PATTERNS:
- No patterns detected

RECOMMENDATIONS:
- Continue monitoring

PRIORITY: None

ANALYSIS:
Insufficient data.
`

	store := &mockStore{}
	llm := &mockLLM{response: llmResponse}
	analyzer := NewAnalyzer(store, llm)

	input := AnalysisInput{
		AgentID:        "test-agent",
		AgentMD:        "# Test Agent",
		EvolveMD:       "# Evolution Rules",
		PromptMD:       "You are helpful.",
		Metrics:        nil, // No metrics provided.
		RecentFailures: []trace.Trace{},
	}

	result, err := analyzer.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should still produce a result even without metrics.
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestAnalyze_TruncatesLongRequestBody(t *testing.T) {
	longBody := make([]byte, 2000)
	for i := range longBody {
		longBody[i] = 'x'
	}

	llmResponse := `FAILURE_PATTERNS:
- Long request

RECOMMENDATIONS:
- Reduce size

PRIORITY: High

ANALYSIS:
Request too large.
`

	store := &mockStore{}
	llm := &mockLLM{response: llmResponse}
	analyzer := NewAnalyzer(store, llm)

	now := time.Now()
	input := AnalysisInput{
		AgentID:  "test-agent",
		AgentMD:  "# Test Agent",
		EvolveMD: "# Evolution Rules",
		PromptMD: "You are helpful.",
		RecentFailures: []trace.Trace{
			{
				ID:          "fail1",
				Status:      trace.StatusDenied,
				Timestamp:   now,
				RequestBody: json.RawMessage(longBody),
			},
		},
	}

	result, err := analyzer.Analyze(context.Background(), input)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should complete without error (truncation happens internally).
	if result == nil {
		t.Fatal("expected result, got nil")
	}
}
