// Package killswitch implements an emergency stop mechanism that operates
// outside the LLM's context window. When triggered, it immediately blocks
// all agent actions at the proxy level — no exceptions. This directly
// addresses the Summer Yue incident where an OpenClaw agent ignored STOP
// commands because context compaction dropped safety instructions.
package killswitch

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State represents the kill switch state.
type State string

const (
	StateArmed    State = "armed"     // normal operation, ready to trigger
	StateTriggered State = "triggered" // kill switch active, all actions blocked
)

// Scope determines what the kill switch affects.
type Scope string

const (
	ScopeGlobal  Scope = "global"  // all agents and sessions
	ScopeAgent   Scope = "agent"   // specific agent
	ScopeSession Scope = "session" // specific session
)

// TriggerRecord logs who/what triggered the kill switch and when.
type TriggerRecord struct {
	Scope     Scope     `json:"scope"`
	TargetID  string    `json:"target_id,omitempty"` // agent ID or session ID
	Reason    string    `json:"reason"`
	Source    string    `json:"source"` // api, cli, dashboard, slack, file
	Timestamp time.Time `json:"timestamp"`
}

// KillSwitch is an emergency stop mechanism that blocks all agent actions
// when triggered. It is checked BEFORE policy evaluation — it cannot be
// bypassed by context compaction, prompt injection, or any other mechanism.
type KillSwitch struct {
	mu sync.RWMutex

	// globalTriggered is the master kill switch.
	globalTriggered bool

	// agentKills tracks per-agent kill switches. Key is agent ID.
	agentKills map[string]TriggerRecord

	// sessionKills tracks per-session kill switches. Key is session ID.
	sessionKills map[string]TriggerRecord

	// history keeps a record of all triggers for audit.
	history []TriggerRecord

	// fileWatchPath is checked for a KILL sentinel file.
	fileWatchPath string

	logger *slog.Logger
}

// New creates a new KillSwitch. The fileWatchPath is optional — if set,
// the presence of a KILL file at that path triggers a global kill.
func New(logger *slog.Logger) *KillSwitch {
	if logger == nil {
		logger = slog.Default()
	}

	homeDir, _ := os.UserHomeDir()
	watchPath := filepath.Join(homeDir, ".agentwarden", "KILL")

	return &KillSwitch{
		agentKills:    make(map[string]TriggerRecord),
		sessionKills:  make(map[string]TriggerRecord),
		fileWatchPath: watchPath,
		logger:        logger.With("component", "killswitch"),
	}
}

// IsBlocked checks whether an action should be blocked. This is the hot-path
// method called on every single request. It must be fast (< 1 microsecond).
func (ks *KillSwitch) IsBlocked(agentID, sessionID string) (bool, string) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	// Global kill switch — blocks everything.
	if ks.globalTriggered {
		return true, "global kill switch activated"
	}

	// Agent-level kill switch.
	if record, ok := ks.agentKills[agentID]; ok {
		return true, fmt.Sprintf("agent kill switch activated: %s", record.Reason)
	}

	// Session-level kill switch.
	if record, ok := ks.sessionKills[sessionID]; ok {
		return true, fmt.Sprintf("session kill switch activated: %s", record.Reason)
	}

	return false, ""
}

// TriggerGlobal activates the global kill switch, blocking ALL actions.
func (ks *KillSwitch) TriggerGlobal(reason, source string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	ks.globalTriggered = true
	record := TriggerRecord{
		Scope:     ScopeGlobal,
		Reason:    reason,
		Source:    source,
		Timestamp: time.Now(),
	}
	ks.history = append(ks.history, record)

	ks.logger.Error("GLOBAL KILL SWITCH TRIGGERED",
		"reason", reason,
		"source", source,
	)
}

// TriggerAgent activates the kill switch for a specific agent.
func (ks *KillSwitch) TriggerAgent(agentID, reason, source string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	record := TriggerRecord{
		Scope:     ScopeAgent,
		TargetID:  agentID,
		Reason:    reason,
		Source:    source,
		Timestamp: time.Now(),
	}
	ks.agentKills[agentID] = record
	ks.history = append(ks.history, record)

	ks.logger.Error("AGENT KILL SWITCH TRIGGERED",
		"agent_id", agentID,
		"reason", reason,
		"source", source,
	)
}

// TriggerSession activates the kill switch for a specific session.
func (ks *KillSwitch) TriggerSession(sessionID, reason, source string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	record := TriggerRecord{
		Scope:     ScopeSession,
		TargetID:  sessionID,
		Reason:    reason,
		Source:    source,
		Timestamp: time.Now(),
	}
	ks.sessionKills[sessionID] = record
	ks.history = append(ks.history, record)

	ks.logger.Error("SESSION KILL SWITCH TRIGGERED",
		"session_id", sessionID,
		"reason", reason,
		"source", source,
	)
}

// ResetGlobal disarms the global kill switch.
func (ks *KillSwitch) ResetGlobal() {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.globalTriggered = false
	ks.logger.Info("global kill switch reset")
}

// ResetAgent disarms the kill switch for a specific agent.
func (ks *KillSwitch) ResetAgent(agentID string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	delete(ks.agentKills, agentID)
	ks.logger.Info("agent kill switch reset", "agent_id", agentID)
}

// ResetSession disarms the kill switch for a specific session.
func (ks *KillSwitch) ResetSession(sessionID string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	delete(ks.sessionKills, sessionID)
	ks.logger.Info("session kill switch reset", "session_id", sessionID)
}

// Status returns the current state of all kill switches.
func (ks *KillSwitch) Status() map[string]interface{} {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	agentKills := make(map[string]TriggerRecord, len(ks.agentKills))
	for k, v := range ks.agentKills {
		agentKills[k] = v
	}
	sessionKills := make(map[string]TriggerRecord, len(ks.sessionKills))
	for k, v := range ks.sessionKills {
		sessionKills[k] = v
	}

	return map[string]interface{}{
		"global_triggered": ks.globalTriggered,
		"agent_kills":      agentKills,
		"session_kills":    sessionKills,
		"history_count":    len(ks.history),
	}
}

// History returns the full trigger history for audit purposes.
func (ks *KillSwitch) History() []TriggerRecord {
	ks.mu.RLock()
	defer ks.mu.RUnlock()
	out := make([]TriggerRecord, len(ks.history))
	copy(out, ks.history)
	return out
}

// CheckFileKill checks for a sentinel KILL file and triggers the global
// kill switch if found. Call this periodically (e.g., every second).
func (ks *KillSwitch) CheckFileKill() {
	if ks.fileWatchPath == "" {
		return
	}
	if _, err := os.Stat(ks.fileWatchPath); err == nil {
		ks.mu.RLock()
		alreadyTriggered := ks.globalTriggered
		ks.mu.RUnlock()

		if !alreadyTriggered {
			ks.TriggerGlobal("KILL sentinel file detected", "file")
		}
	}
}
