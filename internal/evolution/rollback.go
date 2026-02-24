package evolution

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/agentwarden/agentwarden/internal/trace"
)

// RollbackMonitor watches agent metrics after a version promotion and
// triggers an automatic rollback if degradation is detected. It parses
// the trigger string from RollbackConfig to determine what constitutes
// a degradation (e.g. "error_rate increases by 10% within 1h").
type RollbackMonitor struct {
	store    trace.Store
	versions *VersionManager
	analyzer *Analyzer
	config   RollbackMonitorConfig
	logger   *slog.Logger
}

// RollbackMonitorConfig controls rollback behavior.
type RollbackMonitorConfig struct {
	Auto    bool
	Trigger string // e.g. "error_rate increases by 10% within 1h"
}

// rollbackTrigger is the parsed form of the trigger string.
type rollbackTrigger struct {
	Metric    string        // "error_rate", "cost_per_task", "completion_rate"
	Direction string        // "increases", "decreases"
	Threshold float64       // the percentage change threshold
	Window    time.Duration // the observation window
}

// NewRollbackMonitor creates a RollbackMonitor.
func NewRollbackMonitor(
	store trace.Store,
	vm *VersionManager,
	analyzer *Analyzer,
	config RollbackMonitorConfig,
	logger *slog.Logger,
) *RollbackMonitor {
	if logger == nil {
		logger = slog.Default()
	}

	return &RollbackMonitor{
		store:    store,
		versions: vm,
		analyzer: analyzer,
		config:   config,
		logger:   logger,
	}
}

// Check evaluates whether the current version is degrading and needs rollback.
// It compares metrics from the recent window against the previous window of
// the same duration. Returns shouldRollback=true with a reason if degradation
// exceeds the configured threshold.
func (rm *RollbackMonitor) Check(agentID string) (shouldRollback bool, reason string, err error) {
	if !rm.config.Auto {
		return false, "", nil
	}

	trigger, err := parseTrigger(rm.config.Trigger)
	if err != nil {
		return false, "", fmt.Errorf("parse trigger %q: %w", rm.config.Trigger, err)
	}

	// Get metrics for the recent window (post-promotion).
	currentMetrics, err := rm.analyzer.GetMetrics(agentID, trigger.Window)
	if err != nil {
		return false, "", fmt.Errorf("get current metrics: %w", err)
	}

	// Not enough data to make a decision.
	if currentMetrics.TotalSessions < 5 {
		rm.logger.Debug("not enough sessions for rollback check",
			"agent_id", agentID,
			"sessions", currentMetrics.TotalSessions,
		)
		return false, "", nil
	}

	// Get metrics for the previous window (pre-promotion baseline).
	baselineMetrics, err := rm.analyzer.GetMetrics(agentID, trigger.Window*2)
	if err != nil {
		return false, "", fmt.Errorf("get baseline metrics: %w", err)
	}

	currentVal := getMetricValue(currentMetrics, trigger.Metric)
	baselineVal := getMetricValue(baselineMetrics, trigger.Metric)

	// Avoid division by zero.
	if baselineVal == 0 {
		rm.logger.Debug("baseline metric is zero, skipping rollback check",
			"agent_id", agentID,
			"metric", trigger.Metric,
		)
		return false, "", nil
	}

	// Calculate percentage change.
	pctChange := ((currentVal - baselineVal) / baselineVal) * 100

	rm.logger.Info("rollback check",
		"agent_id", agentID,
		"metric", trigger.Metric,
		"current", currentVal,
		"baseline", baselineVal,
		"pct_change", pctChange,
		"threshold", trigger.Threshold,
		"direction", trigger.Direction,
	)

	// Check if degradation exceeds threshold.
	switch trigger.Direction {
	case "increases":
		if pctChange >= trigger.Threshold {
			reason = fmt.Sprintf("%s increased by %.1f%% (threshold: %.1f%%) over %s window — current: %.4f, baseline: %.4f",
				trigger.Metric, pctChange, trigger.Threshold, trigger.Window, currentVal, baselineVal)
			return true, reason, nil
		}
	case "decreases":
		// For "decreases" triggers (e.g. completion_rate decreases by 10%),
		// the change is negative, so we check the absolute value.
		if pctChange <= -trigger.Threshold {
			reason = fmt.Sprintf("%s decreased by %.1f%% (threshold: %.1f%%) over %s window — current: %.4f, baseline: %.4f",
				trigger.Metric, -pctChange, trigger.Threshold, trigger.Window, currentVal, baselineVal)
			return true, reason, nil
		}
	}

	return false, "", nil
}

// ExecuteRollback performs the rollback and returns the version rolled back to.
func (rm *RollbackMonitor) ExecuteRollback(agentID, reason string) (string, error) {
	rm.logger.Warn("executing auto-rollback",
		"agent_id", agentID,
		"reason", reason,
	)

	rolledBackTo, err := rm.versions.Rollback(agentID)
	if err != nil {
		return "", fmt.Errorf("rollback failed: %w", err)
	}

	rm.logger.Info("rollback complete",
		"agent_id", agentID,
		"rolled_back_to", rolledBackTo,
	)

	return rolledBackTo, nil
}

// parseTrigger parses a trigger string like "error_rate increases by 10% within 1h".
func parseTrigger(s string) (*rollbackTrigger, error) {
	if s == "" {
		return nil, fmt.Errorf("empty trigger string")
	}

	// Expected format: "<metric> <increases|decreases> by <N>% within <duration>"
	parts := strings.Fields(s)
	if len(parts) < 6 {
		return nil, fmt.Errorf("trigger must have format: '<metric> <increases|decreases> by <N>%% within <duration>', got: %s", s)
	}

	trigger := &rollbackTrigger{
		Metric: parts[0],
	}

	// Direction
	switch parts[1] {
	case "increases", "decreases":
		trigger.Direction = parts[1]
	default:
		return nil, fmt.Errorf("unknown direction %q, expected 'increases' or 'decreases'", parts[1])
	}

	// "by"
	if parts[2] != "by" {
		return nil, fmt.Errorf("expected 'by' at position 3, got %q", parts[2])
	}

	// Threshold — strip trailing %
	thresholdStr := strings.TrimSuffix(parts[3], "%")
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return nil, fmt.Errorf("parse threshold %q: %w", parts[3], err)
	}
	trigger.Threshold = threshold

	// "within"
	if parts[4] != "within" {
		return nil, fmt.Errorf("expected 'within' at position 5, got %q", parts[4])
	}

	// Duration
	window, err := time.ParseDuration(parts[5])
	if err != nil {
		return nil, fmt.Errorf("parse window duration %q: %w", parts[5], err)
	}
	trigger.Window = window

	return trigger, nil
}

// getMetricValue extracts a named metric value from AgentMetrics.
func getMetricValue(m *AgentMetrics, metric string) float64 {
	switch metric {
	case "error_rate":
		return m.ErrorRate
	case "completion_rate":
		return m.CompletionRate
	case "cost_per_task":
		return m.CostPerTask
	case "human_override_rate":
		return m.HumanOverrideRate
	case "avg_latency":
		return m.AvgLatency
	default:
		return 0
	}
}
