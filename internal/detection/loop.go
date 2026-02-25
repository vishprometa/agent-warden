package detection

import (
	"fmt"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// LoopDetector detects repeated identical actions within a sliding window.
type LoopDetector struct {
	mu     sync.Mutex
	config config.LoopDetectionConfig
	// sessionID → signature → timestamps
	windows map[string]map[string][]time.Time
}

// NewLoopDetector creates a new loop detector.
func NewLoopDetector(cfg config.LoopDetectionConfig) *LoopDetector {
	return &LoopDetector{
		config:  cfg,
		windows: make(map[string]map[string][]time.Time),
	}
}

// Check records an action and returns a detection event if a loop is found.
func (d *LoopDetector) Check(event ActionEvent) *Event {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	sessionWindows, ok := d.windows[event.SessionID]
	if !ok {
		sessionWindows = make(map[string][]time.Time)
		d.windows[event.SessionID] = sessionWindows
	}

	// Add current timestamp
	sessionWindows[event.Signature] = append(sessionWindows[event.Signature], now)

	// Prune timestamps outside the window
	cutoff := now.Add(-d.config.Window)
	timestamps := sessionWindows[event.Signature]
	pruned := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	sessionWindows[event.Signature] = pruned

	// Check threshold
	if len(pruned) > d.config.Threshold {
		return &Event{
			Type:      "loop",
			SessionID: event.SessionID,
			AgentID:   event.AgentID,
			Action:    d.config.Action,
			Message: fmt.Sprintf("Loop detected: action %q repeated %d times in %s (threshold: %d)",
				event.Signature, len(pruned), d.config.Window, d.config.Threshold),
			Details: map[string]interface{}{
				"signature": event.Signature,
				"count":     len(pruned),
				"window":    d.config.Window.String(),
				"threshold": d.config.Threshold,
			},
		}
	}

	return nil
}

// ResetSession clears state for a session.
func (d *LoopDetector) ResetSession(sessionID string) {
	d.mu.Lock()
	delete(d.windows, sessionID)
	d.mu.Unlock()
}
