package detection

import (
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

func TestLoopDetector_ExceedsThreshold(t *testing.T) {
	cfg := config.LoopDetectionConfig{
		Enabled:   true,
		Threshold: 3,
		Window:    10 * time.Second,
		Action:    "pause",
	}
	d := NewLoopDetector(cfg)

	event := ActionEvent{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		Signature: "action:chat_completion:gpt-4",
	}

	// First 3 checks should not trigger (count <= threshold)
	for i := 0; i < 3; i++ {
		result := d.Check(event)
		if result != nil {
			t.Errorf("Check #%d: expected nil, got detection event", i+1)
		}
	}

	// 4th check should trigger (count 4 > threshold 3)
	result := d.Check(event)
	if result == nil {
		t.Fatal("Check #4: expected detection event, got nil")
	}
	if result.Type != "loop" {
		t.Errorf("event type = %q, want \"loop\"", result.Type)
	}
	if result.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want \"sess-1\"", result.SessionID)
	}
	if result.Action != "pause" {
		t.Errorf("action = %q, want \"pause\"", result.Action)
	}
}

func TestLoopDetector_BelowThreshold(t *testing.T) {
	cfg := config.LoopDetectionConfig{
		Enabled:   true,
		Threshold: 5,
		Window:    10 * time.Second,
		Action:    "pause",
	}
	d := NewLoopDetector(cfg)

	event := ActionEvent{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		Signature: "action:chat",
	}

	// Send exactly threshold (5) events; none should trigger (count <= threshold)
	for i := 0; i < 5; i++ {
		result := d.Check(event)
		if result != nil {
			t.Errorf("Check #%d: expected nil, got detection event", i+1)
		}
	}
}

func TestLoopDetector_DifferentSignatures(t *testing.T) {
	cfg := config.LoopDetectionConfig{
		Enabled:   true,
		Threshold: 2,
		Window:    10 * time.Second,
		Action:    "pause",
	}
	d := NewLoopDetector(cfg)

	// Different signatures should not accumulate
	for i := 0; i < 5; i++ {
		event := ActionEvent{
			SessionID: "sess-1",
			AgentID:   "agent-1",
			Signature: "action:" + string(rune('A'+i)),
		}
		result := d.Check(event)
		if result != nil {
			t.Errorf("Check with distinct signature #%d: expected nil, got detection", i+1)
		}
	}
}

func TestLoopDetector_DifferentSessions(t *testing.T) {
	cfg := config.LoopDetectionConfig{
		Enabled:   true,
		Threshold: 2,
		Window:    10 * time.Second,
		Action:    "pause",
	}
	d := NewLoopDetector(cfg)

	// Each session gets its own counter
	for i := 0; i < 5; i++ {
		event := ActionEvent{
			SessionID: "sess-" + string(rune('A'+i)),
			AgentID:   "agent-1",
			Signature: "same-action",
		}
		result := d.Check(event)
		if result != nil {
			t.Errorf("Check with distinct session #%d: expected nil, got detection", i+1)
		}
	}
}

func TestLoopDetector_WindowExpiry(t *testing.T) {
	cfg := config.LoopDetectionConfig{
		Enabled:   true,
		Threshold: 2,
		Window:    50 * time.Millisecond,
		Action:    "alert",
	}
	d := NewLoopDetector(cfg)

	event := ActionEvent{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		Signature: "action:chat",
	}

	// Record 2 events (at threshold)
	d.Check(event)
	d.Check(event)

	// Wait for window to expire
	time.Sleep(100 * time.Millisecond)

	// The old events should be pruned, so count should be 1 (this new event)
	result := d.Check(event)
	if result != nil {
		t.Error("expected nil after window expiry, got detection event")
	}
}

func TestLoopDetector_ResetSession(t *testing.T) {
	cfg := config.LoopDetectionConfig{
		Enabled:   true,
		Threshold: 2,
		Window:    10 * time.Second,
		Action:    "pause",
	}
	d := NewLoopDetector(cfg)

	event := ActionEvent{
		SessionID: "sess-1",
		AgentID:   "agent-1",
		Signature: "action:chat",
	}

	// Record 2 events
	d.Check(event)
	d.Check(event)

	// Reset the session
	d.ResetSession("sess-1")

	// After reset, count starts from 0; next check should not trigger
	result := d.Check(event)
	if result != nil {
		t.Error("expected nil after ResetSession, got detection event")
	}
}

func TestLoopDetector_DetectionDetails(t *testing.T) {
	cfg := config.LoopDetectionConfig{
		Enabled:   true,
		Threshold: 1,
		Window:    10 * time.Second,
		Action:    "terminate",
	}
	d := NewLoopDetector(cfg)

	event := ActionEvent{
		SessionID: "sess-42",
		AgentID:   "agent-7",
		Signature: "repeated-action",
	}

	// First check: count=1, not > threshold=1
	d.Check(event)

	// Second check: count=2, > threshold=1 -> triggers
	result := d.Check(event)
	if result == nil {
		t.Fatal("expected detection event")
	}

	if result.Details["signature"] != "repeated-action" {
		t.Errorf("details.signature = %v, want \"repeated-action\"", result.Details["signature"])
	}
	if result.Details["count"] != 2 {
		t.Errorf("details.count = %v, want 2", result.Details["count"])
	}
	if result.Details["threshold"] != 1 {
		t.Errorf("details.threshold = %v, want 1", result.Details["threshold"])
	}
	if result.AgentID != "agent-7" {
		t.Errorf("agent_id = %q, want \"agent-7\"", result.AgentID)
	}
}
