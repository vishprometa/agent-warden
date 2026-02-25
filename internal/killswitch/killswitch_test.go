package killswitch

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKillSwitch_GlobalTrigger(t *testing.T) {
	ks := New(nil)

	// Initially not blocked.
	blocked, _ := ks.IsBlocked("agent-1", "sess-1")
	if blocked {
		t.Fatal("expected not blocked initially")
	}

	// Trigger global kill.
	ks.TriggerGlobal("runaway agent", "api")

	// Now everything should be blocked.
	blocked, msg := ks.IsBlocked("agent-1", "sess-1")
	if !blocked {
		t.Fatal("expected blocked after global trigger")
	}
	if msg != "global kill switch activated" {
		t.Errorf("message = %q, want %q", msg, "global kill switch activated")
	}

	// Different agent/session also blocked.
	blocked, _ = ks.IsBlocked("agent-99", "sess-99")
	if !blocked {
		t.Fatal("expected all agents blocked after global trigger")
	}
}

func TestKillSwitch_GlobalReset(t *testing.T) {
	ks := New(nil)
	ks.TriggerGlobal("test", "cli")

	blocked, _ := ks.IsBlocked("agent-1", "sess-1")
	if !blocked {
		t.Fatal("expected blocked")
	}

	ks.ResetGlobal()

	blocked, _ = ks.IsBlocked("agent-1", "sess-1")
	if blocked {
		t.Fatal("expected not blocked after reset")
	}
}

func TestKillSwitch_AgentTrigger(t *testing.T) {
	ks := New(nil)

	ks.TriggerAgent("agent-1", "cost exceeded", "dashboard")

	// Agent-1 should be blocked.
	blocked, msg := ks.IsBlocked("agent-1", "sess-1")
	if !blocked {
		t.Fatal("expected agent-1 blocked")
	}
	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	// Agent-2 should NOT be blocked.
	blocked, _ = ks.IsBlocked("agent-2", "sess-2")
	if blocked {
		t.Fatal("expected agent-2 not blocked")
	}
}

func TestKillSwitch_AgentReset(t *testing.T) {
	ks := New(nil)
	ks.TriggerAgent("agent-1", "test", "api")

	blocked, _ := ks.IsBlocked("agent-1", "sess-1")
	if !blocked {
		t.Fatal("expected blocked")
	}

	ks.ResetAgent("agent-1")

	blocked, _ = ks.IsBlocked("agent-1", "sess-1")
	if blocked {
		t.Fatal("expected not blocked after agent reset")
	}
}

func TestKillSwitch_SessionTrigger(t *testing.T) {
	ks := New(nil)

	ks.TriggerSession("sess-42", "loop detected", "detection")

	// Session-42 should be blocked.
	blocked, msg := ks.IsBlocked("agent-1", "sess-42")
	if !blocked {
		t.Fatal("expected session-42 blocked")
	}
	if msg == "" {
		t.Fatal("expected non-empty message")
	}

	// Different session should NOT be blocked.
	blocked, _ = ks.IsBlocked("agent-1", "sess-99")
	if blocked {
		t.Fatal("expected sess-99 not blocked")
	}
}

func TestKillSwitch_SessionReset(t *testing.T) {
	ks := New(nil)
	ks.TriggerSession("sess-1", "test", "api")

	ks.ResetSession("sess-1")

	blocked, _ := ks.IsBlocked("agent-1", "sess-1")
	if blocked {
		t.Fatal("expected not blocked after session reset")
	}
}

func TestKillSwitch_PriorityOrder(t *testing.T) {
	ks := New(nil)

	// Trigger agent kill.
	ks.TriggerAgent("agent-1", "agent reason", "api")

	// Trigger session kill for same session.
	ks.TriggerSession("sess-1", "session reason", "api")

	// Both should block but agent-level message should take precedence
	// (checked before session).
	blocked, msg := ks.IsBlocked("agent-1", "sess-1")
	if !blocked {
		t.Fatal("expected blocked")
	}
	if msg != "agent kill switch activated: agent reason" {
		t.Errorf("expected agent-level message, got %q", msg)
	}

	// Trigger global — should take absolute precedence.
	ks.TriggerGlobal("global reason", "api")

	blocked, msg = ks.IsBlocked("agent-1", "sess-1")
	if !blocked {
		t.Fatal("expected blocked")
	}
	if msg != "global kill switch activated" {
		t.Errorf("expected global message, got %q", msg)
	}
}

func TestKillSwitch_History(t *testing.T) {
	ks := New(nil)

	ks.TriggerGlobal("reason1", "api")
	ks.TriggerAgent("agent-1", "reason2", "cli")
	ks.TriggerSession("sess-1", "reason3", "dashboard")

	history := ks.History()
	if len(history) != 3 {
		t.Fatalf("history length = %d, want 3", len(history))
	}

	if history[0].Scope != ScopeGlobal {
		t.Errorf("history[0].Scope = %q, want %q", history[0].Scope, ScopeGlobal)
	}
	if history[1].Scope != ScopeAgent {
		t.Errorf("history[1].Scope = %q, want %q", history[1].Scope, ScopeAgent)
	}
	if history[2].Scope != ScopeSession {
		t.Errorf("history[2].Scope = %q, want %q", history[2].Scope, ScopeSession)
	}
}

func TestKillSwitch_Status(t *testing.T) {
	ks := New(nil)

	status := ks.Status()
	if status["global_triggered"].(bool) {
		t.Error("expected global_triggered=false")
	}
	if status["history_count"].(int) != 0 {
		t.Error("expected history_count=0")
	}

	ks.TriggerGlobal("test", "api")
	ks.TriggerAgent("agent-1", "test", "api")

	status = ks.Status()
	if !status["global_triggered"].(bool) {
		t.Error("expected global_triggered=true")
	}
	if status["history_count"].(int) != 2 {
		t.Errorf("history_count = %d, want 2", status["history_count"].(int))
	}
	agents := status["agent_kills"].(map[string]TriggerRecord)
	if _, ok := agents["agent-1"]; !ok {
		t.Error("expected agent-1 in agent_kills")
	}
}

func TestKillSwitch_FileKill(t *testing.T) {
	// Create a temp directory for the KILL file.
	tmpDir := t.TempDir()
	killFile := filepath.Join(tmpDir, "KILL")

	ks := New(nil)
	ks.fileWatchPath = killFile

	// No KILL file — should not trigger.
	ks.CheckFileKill()
	blocked, _ := ks.IsBlocked("agent-1", "sess-1")
	if blocked {
		t.Fatal("expected not blocked without KILL file")
	}

	// Create KILL file.
	if err := os.WriteFile(killFile, []byte("STOP"), 0644); err != nil {
		t.Fatal(err)
	}

	ks.CheckFileKill()
	blocked, _ = ks.IsBlocked("agent-1", "sess-1")
	if !blocked {
		t.Fatal("expected blocked after KILL file created")
	}

	// Calling again should not create duplicate history entries.
	historyBefore := len(ks.History())
	ks.CheckFileKill()
	historyAfter := len(ks.History())
	if historyAfter != historyBefore {
		t.Errorf("duplicate history entry created: before=%d, after=%d", historyBefore, historyAfter)
	}
}
