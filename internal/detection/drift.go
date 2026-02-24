package detection

import (
	"math"
	"sort"
	"sync"
	"time"
)

// DriftDetector monitors agents for behavioral drift by comparing current
// action distributions against historical baselines using KL-divergence.
type DriftDetector struct {
	baselines map[string]*ActionDistribution // per agent
	current   map[string]*ActionDistribution // current window
	mu        sync.RWMutex
	window    time.Duration
	threshold float64 // KL-divergence threshold
}

// ActionDistribution tracks action type frequencies for an agent.
type ActionDistribution struct {
	Counts    map[string]int // action_type -> count
	Total     int
	UpdatedAt time.Time
}

// DriftResult is the outcome of a drift check.
type DriftResult struct {
	Drifted    bool
	Divergence float64            // KL-divergence score
	Expected   map[string]float64 // baseline distribution (probabilities)
	Actual     map[string]float64 // current distribution (probabilities)
	TopDrifts  []DriftDetail      // top drifting action types, sorted by |delta|
}

// DriftDetail describes drift for a single action type.
type DriftDetail struct {
	ActionType string
	Expected   float64
	Actual     float64
	Delta      float64 // Actual - Expected
}

// NewDriftDetector creates a DriftDetector.
//
// window controls how long current-window counts are accumulated before they
// become stale. threshold is the KL-divergence value above which drift is
// reported (typical values: 0.1 for sensitive, 0.5 for lenient).
func NewDriftDetector(window time.Duration, threshold float64) *DriftDetector {
	return &DriftDetector{
		baselines: make(map[string]*ActionDistribution),
		current:   make(map[string]*ActionDistribution),
		window:    window,
		threshold: threshold,
	}
}

// RecordAction records an action for the agent's current observation window.
func (d *DriftDetector) RecordAction(agentID, actionType string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	dist, ok := d.current[agentID]
	if !ok {
		dist = &ActionDistribution{
			Counts: make(map[string]int),
		}
		d.current[agentID] = dist
	}

	dist.Counts[actionType]++
	dist.Total++
	dist.UpdatedAt = time.Now()
}

// Check checks if the agent has drifted from its baseline distribution.
// Returns nil if no baseline exists yet (not enough data to compare).
func (d *DriftDetector) Check(agentID string) *DriftResult {
	d.mu.RLock()
	defer d.mu.RUnlock()

	baseline, hasBaseline := d.baselines[agentID]
	current, hasCurrent := d.current[agentID]

	if !hasBaseline || !hasCurrent {
		return nil
	}

	if baseline.Total == 0 || current.Total == 0 {
		return nil
	}

	// Build probability distributions over the union of all action types.
	allTypes := make(map[string]struct{})
	for k := range baseline.Counts {
		allTypes[k] = struct{}{}
	}
	for k := range current.Counts {
		allTypes[k] = struct{}{}
	}

	baselineDist := normalize(baseline.Counts, baseline.Total, allTypes)
	currentDist := normalize(current.Counts, current.Total, allTypes)

	divergence := klDivergence(currentDist, baselineDist)

	// Build top drifts sorted by absolute delta.
	drifts := make([]DriftDetail, 0, len(allTypes))
	for at := range allTypes {
		exp := baselineDist[at]
		act := currentDist[at]
		drifts = append(drifts, DriftDetail{
			ActionType: at,
			Expected:   exp,
			Actual:     act,
			Delta:      act - exp,
		})
	}
	sort.Slice(drifts, func(i, j int) bool {
		return math.Abs(drifts[i].Delta) > math.Abs(drifts[j].Delta)
	})
	if len(drifts) > 5 {
		drifts = drifts[:5]
	}

	return &DriftResult{
		Drifted:    divergence >= d.threshold,
		Divergence: divergence,
		Expected:   baselineDist,
		Actual:     currentDist,
		TopDrifts:  drifts,
	}
}

// PromoteBaseline promotes the current observation window to be the new
// baseline for the agent. The current window is reset. This should be called
// periodically (e.g. every window duration) once enough data has accumulated.
func (d *DriftDetector) PromoteBaseline(agentID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	current, ok := d.current[agentID]
	if !ok || current.Total == 0 {
		return
	}

	// Deep-copy the current distribution into the baseline.
	newBaseline := &ActionDistribution{
		Counts:    make(map[string]int, len(current.Counts)),
		Total:     current.Total,
		UpdatedAt: current.UpdatedAt,
	}
	for k, v := range current.Counts {
		newBaseline.Counts[k] = v
	}
	d.baselines[agentID] = newBaseline

	// Reset the current window.
	d.current[agentID] = &ActionDistribution{
		Counts: make(map[string]int),
	}
}

// SetBaseline manually sets a baseline distribution for an agent. This is
// useful for bootstrapping from known-good profiles.
func (d *DriftDetector) SetBaseline(agentID string, dist *ActionDistribution) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.baselines[agentID] = dist
}

// normalize converts raw counts into a smoothed probability distribution over
// the given set of action types. Laplace smoothing (epsilon) is applied to
// avoid zero probabilities which would cause log(0) in KL-divergence.
func normalize(counts map[string]int, total int, allTypes map[string]struct{}) map[string]float64 {
	const epsilon = 1e-10
	numTypes := len(allTypes)

	dist := make(map[string]float64, numTypes)
	smoothedTotal := float64(total) + epsilon*float64(numTypes)

	for at := range allTypes {
		count := float64(counts[at])
		dist[at] = (count + epsilon) / smoothedTotal
	}

	return dist
}

// klDivergence computes the Kullback-Leibler divergence D_KL(P || Q) where
// P is the "actual" (current) distribution and Q is the "expected" (baseline).
// Both distributions must be defined over the same set of keys and must already
// be smoothed to avoid zero values.
//
// D_KL(P || Q) = sum_x P(x) * log(P(x) / Q(x))
func klDivergence(p, q map[string]float64) float64 {
	var kl float64
	for x, px := range p {
		qx, ok := q[x]
		if !ok || qx == 0 {
			continue
		}
		if px > 0 {
			kl += px * math.Log(px/qx)
		}
	}
	return kl
}
