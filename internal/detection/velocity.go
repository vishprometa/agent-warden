package detection

import (
	"fmt"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// VelocityDetector detects when an agent is firing actions too rapidly,
// suggesting it has gone out of control. Unlike loop detection (which
// catches repeated identical actions), velocity detection catches diverse
// rapid actions — the hallmark of a runaway agent.
type VelocityDetector struct {
	mu     sync.Mutex
	config config.VelocityDetectionConfig
	// sessionID → list of action timestamps
	windows map[string][]time.Time
	// sessionID → time when velocity first exceeded threshold
	breachStart map[string]time.Time
}

// NewVelocityDetector creates a new velocity detector.
func NewVelocityDetector(cfg config.VelocityDetectionConfig) *VelocityDetector {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 10
	}
	if cfg.SustainedSeconds <= 0 {
		cfg.SustainedSeconds = 5
	}
	return &VelocityDetector{
		config:      cfg,
		windows:     make(map[string][]time.Time),
		breachStart: make(map[string]time.Time),
	}
}

// Check records an action and returns a detection event if velocity
// has been exceeded for the sustained period.
func (d *VelocityDetector) Check(event ActionEvent) *Event {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Record this action.
	d.windows[event.SessionID] = append(d.windows[event.SessionID], now)

	// Prune timestamps older than the sustained window + 1 second.
	cutoff := now.Add(-time.Duration(d.config.SustainedSeconds+1) * time.Second)
	timestamps := d.windows[event.SessionID]
	pruned := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	d.windows[event.SessionID] = pruned

	// Calculate current velocity (actions in the last second).
	oneSecAgo := now.Add(-time.Second)
	recentCount := 0
	for _, ts := range pruned {
		if ts.After(oneSecAgo) {
			recentCount++
		}
	}

	// Check if we're above threshold.
	if recentCount > d.config.Threshold {
		// Start or continue breach tracking.
		if _, ok := d.breachStart[event.SessionID]; !ok {
			d.breachStart[event.SessionID] = now
		}

		// Check if sustained long enough.
		breachDuration := now.Sub(d.breachStart[event.SessionID])
		if breachDuration >= time.Duration(d.config.SustainedSeconds)*time.Second {
			return &Event{
				Type:      "velocity",
				SessionID: event.SessionID,
				AgentID:   event.AgentID,
				Action:    d.config.Action,
				Message: fmt.Sprintf("Action velocity breach: %d actions/sec sustained for %s (threshold: %d/sec for %ds)",
					recentCount, breachDuration.Round(time.Second), d.config.Threshold, d.config.SustainedSeconds),
				Details: map[string]interface{}{
					"velocity":          recentCount,
					"threshold":         d.config.Threshold,
					"sustained_seconds": d.config.SustainedSeconds,
					"breach_duration":   breachDuration.String(),
				},
			}
		}
	} else {
		// Below threshold — reset breach tracking.
		delete(d.breachStart, event.SessionID)
	}

	return nil
}

// ResetSession clears state for a session.
func (d *VelocityDetector) ResetSession(sessionID string) {
	d.mu.Lock()
	delete(d.windows, sessionID)
	delete(d.breachStart, sessionID)
	d.mu.Unlock()
}
