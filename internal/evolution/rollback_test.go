package evolution

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// mockStoreForRollback implements trace.Store for rollback testing.
type mockStoreForRollback struct {
	listTracesFn   func(filter trace.TraceFilter) ([]*trace.Trace, int, error)
	listSessionsFn func(filter trace.SessionFilter) ([]*trace.Session, int, error)
}

func (m *mockStoreForRollback) ListTraces(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
	if m.listTracesFn != nil {
		return m.listTracesFn(filter)
	}
	return nil, 0, nil
}

func (m *mockStoreForRollback) ListSessions(filter trace.SessionFilter) ([]*trace.Session, int, error) {
	if m.listSessionsFn != nil {
		return m.listSessionsFn(filter)
	}
	return nil, 0, nil
}

// Stub methods for unused Store interface methods.
func (m *mockStoreForRollback) Initialize() error                              { return nil }
func (m *mockStoreForRollback) Close() error                                   { return nil }
func (m *mockStoreForRollback) InsertTrace(t *trace.Trace) error               { return nil }
func (m *mockStoreForRollback) GetTrace(id string) (*trace.Trace, error)       { return nil, nil }
func (m *mockStoreForRollback) SearchTraces(query string, limit int) ([]*trace.Trace, error) { return nil, nil }
func (m *mockStoreForRollback) UpsertSession(s *trace.Session) error           { return nil }
func (m *mockStoreForRollback) GetSession(id string) (*trace.Session, error)   { return nil, nil }
func (m *mockStoreForRollback) UpdateSessionStatus(id, status string) error    { return nil }
func (m *mockStoreForRollback) UpdateSessionCost(id string, cost float64, actionCount int) error { return nil }
func (m *mockStoreForRollback) ScoreSession(id string, score []byte) error     { return nil }
func (m *mockStoreForRollback) UpsertAgent(a *trace.Agent) error               { return nil }
func (m *mockStoreForRollback) GetAgent(id string) (*trace.Agent, error)       { return nil, nil }
func (m *mockStoreForRollback) ListAgents() ([]*trace.Agent, error)            { return nil, nil }
func (m *mockStoreForRollback) GetAgentStats(agentID string) (*trace.AgentStats, error) { return nil, nil }
func (m *mockStoreForRollback) InsertAgentVersion(v *trace.AgentVersion) error { return nil }
func (m *mockStoreForRollback) GetAgentVersion(id string) (*trace.AgentVersion, error) { return nil, nil }
func (m *mockStoreForRollback) ListAgentVersions(agentID string) ([]*trace.AgentVersion, error) { return nil, nil }
func (m *mockStoreForRollback) InsertApproval(a *trace.Approval) error         { return nil }
func (m *mockStoreForRollback) GetApproval(id string) (*trace.Approval, error) { return nil, nil }
func (m *mockStoreForRollback) ListPendingApprovals() ([]*trace.Approval, error) { return nil, nil }
func (m *mockStoreForRollback) ResolveApproval(id, status, resolvedBy string) error { return nil }
func (m *mockStoreForRollback) InsertViolation(v *trace.Violation) error       { return nil }
func (m *mockStoreForRollback) ListViolations(agentID string, limit int) ([]*trace.Violation, error) { return nil, nil }
func (m *mockStoreForRollback) PruneOlderThan(days int) (int64, error)         { return 0, nil }
func (m *mockStoreForRollback) VerifyHashChain(sessionID string) (bool, int, error) { return true, 0, nil }
func (m *mockStoreForRollback) GetSystemStats() (*trace.SystemStats, error)    { return nil, nil }

// mockLLMForRollback implements LLMChatClient for rollback testing.
type mockLLMForRollback struct {
	response string
	err      error
}

func (m *mockLLMForRollback) Chat(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestNewRollbackMonitor(t *testing.T) {
	store := &mockStoreForRollback{}
	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    true,
		Trigger: "error_rate increases by 10% within 1h",
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)

	if rm == nil {
		t.Fatal("expected non-nil RollbackMonitor")
	}
	if rm.versions != vm {
		t.Error("expected versions to be set")
	}
	if rm.analyzer != analyzer {
		t.Error("expected analyzer to be set")
	}
	if !rm.config.Auto {
		t.Error("expected Auto to be true")
	}
	if rm.logger == nil {
		t.Error("expected default logger to be set")
	}
}

func TestNewRollbackMonitor_WithLogger(t *testing.T) {
	store := &mockStoreForRollback{}
	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	config := RollbackMonitorConfig{Auto: false}
	rm := NewRollbackMonitor(store, vm, analyzer, config, logger)

	if rm.logger != logger {
		t.Error("expected custom logger to be set")
	}
}

func TestParseTrigger_Valid(t *testing.T) {
	tests := []struct {
		name              string
		trigger           string
		expectedMetric    string
		expectedDirection string
		expectedThreshold float64
		expectedWindow    time.Duration
	}{
		{
			name:              "error_rate increases",
			trigger:           "error_rate increases by 10% within 1h",
			expectedMetric:    "error_rate",
			expectedDirection: "increases",
			expectedThreshold: 10.0,
			expectedWindow:    time.Hour,
		},
		{
			name:              "completion_rate decreases",
			trigger:           "completion_rate decreases by 15% within 30m",
			expectedMetric:    "completion_rate",
			expectedDirection: "decreases",
			expectedThreshold: 15.0,
			expectedWindow:    30 * time.Minute,
		},
		{
			name:              "cost_per_task increases",
			trigger:           "cost_per_task increases by 25.5% within 2h",
			expectedMetric:    "cost_per_task",
			expectedDirection: "increases",
			expectedThreshold: 25.5,
			expectedWindow:    2 * time.Hour,
		},
		{
			name:              "threshold without percent sign",
			trigger:           "error_rate increases by 5 within 1h",
			expectedMetric:    "error_rate",
			expectedDirection: "increases",
			expectedThreshold: 5.0,
			expectedWindow:    time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger, err := parseTrigger(tt.trigger)
			if err != nil {
				t.Fatalf("parseTrigger failed: %v", err)
			}

			if trigger.Metric != tt.expectedMetric {
				t.Errorf("Metric = %q, want %q", trigger.Metric, tt.expectedMetric)
			}
			if trigger.Direction != tt.expectedDirection {
				t.Errorf("Direction = %q, want %q", trigger.Direction, tt.expectedDirection)
			}
			if trigger.Threshold != tt.expectedThreshold {
				t.Errorf("Threshold = %v, want %v", trigger.Threshold, tt.expectedThreshold)
			}
			if trigger.Window != tt.expectedWindow {
				t.Errorf("Window = %v, want %v", trigger.Window, tt.expectedWindow)
			}
		})
	}
}

func TestParseTrigger_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		trigger     string
		errContains string
	}{
		{
			name:        "empty string",
			trigger:     "",
			errContains: "empty trigger string",
		},
		{
			name:        "too few parts",
			trigger:     "error_rate increases by 10%",
			errContains: "trigger must have format",
		},
		{
			name:        "invalid direction",
			trigger:     "error_rate rises by 10% within 1h",
			errContains: "unknown direction",
		},
		{
			name:        "missing 'by'",
			trigger:     "error_rate increases to 10% within 1h",
			errContains: "expected 'by'",
		},
		{
			name:        "invalid threshold",
			trigger:     "error_rate increases by abc% within 1h",
			errContains: "parse threshold",
		},
		{
			name:        "missing 'within'",
			trigger:     "error_rate increases by 10% during 1h",
			errContains: "expected 'within'",
		},
		{
			name:        "invalid duration",
			trigger:     "error_rate increases by 10% within 1hour",
			errContains: "parse window duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger, err := parseTrigger(tt.trigger)
			if err == nil {
				t.Fatalf("expected error, got trigger: %+v", trigger)
			}
			if !contains(err.Error(), tt.errContains) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.errContains)
			}
		})
	}
}

func TestGetMetricValue(t *testing.T) {
	metrics := &AgentMetrics{
		CompletionRate:    0.95,
		ErrorRate:         0.05,
		HumanOverrideRate: 0.10,
		CostPerTask:       0.02,
		AvgLatency:        150.0,
		TotalSessions:     100,
		Window:            time.Hour,
	}

	tests := []struct {
		name     string
		metric   string
		expected float64
	}{
		{"completion_rate", "completion_rate", 0.95},
		{"error_rate", "error_rate", 0.05},
		{"human_override_rate", "human_override_rate", 0.10},
		{"cost_per_task", "cost_per_task", 0.02},
		{"avg_latency", "avg_latency", 150.0},
		{"unknown_metric", "unknown_metric", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := getMetricValue(metrics, tt.metric)
			if val != tt.expected {
				t.Errorf("getMetricValue(%q) = %v, want %v", tt.metric, val, tt.expected)
			}
		})
	}
}

func TestRollbackMonitor_Check_AutoDisabled(t *testing.T) {
	store := &mockStoreForRollback{}
	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    false, // disabled
		Trigger: "error_rate increases by 10% within 1h",
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)
	shouldRollback, reason, err := rm.Check("agent-1")

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if shouldRollback {
		t.Error("expected shouldRollback=false when Auto is disabled")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestRollbackMonitor_Check_InvalidTrigger(t *testing.T) {
	store := &mockStoreForRollback{}
	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    true,
		Trigger: "invalid trigger format",
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)
	shouldRollback, reason, err := rm.Check("agent-1")

	if err == nil {
		t.Fatal("expected error for invalid trigger")
	}
	if shouldRollback {
		t.Error("expected shouldRollback=false on error")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestRollbackMonitor_Check_NotEnoughSessions(t *testing.T) {
	// Mock store that returns < 5 sessions for current metrics.
	store := &mockStoreForRollback{
		listSessionsFn: func(filter trace.SessionFilter) ([]*trace.Session, int, error) {
			// Return only 3 sessions (below threshold of 5).
			sessions := []*trace.Session{
				{ID: "s1", Status: "completed"},
				{ID: "s2", Status: "completed"},
				{ID: "s3", Status: "active"},
			}
			return sessions, len(sessions), nil
		},
		listTracesFn: func(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
			return []*trace.Trace{}, 0, nil
		},
	}

	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    true,
		Trigger: "error_rate increases by 10% within 1h",
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)
	shouldRollback, reason, err := rm.Check("agent-1")

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if shouldRollback {
		t.Error("expected shouldRollback=false when not enough sessions")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestRollbackMonitor_Check_BaselineZero(t *testing.T) {
	// Mock store that returns sessions but all have zero error rate in baseline.
	callCount := 0
	store := &mockStoreForRollback{
		listSessionsFn: func(filter trace.SessionFilter) ([]*trace.Session, int, error) {
			// Return enough sessions for both current and baseline windows.
			sessions := []*trace.Session{
				{ID: "s1", Status: "completed"},
				{ID: "s2", Status: "completed"},
				{ID: "s3", Status: "completed"},
				{ID: "s4", Status: "completed"},
				{ID: "s5", Status: "completed"},
				{ID: "s6", Status: "completed"},
			}
			return sessions, len(sessions), nil
		},
		listTracesFn: func(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
			// No errors in baseline (all traces allowed) → error_rate = 0.
			return []*trace.Trace{}, 0, nil
		},
	}

	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    true,
		Trigger: "error_rate increases by 10% within 1h",
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)
	shouldRollback, reason, err := rm.Check("agent-1")

	// When baseline is zero, Check returns early with no rollback.
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if shouldRollback {
		t.Error("expected shouldRollback=false when baseline is zero")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}

	_ = callCount // suppress unused warning
}

func TestRollbackMonitor_Check_ErrorRateIncreases_TriggersRollback(t *testing.T) {
	// Baseline: error_rate = 0.05 (5%) = 1 error / 20 total traces
	// Current: error_rate = 0.20 (20%) = 2 errors / 10 total traces
	// Change: +300% (exceeds threshold of 10%)
	callCount := 0
	store := &mockStoreForRollback{
		listSessionsFn: func(filter trace.SessionFilter) ([]*trace.Session, int, error) {
			// Return 10 sessions total.
			sessions := []*trace.Session{
				{ID: "s1", Status: "completed", AgentID: "agent-1"},
				{ID: "s2", Status: "completed", AgentID: "agent-1"},
				{ID: "s3", Status: "completed", AgentID: "agent-1"},
				{ID: "s4", Status: "completed", AgentID: "agent-1"},
				{ID: "s5", Status: "completed", AgentID: "agent-1"},
				{ID: "s6", Status: "completed", AgentID: "agent-1"},
				{ID: "s7", Status: "completed", AgentID: "agent-1"},
				{ID: "s8", Status: "completed", AgentID: "agent-1"},
				{ID: "s9", Status: "completed", AgentID: "agent-1"},
				{ID: "s10", Status: "completed", AgentID: "agent-1"},
			}
			return sessions, len(sessions), nil
		},
		listTracesFn: func(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
			callCount++
			// First call (current window): 2 errors + 8 allowed = 10 total, error_rate = 2/10 = 20%.
			if callCount == 1 {
				traces := []*trace.Trace{
					{ID: "t1", Status: trace.StatusDenied, AgentID: "agent-1"},
					{ID: "t2", Status: trace.StatusDenied, AgentID: "agent-1"},
					{ID: "t3", Status: trace.StatusAllowed, AgentID: "agent-1"},
					{ID: "t4", Status: trace.StatusAllowed, AgentID: "agent-1"},
					{ID: "t5", Status: trace.StatusAllowed, AgentID: "agent-1"},
					{ID: "t6", Status: trace.StatusAllowed, AgentID: "agent-1"},
					{ID: "t7", Status: trace.StatusAllowed, AgentID: "agent-1"},
					{ID: "t8", Status: trace.StatusAllowed, AgentID: "agent-1"},
					{ID: "t9", Status: trace.StatusAllowed, AgentID: "agent-1"},
					{ID: "t10", Status: trace.StatusAllowed, AgentID: "agent-1"},
				}
				return traces, len(traces), nil
			}
			// Second call (baseline window, 2x duration): 1 error + 19 allowed = 20 total, error_rate = 1/20 = 5%.
			traces := []*trace.Trace{
				{ID: "t11", Status: trace.StatusDenied, AgentID: "agent-1"},
				{ID: "t12", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t13", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t14", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t15", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t16", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t17", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t18", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t19", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t20", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t21", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t22", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t23", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t24", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t25", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t26", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t27", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t28", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t29", Status: trace.StatusAllowed, AgentID: "agent-1"},
				{ID: "t30", Status: trace.StatusAllowed, AgentID: "agent-1"},
			}
			return traces, len(traces), nil
		},
	}

	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    true,
		Trigger: "error_rate increases by 10% within 1h",
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)
	shouldRollback, reason, err := rm.Check("agent-1")

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !shouldRollback {
		t.Error("expected shouldRollback=true when error_rate increases significantly")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
	if !contains(reason, "error_rate increased") {
		t.Errorf("reason should mention 'error_rate increased', got: %s", reason)
	}
}

func TestRollbackMonitor_Check_CompletionRateDecreases_TriggersRollback(t *testing.T) {
	// Baseline: completion_rate = 0.90 (90%) = 9 completed / 10 total
	// Current: completion_rate = 0.70 (70%) = 7 completed / 10 total
	// Change: -22.2% (exceeds threshold of 15%)
	callCount := 0
	store := &mockStoreForRollback{
		listSessionsFn: func(filter trace.SessionFilter) ([]*trace.Session, int, error) {
			callCount++
			// First call (current window): 7 completed / 10 total = 70%.
			if callCount == 1 {
				sessions := []*trace.Session{
					{ID: "s1", Status: "completed", AgentID: "agent-1"},
					{ID: "s2", Status: "completed", AgentID: "agent-1"},
					{ID: "s3", Status: "completed", AgentID: "agent-1"},
					{ID: "s4", Status: "completed", AgentID: "agent-1"},
					{ID: "s5", Status: "completed", AgentID: "agent-1"},
					{ID: "s6", Status: "completed", AgentID: "agent-1"},
					{ID: "s7", Status: "completed", AgentID: "agent-1"},
					{ID: "s8", Status: "active", AgentID: "agent-1"},
					{ID: "s9", Status: "active", AgentID: "agent-1"},
					{ID: "s10", Status: "active", AgentID: "agent-1"},
				}
				return sessions, len(sessions), nil
			}
			// Second call (baseline window): 9 completed / 10 total = 90%.
			sessions := []*trace.Session{
				{ID: "s11", Status: "completed", AgentID: "agent-1"},
				{ID: "s12", Status: "completed", AgentID: "agent-1"},
				{ID: "s13", Status: "completed", AgentID: "agent-1"},
				{ID: "s14", Status: "completed", AgentID: "agent-1"},
				{ID: "s15", Status: "completed", AgentID: "agent-1"},
				{ID: "s16", Status: "completed", AgentID: "agent-1"},
				{ID: "s17", Status: "completed", AgentID: "agent-1"},
				{ID: "s18", Status: "completed", AgentID: "agent-1"},
				{ID: "s19", Status: "completed", AgentID: "agent-1"},
				{ID: "s20", Status: "active", AgentID: "agent-1"},
			}
			return sessions, len(sessions), nil
		},
		listTracesFn: func(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
			// Need at least one trace so totalCount > 0 (otherwise GetMetrics returns early).
			traces := []*trace.Trace{
				{ID: "t1", Status: trace.StatusAllowed, AgentID: "agent-1"},
			}
			return traces, len(traces), nil
		},
	}

	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    true,
		Trigger: "completion_rate decreases by 15% within 30m",
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)
	shouldRollback, reason, err := rm.Check("agent-1")

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !shouldRollback {
		t.Error("expected shouldRollback=true when completion_rate decreases significantly")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
	if !contains(reason, "completion_rate decreased") {
		t.Errorf("reason should mention 'completion_rate decreased', got: %s", reason)
	}
}

func TestRollbackMonitor_Check_BelowThreshold_NoRollback(t *testing.T) {
	// Baseline: error_rate = 0.05 (5%)
	// Current: error_rate = 0.06 (6%)
	// Change: +20% (below threshold of 50%)
	callCount := 0
	store := &mockStoreForRollback{
		listSessionsFn: func(filter trace.SessionFilter) ([]*trace.Session, int, error) {
			sessions := []*trace.Session{
				{ID: "s1", Status: "completed", AgentID: "agent-1"},
				{ID: "s2", Status: "completed", AgentID: "agent-1"},
				{ID: "s3", Status: "completed", AgentID: "agent-1"},
				{ID: "s4", Status: "completed", AgentID: "agent-1"},
				{ID: "s5", Status: "completed", AgentID: "agent-1"},
				{ID: "s6", Status: "completed", AgentID: "agent-1"},
			}
			return sessions, len(sessions), nil
		},
		listTracesFn: func(filter trace.TraceFilter) ([]*trace.Trace, int, error) {
			callCount++
			// Current: 6% error rate.
			if callCount == 1 {
				traces := []*trace.Trace{
					{ID: "t1", Status: "denied", AgentID: "agent-1"},
				}
				return traces, len(traces), nil
			}
			// Baseline: 5% error rate.
			traces := []*trace.Trace{
				{ID: "t2", Status: "denied", AgentID: "agent-1"},
			}
			return traces, len(traces), nil
		},
	}

	llm := &mockLLMForRollback{}
	vm := NewVersionManager(t.TempDir())
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{
		Auto:    true,
		Trigger: "error_rate increases by 50% within 1h", // High threshold.
	}

	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)
	shouldRollback, reason, err := rm.Check("agent-1")

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if shouldRollback {
		t.Error("expected shouldRollback=false when change is below threshold")
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestRollbackMonitor_ExecuteRollback_Success(t *testing.T) {
	// Create temp dir with version directories.
	agentsDir := t.TempDir()
	agentDir := filepath.Join(agentsDir, "agent-1")
	versionsDir := filepath.Join(agentDir, "versions")
	_ = os.MkdirAll(versionsDir, 0755)
	_ = os.Mkdir(filepath.Join(versionsDir, "v1"), 0755)
	_ = os.Mkdir(filepath.Join(versionsDir, "v2"), 0755)
	_ = os.Mkdir(filepath.Join(versionsDir, "v3"), 0755)

	store := &mockStoreForRollback{}
	llm := &mockLLMForRollback{}
	vm := NewVersionManager(agentsDir)
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{Auto: true}
	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)

	rolledBackTo, err := rm.ExecuteRollback("agent-1", "test degradation")
	if err != nil {
		t.Fatalf("ExecuteRollback failed: %v", err)
	}

	// Should roll back from v3 to v2.
	if rolledBackTo != "v2" {
		t.Errorf("rolledBackTo = %q, want %q", rolledBackTo, "v2")
	}

	// Verify v3 renamed to v3-rolledback.
	if _, err := os.Stat(filepath.Join(versionsDir, "v3-rolledback")); err != nil {
		t.Errorf("expected v3-rolledback directory to exist: %v", err)
	}
}

func TestRollbackMonitor_ExecuteRollback_InsufficientVersions(t *testing.T) {
	// Create temp dir with only one version (cannot rollback).
	agentsDir := t.TempDir()
	agentDir := filepath.Join(agentsDir, "agent-1")
	versionsDir := filepath.Join(agentDir, "versions")
	_ = os.MkdirAll(versionsDir, 0755)
	_ = os.Mkdir(filepath.Join(versionsDir, "v1"), 0755)

	store := &mockStoreForRollback{}
	llm := &mockLLMForRollback{}
	vm := NewVersionManager(agentsDir)
	analyzer := NewAnalyzer(store, llm)

	config := RollbackMonitorConfig{Auto: true}
	rm := NewRollbackMonitor(store, vm, analyzer, config, nil)

	rolledBackTo, err := rm.ExecuteRollback("agent-1", "test degradation")
	if err == nil {
		t.Fatal("expected error when insufficient versions for rollback")
	}
	if rolledBackTo != "" {
		t.Errorf("expected empty rolledBackTo, got %q", rolledBackTo)
	}
}

// Helper function for substring matching.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
