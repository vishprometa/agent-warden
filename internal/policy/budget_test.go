package policy

import (
	"testing"
)

func TestBudgetChecker_ExceedsThreshold(t *testing.T) {
	bc := NewBudgetChecker(nil)

	tests := []struct {
		name        string
		sessionCost float64
		threshold   float64
		want        bool
	}{
		{"cost exceeds threshold", 15.0, 10.0, true},
		{"cost equals threshold", 10.0, 10.0, false},
		{"cost below threshold", 5.0, 10.0, false},
		{"zero cost", 0.0, 10.0, false},
		{"very small excess", 10.001, 10.0, true},
		{"large cost", 1000.0, 100.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bc.Check(tt.sessionCost, tt.threshold)
			if got != tt.want {
				t.Errorf("Check(%f, %f) = %v, want %v",
					tt.sessionCost, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestBudgetChecker_ZeroThreshold(t *testing.T) {
	bc := NewBudgetChecker(nil)

	// Zero threshold should always return false (disabled)
	if bc.Check(100.0, 0) {
		t.Error("Check with threshold=0 should return false (disabled)")
	}
}

func TestBudgetChecker_NegativeThreshold(t *testing.T) {
	bc := NewBudgetChecker(nil)

	// Negative threshold should return false (treated as disabled)
	if bc.Check(100.0, -5.0) {
		t.Error("Check with negative threshold should return false")
	}
}

func TestBudgetChecker_ZeroCost(t *testing.T) {
	bc := NewBudgetChecker(nil)

	if bc.Check(0, 10.0) {
		t.Error("Check with zero cost should return false")
	}
}
