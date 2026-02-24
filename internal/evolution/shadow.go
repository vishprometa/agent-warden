package evolution

import (
	"fmt"
	"sync"
	"time"
)

// ShadowRunner manages shadow tests that fork actions to both the current
// and candidate PROMPT.md versions, then compare their outputs. This enables
// safe evaluation of prompt changes before promotion.
type ShadowRunner struct {
	mu        sync.RWMutex
	active    map[string]*ShadowTest // agentID -> active test
	minRuns   int
	threshold float64
}

// ShadowTest tracks an ongoing shadow comparison between two versions.
type ShadowTest struct {
	AgentID          string
	CurrentVersion   string
	CandidateVersion string
	CurrentResults   []ShadowResult
	CandidateResults []ShadowResult
	StartedAt        time.Time
	MinRuns          int
	Threshold        float64
}

// ShadowResult records the outcome of a single shadow execution.
type ShadowResult struct {
	TraceID    string
	ActionType string
	Verdict    string  // allow, deny, etc.
	CostUSD    float64
	LatencyMS  int
	Success    bool
	Timestamp  time.Time
}

// ShadowProgress reports the current state of a shadow test.
type ShadowProgress struct {
	CurrentRuns      int
	MinRuns          int
	CurrentMetrics   ShadowMetrics
	CandidateMetrics ShadowMetrics
	ReadyToPromote   bool
}

// ShadowMetrics holds aggregate metrics for one side of a shadow test.
type ShadowMetrics struct {
	SuccessRate float64
	AvgCost     float64
	AvgLatency  float64
	ErrorRate   float64
}

// NewShadowRunner creates a ShadowRunner with the given minimum runs
// and success threshold for promotion eligibility.
func NewShadowRunner(minRuns int, threshold float64) *ShadowRunner {
	return &ShadowRunner{
		active:    make(map[string]*ShadowTest),
		minRuns:   minRuns,
		threshold: threshold,
	}
}

// StartShadowTest begins a shadow test for an agent. Returns an error if
// a test is already active for this agent.
func (s *ShadowRunner) StartShadowTest(agentID, currentVersion, candidateVersion string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.active[agentID]; exists {
		return fmt.Errorf("shadow test already active for agent %s", agentID)
	}

	s.active[agentID] = &ShadowTest{
		AgentID:          agentID,
		CurrentVersion:   currentVersion,
		CandidateVersion: candidateVersion,
		CurrentResults:   make([]ShadowResult, 0),
		CandidateResults: make([]ShadowResult, 0),
		StartedAt:        time.Now(),
		MinRuns:          s.minRuns,
		Threshold:        s.threshold,
	}

	return nil
}

// RecordResult records a shadow execution result for either the current
// or candidate version.
func (s *ShadowRunner) RecordResult(agentID string, isCandidate bool, result ShadowResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	test, exists := s.active[agentID]
	if !exists {
		return
	}

	if isCandidate {
		test.CandidateResults = append(test.CandidateResults, result)
	} else {
		test.CurrentResults = append(test.CurrentResults, result)
	}
}

// GetProgress returns the current shadow test progress for an agent.
func (s *ShadowRunner) GetProgress(agentID string) (*ShadowProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	test, exists := s.active[agentID]
	if !exists {
		return nil, fmt.Errorf("no active shadow test for agent %s", agentID)
	}

	currentMetrics := computeShadowMetrics(test.CurrentResults)
	candidateMetrics := computeShadowMetrics(test.CandidateResults)

	candidateRuns := len(test.CandidateResults)
	ready := candidateRuns >= test.MinRuns && candidateMetrics.SuccessRate >= test.Threshold

	return &ShadowProgress{
		CurrentRuns:      candidateRuns,
		MinRuns:          test.MinRuns,
		CurrentMetrics:   currentMetrics,
		CandidateMetrics: candidateMetrics,
		ReadyToPromote:   ready,
	}, nil
}

// IsReadyToPromote checks if the candidate has enough runs and meets
// the success threshold. Returns false with nil progress if no test is active.
func (s *ShadowRunner) IsReadyToPromote(agentID string) (bool, *ShadowProgress) {
	progress, err := s.GetProgress(agentID)
	if err != nil {
		return false, nil
	}
	return progress.ReadyToPromote, progress
}

// StopShadowTest ends a shadow test for the given agent.
func (s *ShadowRunner) StopShadowTest(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.active, agentID)
}

// HasActiveTest returns true if a shadow test is currently running for the agent.
func (s *ShadowRunner) HasActiveTest(agentID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.active[agentID]
	return exists
}

// computeShadowMetrics calculates aggregate metrics from a slice of shadow results.
func computeShadowMetrics(results []ShadowResult) ShadowMetrics {
	if len(results) == 0 {
		return ShadowMetrics{}
	}

	var (
		successCount int
		errorCount   int
		totalCost    float64
		totalLatency float64
	)

	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			errorCount++
		}
		totalCost += r.CostUSD
		totalLatency += float64(r.LatencyMS)
	}

	n := float64(len(results))
	return ShadowMetrics{
		SuccessRate: float64(successCount) / n,
		AvgCost:     totalCost / n,
		AvgLatency:  totalLatency / n,
		ErrorRate:   float64(errorCount) / n,
	}
}
