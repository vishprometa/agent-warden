package detection

import (
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

func TestVelocityDetector_BelowThreshold(t *testing.T) {
	cfg := config.VelocityDetectionConfig{
		Enabled:          true,
		Threshold:        10,
		SustainedSeconds: 5,
		Action:           "pause",
	}
	d := NewVelocityDetector(cfg)

	// Send a few actions — below threshold.
	for i := 0; i < 3; i++ {
		result := d.Check(ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Signature: "action:" + string(rune('A'+i)),
		})
		if result != nil {
			t.Errorf("check %d: expected nil, got detection", i)
		}
	}
}

func TestVelocityDetector_ResetSession(t *testing.T) {
	cfg := config.VelocityDetectionConfig{
		Enabled:          true,
		Threshold:        5,
		SustainedSeconds: 1,
		Action:           "pause",
	}
	d := NewVelocityDetector(cfg)

	// Send some events.
	for i := 0; i < 3; i++ {
		d.Check(ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Signature: "test",
		})
	}

	d.ResetSession("sess-1")

	// After reset, internal state should be clean.
	d.mu.Lock()
	_, hasWindow := d.windows["sess-1"]
	_, hasBreach := d.breachStart["sess-1"]
	d.mu.Unlock()

	if hasWindow {
		t.Error("expected windows cleared after reset")
	}
	if hasBreach {
		t.Error("expected breachStart cleared after reset")
	}
}

func TestVelocityDetector_DifferentSessions(t *testing.T) {
	cfg := config.VelocityDetectionConfig{
		Enabled:          true,
		Threshold:        2,
		SustainedSeconds: 1,
		Action:           "pause",
	}
	d := NewVelocityDetector(cfg)

	// Each session only gets one event — no breach.
	for i := 0; i < 5; i++ {
		result := d.Check(ActionEvent{
			SessionID: "sess-" + string(rune('A'+i)),
			AgentID:   "agent-1",
			Signature: "action",
		})
		if result != nil {
			t.Errorf("session-%c: expected nil, got detection", rune('A'+i))
		}
	}
}

func TestVelocityDetector_DefaultValues(t *testing.T) {
	// With zero values, should use defaults.
	cfg := config.VelocityDetectionConfig{
		Enabled: true,
	}
	d := NewVelocityDetector(cfg)

	if d.config.Threshold != 10 {
		t.Errorf("default threshold = %d, want 10", d.config.Threshold)
	}
	if d.config.SustainedSeconds != 5 {
		t.Errorf("default sustained_seconds = %d, want 5", d.config.SustainedSeconds)
	}
}

func TestVelocityDetector_BreachResetWhenBelowThreshold(t *testing.T) {
	cfg := config.VelocityDetectionConfig{
		Enabled:          true,
		Threshold:        1,
		SustainedSeconds: 10, // long sustained period
		Action:           "pause",
	}
	d := NewVelocityDetector(cfg)

	// Send 2 rapid events (above threshold of 1/sec).
	d.Check(ActionEvent{SessionID: "sess-1", AgentID: "agent-1", Signature: "a"})
	d.Check(ActionEvent{SessionID: "sess-1", AgentID: "agent-1", Signature: "b"})

	// Breach should be started but not sustained.
	d.mu.Lock()
	_, hasBreach := d.breachStart["sess-1"]
	d.mu.Unlock()
	if !hasBreach {
		t.Error("expected breach tracking started")
	}

	// Wait for velocity to drop.
	time.Sleep(1100 * time.Millisecond)

	// Send one more event — velocity should be below threshold, resetting breach.
	d.Check(ActionEvent{SessionID: "sess-1", AgentID: "agent-1", Signature: "c"})

	d.mu.Lock()
	_, hasBreach = d.breachStart["sess-1"]
	d.mu.Unlock()
	if hasBreach {
		t.Error("expected breach tracking reset when velocity drops")
	}
}
