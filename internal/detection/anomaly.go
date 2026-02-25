package detection

import (
	"fmt"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// costEntry records a cost event with timestamp.
type costEntry struct {
	Cost      float64
	Timestamp time.Time
}

// CostAnomalyDetector detects abnormal cost velocity spikes.
type CostAnomalyDetector struct {
	mu     sync.Mutex
	config config.CostAnomalyConfig
	// sessionID → cost entries
	history map[string][]costEntry
}

// NewCostAnomalyDetector creates a new cost anomaly detector.
func NewCostAnomalyDetector(cfg config.CostAnomalyConfig) *CostAnomalyDetector {
	return &CostAnomalyDetector{
		config:  cfg,
		history: make(map[string][]costEntry),
	}
}

// Check records a cost event and returns a detection event if anomalous.
func (d *CostAnomalyDetector) Check(event ActionEvent) *Event {
	if event.CostUSD <= 0 {
		return nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	d.history[event.SessionID] = append(d.history[event.SessionID], costEntry{
		Cost:      event.CostUSD,
		Timestamp: now,
	})

	entries := d.history[event.SessionID]
	if len(entries) < 3 {
		return nil // Need at least 3 data points
	}

	// Calculate recent velocity (last 30s) vs baseline velocity (older)
	cutoff := now.Add(-30 * time.Second)
	var recentCost, baselineCost float64
	var recentCount, baselineCount int

	for _, e := range entries {
		if e.Timestamp.After(cutoff) {
			recentCost += e.Cost
			recentCount++
		} else {
			baselineCost += e.Cost
			baselineCount++
		}
	}

	if baselineCount == 0 || baselineCost == 0 {
		return nil
	}

	// Compare cost per action
	recentRate := recentCost / float64(max(recentCount, 1))
	baselineRate := baselineCost / float64(baselineCount)

	if baselineRate > 0 && recentRate > baselineRate*d.config.Multiplier {
		return &Event{
			Type:      "cost_anomaly",
			SessionID: event.SessionID,
			AgentID:   event.AgentID,
			Action:    d.config.Action,
			Message: fmt.Sprintf("Cost anomaly: recent rate $%.4f/action vs baseline $%.4f/action (%.1fx, threshold: %.1fx)",
				recentRate, baselineRate, recentRate/baselineRate, d.config.Multiplier),
			Details: map[string]interface{}{
				"recent_rate":   recentRate,
				"baseline_rate": baselineRate,
				"multiplier":    recentRate / baselineRate,
				"threshold":     d.config.Multiplier,
			},
		}
	}

	return nil
}

// ResetSession clears state for a session.
func (d *CostAnomalyDetector) ResetSession(sessionID string) {
	d.mu.Lock()
	delete(d.history, sessionID)
	d.mu.Unlock()
}
