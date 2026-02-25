package evolution

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// Analyzer queries traces and uses an LLM to identify failure patterns
// for a given agent. It combines AGENT.md, EVOLVE.md, and PROMPT.md context
// with real metrics and failure traces to produce actionable analysis.
type Analyzer struct {
	store trace.Store
	llm   *LLMClient
}

// AnalysisInput bundles all context the analyzer needs to produce a result.
type AnalysisInput struct {
	AgentID        string
	AgentMD        string // AGENT.md content
	EvolveMD       string // EVOLVE.md content
	PromptMD       string // current PROMPT.md
	Metrics        *AgentMetrics
	RecentFailures []trace.Trace
}

// AgentMetrics holds computed performance metrics for an agent over a time window.
type AgentMetrics struct {
	CompletionRate    float64       // fraction of sessions that completed successfully
	ErrorRate         float64       // fraction of traces with denied/terminated/error status
	HumanOverrideRate float64       // fraction of traces that needed human approval
	CostPerTask       float64       // average cost per session
	AvgLatency        float64       // average latency in milliseconds
	TotalSessions     int           // total sessions in the window
	Window            time.Duration // the time window these metrics cover
}

// AnalysisResult is the LLM-produced analysis of an agent's failure patterns.
type AnalysisResult struct {
	FailurePatterns []string // distinct failure patterns identified
	Recommendations []string // suggested prompt/config changes
	Priority        string   // what to fix first (derived from EVOLVE.md priorities)
	RawAnalysis     string   // the full LLM response for audit
}

// NewAnalyzer creates an Analyzer with the given trace store and LLM client.
func NewAnalyzer(store trace.Store, llm *LLMClient) *Analyzer {
	return &Analyzer{
		store: store,
		llm:   llm,
	}
}

// Analyze queries traces for the agent, combines them with MD context,
// and sends everything to the LLM for failure pattern analysis.
func (a *Analyzer) Analyze(ctx context.Context, input AnalysisInput) (*AnalysisResult, error) {
	systemPrompt := `You are an agent evolution analyst for AgentWarden. Your job is to analyze an AI agent's recent performance and identify failure patterns that can be fixed by modifying its system prompt (PROMPT.md).

You will receive:
1. AGENT.md - the agent's identity and capabilities
2. EVOLVE.md - evolution rules, priorities, and constraints
3. Current PROMPT.md - the agent's active system prompt
4. Performance metrics
5. Recent failure traces (denied, terminated, or error actions)

Respond with a structured analysis:

FAILURE_PATTERNS:
- [pattern 1]
- [pattern 2]
...

RECOMMENDATIONS:
- [specific prompt change 1]
- [specific prompt change 2]
...

PRIORITY: [what to fix first, based on EVOLVE.md priorities]

ANALYSIS:
[detailed reasoning]`

	var userMsg strings.Builder
	userMsg.WriteString("## AGENT.md\n")
	userMsg.WriteString(input.AgentMD)
	userMsg.WriteString("\n\n## EVOLVE.md\n")
	userMsg.WriteString(input.EvolveMD)
	userMsg.WriteString("\n\n## Current PROMPT.md\n")
	userMsg.WriteString(input.PromptMD)
	userMsg.WriteString("\n\n## Performance Metrics\n")

	if input.Metrics != nil {
		fmt.Fprintf(&userMsg, "- Completion Rate: %.2f%%\n", input.Metrics.CompletionRate*100)
		fmt.Fprintf(&userMsg, "- Error Rate: %.2f%%\n", input.Metrics.ErrorRate*100)
		fmt.Fprintf(&userMsg, "- Human Override Rate: %.2f%%\n", input.Metrics.HumanOverrideRate*100)
		fmt.Fprintf(&userMsg, "- Cost Per Task: $%.4f\n", input.Metrics.CostPerTask)
		fmt.Fprintf(&userMsg, "- Avg Latency: %.0fms\n", input.Metrics.AvgLatency)
		fmt.Fprintf(&userMsg, "- Total Sessions: %d\n", input.Metrics.TotalSessions)
		fmt.Fprintf(&userMsg, "- Window: %s\n", input.Metrics.Window)
	} else {
		userMsg.WriteString("No metrics available.\n")
	}

	userMsg.WriteString("\n## Recent Failures\n")
	if len(input.RecentFailures) == 0 {
		userMsg.WriteString("No recent failures.\n")
	}
	for i, t := range input.RecentFailures {
		fmt.Fprintf(&userMsg, "\n### Failure %d\n", i+1)
		fmt.Fprintf(&userMsg, "- Trace ID: %s\n", t.ID)
		fmt.Fprintf(&userMsg, "- Action: %s / %s\n", t.ActionType, t.ActionName)
		fmt.Fprintf(&userMsg, "- Status: %s\n", t.Status)
		fmt.Fprintf(&userMsg, "- Policy: %s\n", t.PolicyName)
		fmt.Fprintf(&userMsg, "- Reason: %s\n", t.PolicyReason)
		fmt.Fprintf(&userMsg, "- Timestamp: %s\n", t.Timestamp.Format(time.RFC3339))
		if len(t.RequestBody) > 0 && string(t.RequestBody) != "null" {
			body := string(t.RequestBody)
			if len(body) > 1000 {
				body = body[:1000] + "...[truncated]"
			}
			fmt.Fprintf(&userMsg, "- Request (truncated): %s\n", body)
		}
	}

	response, err := a.llm.Chat(ctx, systemPrompt, userMsg.String())
	if err != nil {
		return nil, fmt.Errorf("llm analysis failed: %w", err)
	}

	result := parseAnalysisResponse(response)
	return result, nil
}

// parseAnalysisResponse extracts structured fields from the LLM's response.
func parseAnalysisResponse(response string) *AnalysisResult {
	result := &AnalysisResult{
		RawAnalysis: response,
	}

	sections := map[string]*[]string{
		"FAILURE_PATTERNS:": &result.FailurePatterns,
		"RECOMMENDATIONS:":  &result.Recommendations,
	}

	lines := strings.Split(response, "\n")
	var currentSection *[]string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for section headers.
		for header, target := range sections {
			if strings.HasPrefix(trimmed, header) {
				currentSection = target
				break
			}
		}

		if strings.HasPrefix(trimmed, "PRIORITY:") {
			result.Priority = strings.TrimSpace(strings.TrimPrefix(trimmed, "PRIORITY:"))
			currentSection = nil
			continue
		}

		if strings.HasPrefix(trimmed, "ANALYSIS:") {
			currentSection = nil
			continue
		}

		// Collect bullet points into the current section.
		if currentSection != nil && strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimPrefix(trimmed, "- ")
			*currentSection = append(*currentSection, item)
		}
	}

	return result
}

// GetMetrics computes agent performance metrics from the trace store
// over the specified time window.
func (a *Analyzer) GetMetrics(agentID string, window time.Duration) (*AgentMetrics, error) {
	since := time.Now().Add(-window)

	// Fetch all traces for this agent in the window.
	traces, totalCount, err := a.store.ListTraces(trace.TraceFilter{
		AgentID: agentID,
		Since:   &since,
		Limit:   10000,
	})
	if err != nil {
		return nil, fmt.Errorf("list traces: %w", err)
	}

	if totalCount == 0 {
		return &AgentMetrics{
			Window: window,
		}, nil
	}

	// Fetch sessions for this agent in the window.
	sessions, _, err := a.store.ListSessions(trace.SessionFilter{
		AgentID: agentID,
		Since:   &since,
		Limit:   10000,
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var (
		errorCount        int
		approvedCount     int
		totalLatency      float64
		totalCost         float64
		completedSessions int
	)

	for _, t := range traces {
		switch t.Status {
		case trace.StatusDenied, trace.StatusTerminated:
			errorCount++
		case trace.StatusApproved:
			approvedCount++
		}
		totalLatency += float64(t.LatencyMs)
		totalCost += t.CostUSD
	}

	for _, s := range sessions {
		if s.Status == "completed" {
			completedSessions++
		}
	}

	totalTraces := float64(len(traces))
	totalSessions := len(sessions)

	metrics := &AgentMetrics{
		TotalSessions: totalSessions,
		Window:        window,
	}

	if totalTraces > 0 {
		metrics.ErrorRate = float64(errorCount) / totalTraces
		metrics.HumanOverrideRate = float64(approvedCount) / totalTraces
		metrics.AvgLatency = totalLatency / totalTraces
	}

	if totalSessions > 0 {
		metrics.CompletionRate = float64(completedSessions) / float64(totalSessions)
		metrics.CostPerTask = totalCost / float64(totalSessions)
	}

	return metrics, nil
}

// GetRecentFailures fetches recent traces with denied, terminated, or throttled status.
func (a *Analyzer) GetRecentFailures(agentID string, limit int) ([]trace.Trace, error) {
	var failures []trace.Trace

	for _, status := range []trace.TraceStatus{
		trace.StatusDenied,
		trace.StatusTerminated,
		trace.StatusThrottled,
	} {
		traces, _, err := a.store.ListTraces(trace.TraceFilter{
			AgentID: agentID,
			Status:  status,
			Limit:   limit,
		})
		if err != nil {
			return nil, fmt.Errorf("list %s traces: %w", status, err)
		}
		for _, t := range traces {
			failures = append(failures, *t)
		}
	}

	// Sort by timestamp descending and cap at limit.
	sortTracesByTimestamp(failures)
	if len(failures) > limit {
		failures = failures[:limit]
	}

	return failures, nil
}

// sortTracesByTimestamp sorts traces newest-first.
func sortTracesByTimestamp(traces []trace.Trace) {
	for i := 1; i < len(traces); i++ {
		for j := i; j > 0 && traces[j].Timestamp.After(traces[j-1].Timestamp); j-- {
			traces[j], traces[j-1] = traces[j-1], traces[j]
		}
	}
}
