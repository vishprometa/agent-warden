package policy

import "log/slog"

// BudgetChecker evaluates whether a session's accumulated cost exceeds
// a configured threshold. It is intentionally stateless -- the session cost
// is supplied by the caller (typically from session.Manager).
type BudgetChecker struct {
	logger *slog.Logger
}

// NewBudgetChecker creates a BudgetChecker.
func NewBudgetChecker(logger *slog.Logger) *BudgetChecker {
	if logger == nil {
		logger = slog.Default()
	}
	return &BudgetChecker{logger: logger.With("component", "policy.BudgetChecker")}
}

// Check returns true if the session cost has exceeded the threshold.
func (b *BudgetChecker) Check(sessionCost float64, threshold float64) bool {
	if threshold <= 0 {
		return false
	}
	exceeded := sessionCost > threshold
	if exceeded {
		b.logger.Warn("budget threshold exceeded",
			"session_cost", sessionCost,
			"threshold", threshold,
		)
	}
	return exceeded
}
