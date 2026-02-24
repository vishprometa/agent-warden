package evolution

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// Engine is the evolution orchestrator. It reads AGENT.md + EVOLVE.md,
// queries traces, calls the LLM for analysis, and generates candidate
// PROMPT.md diffs. This replaces the Python sidecar from v1.
type Engine struct {
	store    trace.Store
	loadMD   MDLoader
	shadow   *ShadowRunner
	proposer *Proposer
	analyzer *Analyzer
	versions *VersionManager
	model    string
	logger   *slog.Logger
}

// MDLoader abstracts the mdloader package for loading agent markdown files.
type MDLoader interface {
	LoadAgentMD(agentID string) (string, error)
	LoadEvolveMD(agentID string) (string, error)
	LoadPromptMD(agentID string, version string) (string, error)
	CurrentVersion(agentID string) (string, error)
	ListVersions(agentID string) ([]string, error)
}

// EvolutionResult captures the outcome of a single evolution cycle.
type EvolutionResult struct {
	AgentID          string    `json:"agent_id"`
	CurrentVersion   string    `json:"current_version"`
	CandidateVersion string    `json:"candidate_version"`
	Analysis         string    `json:"analysis"`       // LLM's analysis of failure patterns
	ProposedDiff     string    `json:"proposed_diff"`   // what changed
	ProposedPrompt   string    `json:"proposed_prompt"` // full new PROMPT.md content
	Status           string    `json:"status"`          // "proposed", "shadow_testing", "promoted", "rejected"
	CreatedAt        time.Time `json:"created_at"`
}

// EvolutionStatus reports the current state of evolution for an agent.
type EvolutionStatus struct {
	AgentID          string          `json:"agent_id"`
	CurrentVersion   string          `json:"current_version"`
	CandidateVersion string          `json:"candidate_version,omitempty"`
	ShadowProgress   *ShadowProgress `json:"shadow_progress,omitempty"`
	LastEvolution    time.Time       `json:"last_evolution"`
}

// NewEngine creates a fully wired evolution Engine.
func NewEngine(store trace.Store, md MDLoader, model string, agentsDir string, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}

	llm := NewLLMClient(model)
	analyzer := NewAnalyzer(store, llm)
	proposer := NewProposer(llm, agentsDir)
	shadow := NewShadowRunner(20, 0.8) // default: 20 min runs, 80% success threshold
	versions := NewVersionManager(agentsDir)

	return &Engine{
		store:    store,
		loadMD:   md,
		shadow:   shadow,
		proposer: proposer,
		analyzer: analyzer,
		versions: versions,
		model:    model,
		logger:   logger,
	}
}

// TriggerEvolution runs the full evolution loop for an agent:
//  1. Read AGENT.md + EVOLVE.md
//  2. Score current version (metrics from traces)
//  3. Identify failure patterns (LLM: MDs + traces)
//  4. Generate proposed diff -> new PROMPT.md version
//  5. Write candidate to disk
//  6. Return EvolutionResult (caller submits to policy check + shadow runner)
func (e *Engine) TriggerEvolution(ctx context.Context, agentID string) (*EvolutionResult, error) {
	e.logger.Info("starting evolution cycle", "agent_id", agentID)

	// Step 1: Load markdown files.
	agentMD, err := e.loadMD.LoadAgentMD(agentID)
	if err != nil {
		return nil, fmt.Errorf("load AGENT.md: %w", err)
	}

	evolveMD, err := e.loadMD.LoadEvolveMD(agentID)
	if err != nil {
		return nil, fmt.Errorf("load EVOLVE.md: %w", err)
	}

	currentVersion, err := e.loadMD.CurrentVersion(agentID)
	if err != nil {
		return nil, fmt.Errorf("get current version: %w", err)
	}

	promptMD, err := e.loadMD.LoadPromptMD(agentID, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("load PROMPT.md for %s: %w", currentVersion, err)
	}

	e.logger.Info("loaded agent context",
		"agent_id", agentID,
		"current_version", currentVersion,
		"agent_md_len", len(agentMD),
		"evolve_md_len", len(evolveMD),
		"prompt_md_len", len(promptMD),
	)

	// Step 2: Compute metrics.
	metrics, err := e.analyzer.GetMetrics(agentID, 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("get metrics: %w", err)
	}

	e.logger.Info("computed metrics",
		"agent_id", agentID,
		"completion_rate", metrics.CompletionRate,
		"error_rate", metrics.ErrorRate,
		"total_sessions", metrics.TotalSessions,
	)

	// Step 3: Get recent failures and run LLM analysis.
	failures, err := e.analyzer.GetRecentFailures(agentID, 50)
	if err != nil {
		return nil, fmt.Errorf("get recent failures: %w", err)
	}

	e.logger.Info("fetched failures", "agent_id", agentID, "count", len(failures))

	analysisResult, err := e.analyzer.Analyze(ctx, AnalysisInput{
		AgentID:        agentID,
		AgentMD:        agentMD,
		EvolveMD:       evolveMD,
		PromptMD:       promptMD,
		Metrics:        metrics,
		RecentFailures: failures,
	})
	if err != nil {
		return nil, fmt.Errorf("analyze: %w", err)
	}

	e.logger.Info("analysis complete",
		"agent_id", agentID,
		"failure_patterns", len(analysisResult.FailurePatterns),
		"recommendations", len(analysisResult.Recommendations),
		"priority", analysisResult.Priority,
	)

	// If no failure patterns found, skip proposal.
	if len(analysisResult.FailurePatterns) == 0 {
		e.logger.Info("no failure patterns found, skipping proposal", "agent_id", agentID)
		return &EvolutionResult{
			AgentID:        agentID,
			CurrentVersion: currentVersion,
			Analysis:       analysisResult.RawAnalysis,
			Status:         "no_changes_needed",
			CreatedAt:      time.Now(),
		}, nil
	}

	// Step 4: Generate proposed PROMPT.md.
	proposal, err := e.proposer.Propose(ctx, ProposalInput{
		AgentID:         agentID,
		AgentMD:         agentMD,
		EvolveMD:        evolveMD,
		CurrentPromptMD: promptMD,
		CurrentVersion:  currentVersion,
		Analysis:        analysisResult,
	})
	if err != nil {
		return nil, fmt.Errorf("propose: %w", err)
	}

	e.logger.Info("proposal generated",
		"agent_id", agentID,
		"candidate_version", proposal.CandidateVersion,
		"diff_len", len(proposal.Diff),
	)

	// Step 5: Write candidate to disk.
	if err := e.proposer.WriteCandidatePromptMD(agentID, proposal.CandidateVersion, proposal.NewPromptMD); err != nil {
		return nil, fmt.Errorf("write candidate: %w", err)
	}

	e.logger.Info("candidate written to disk",
		"agent_id", agentID,
		"candidate_version", proposal.CandidateVersion,
	)

	result := &EvolutionResult{
		AgentID:          agentID,
		CurrentVersion:   currentVersion,
		CandidateVersion: proposal.CandidateVersion,
		Analysis:         analysisResult.RawAnalysis,
		ProposedDiff:     proposal.Diff,
		ProposedPrompt:   proposal.NewPromptMD,
		Status:           "proposed",
		CreatedAt:        time.Now(),
	}

	return result, nil
}

// StartShadowTest transitions an evolution result into shadow testing.
func (e *Engine) StartShadowTest(result *EvolutionResult) error {
	if err := e.shadow.StartShadowTest(result.AgentID, result.CurrentVersion, result.CandidateVersion); err != nil {
		return fmt.Errorf("start shadow test: %w", err)
	}

	result.Status = "shadow_testing"

	e.logger.Info("shadow test started",
		"agent_id", result.AgentID,
		"current", result.CurrentVersion,
		"candidate", result.CandidateVersion,
	)

	return nil
}

// PromoteCandidate promotes the candidate version to active after shadow testing passes.
func (e *Engine) PromoteCandidate(agentID string) (string, error) {
	ready, progress := e.shadow.IsReadyToPromote(agentID)
	if !ready {
		if progress != nil {
			return "", fmt.Errorf(
				"candidate not ready: %d/%d runs, success rate %.2f%% (threshold: %.2f%%)",
				progress.CurrentRuns, progress.MinRuns,
				progress.CandidateMetrics.SuccessRate*100,
				80.0, // default threshold
			)
		}
		return "", fmt.Errorf("no active shadow test for agent %s", agentID)
	}

	promoted, err := e.versions.PromoteCandidate(agentID)
	if err != nil {
		return "", fmt.Errorf("promote candidate: %w", err)
	}

	e.shadow.StopShadowTest(agentID)

	e.logger.Info("candidate promoted",
		"agent_id", agentID,
		"promoted_version", promoted,
	)

	return promoted, nil
}

// GetStatus returns the current evolution status for an agent.
func (e *Engine) GetStatus(agentID string) (*EvolutionStatus, error) {
	currentVersion, err := e.versions.GetActiveVersion(agentID)
	if err != nil {
		return nil, fmt.Errorf("get active version: %w", err)
	}

	status := &EvolutionStatus{
		AgentID:        agentID,
		CurrentVersion: currentVersion,
	}

	// Check for active shadow test.
	if e.shadow.HasActiveTest(agentID) {
		progress, err := e.shadow.GetProgress(agentID)
		if err == nil {
			status.ShadowProgress = progress
		}
	}

	// Check for candidate version in version history.
	history, err := e.versions.GetVersionHistory(agentID)
	if err == nil {
		for _, v := range history {
			if v.Status == "candidate" {
				status.CandidateVersion = v.Version
				break
			}
		}
	}

	return status, nil
}

// GetShadowRunner returns the shadow runner for external recording of results.
func (e *Engine) GetShadowRunner() *ShadowRunner {
	return e.shadow
}

// GetVersionManager returns the version manager for external use.
func (e *Engine) GetVersionManager() *VersionManager {
	return e.versions
}
