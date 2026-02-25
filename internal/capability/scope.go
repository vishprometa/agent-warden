// Package capability implements per-agent capability boundaries that restrict
// what actions an agent can perform. These boundaries are enforced at the
// proxy level and cannot be exceeded regardless of LLM output.
//
// This addresses OpenClaw's excessive permissions problem — agents with
// shell access, file system access, and network access need boundaries
// that cannot be prompt-injected away.
package capability

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

// AgentCapabilities defines the boundary of what an agent is allowed to do.
type AgentCapabilities struct {
	Filesystem  FilesystemCap  `yaml:"filesystem" json:"filesystem"`
	Network     NetworkCap     `yaml:"network" json:"network"`
	Shell       ShellCap       `yaml:"shell" json:"shell"`
	Messaging   MessagingCap   `yaml:"messaging" json:"messaging"`
	Financial   FinancialCap   `yaml:"financial" json:"financial"`
	SpawnConfig SpawnCap       `yaml:"spawn" json:"spawn"`
}

// FilesystemCap controls file system access.
type FilesystemCap struct {
	AllowedPaths []string `yaml:"allowed_paths" json:"allowed_paths"`
	DeniedPaths  []string `yaml:"denied_paths" json:"denied_paths"`
	ReadOnly     bool     `yaml:"read_only" json:"read_only"`
	MaxFileSize  string   `yaml:"max_file_size" json:"max_file_size"`
}

// NetworkCap controls network access.
type NetworkCap struct {
	AllowedDomains []string `yaml:"allowed_domains" json:"allowed_domains"`
	BlockedDomains []string `yaml:"blocked_domains" json:"blocked_domains"`
	BlockedPorts   []int    `yaml:"blocked_ports" json:"blocked_ports"`
}

// ShellCap controls shell command execution.
type ShellCap struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	AllowedCommands []string `yaml:"allowed_commands" json:"allowed_commands"`
	BlockedCommands []string `yaml:"blocked_commands" json:"blocked_commands"`
	BlockedPatterns []string `yaml:"blocked_patterns" json:"blocked_patterns"`
}

// MessagingCap controls outbound messaging.
type MessagingCap struct {
	AllowedChannels    []string `yaml:"allowed_channels" json:"allowed_channels"`
	BlockedChannels    []string `yaml:"blocked_channels" json:"blocked_channels"`
	MaxMessagesPerHour int      `yaml:"max_messages_per_hour" json:"max_messages_per_hour"`
}

// FinancialCap controls financial transactions.
type FinancialCap struct {
	MaxTransaction      float64 `yaml:"max_transaction" json:"max_transaction"`
	MaxDailyTotal       float64 `yaml:"max_daily_total" json:"max_daily_total"`
	RequireApprovalOver float64 `yaml:"require_approval_over" json:"require_approval_over"`
}

// SpawnCap controls agent spawning.
type SpawnCap struct {
	Enabled             bool    `yaml:"enabled" json:"enabled"`
	MaxChildren         int     `yaml:"max_children" json:"max_children"`
	MaxDepth            int     `yaml:"max_depth" json:"max_depth"`
	InheritCapabilities bool    `yaml:"inherit_capabilities" json:"inherit_capabilities"`
	ChildBudgetMax      float64 `yaml:"child_budget_max" json:"child_budget_max"`
}

// CheckResult is the result of a capability check.
type CheckResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Engine manages and checks capability boundaries for all agents.
type Engine struct {
	mu     sync.RWMutex
	agents map[string]AgentCapabilities // agentID → capabilities
	logger *slog.Logger
}

// NewEngine creates a new capability engine.
func NewEngine(logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		agents: make(map[string]AgentCapabilities),
		logger: logger.With("component", "capability.Engine"),
	}
}

// SetCapabilities sets the capabilities for an agent.
func (e *Engine) SetCapabilities(agentID string, caps AgentCapabilities) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.agents[agentID] = caps
	e.logger.Info("capabilities set", "agent_id", agentID)
}

// GetCapabilities returns the capabilities for an agent.
func (e *Engine) GetCapabilities(agentID string) (AgentCapabilities, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	caps, ok := e.agents[agentID]
	return caps, ok
}

// Check evaluates whether an action is within an agent's capability boundaries.
func (e *Engine) Check(agentID, actionType string, params map[string]interface{}) CheckResult {
	e.mu.RLock()
	caps, ok := e.agents[agentID]
	e.mu.RUnlock()

	// If no capabilities are configured, allow everything (backwards compat).
	if !ok {
		return CheckResult{Allowed: true}
	}

	switch actionType {
	case "file.write", "file.read", "file.delete":
		return e.checkFilesystem(caps.Filesystem, actionType, params)
	case "tool.call":
		return e.checkShell(caps.Shell, params)
	case "message.send", "message.broadcast":
		return e.checkMessaging(caps.Messaging, params)
	case "financial.transfer":
		return e.checkFinancial(caps.Financial, params)
	case "agent.spawn":
		return e.checkSpawn(caps.SpawnConfig, params)
	case "web.navigate", "api.call":
		return e.checkNetwork(caps.Network, params)
	default:
		return CheckResult{Allowed: true}
	}
}

func (e *Engine) checkFilesystem(caps FilesystemCap, actionType string, params map[string]interface{}) CheckResult {
	path, _ := params["path"].(string)
	if path == "" {
		return CheckResult{Allowed: true}
	}

	// Check read-only.
	if caps.ReadOnly && (actionType == "file.write" || actionType == "file.delete") {
		return CheckResult{
			Allowed: false,
			Reason:  "agent has read-only filesystem access",
		}
	}

	// Check denied paths.
	for _, denied := range caps.DeniedPaths {
		if matchPath(path, denied) {
			return CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("path %q matches denied pattern %q", path, denied),
			}
		}
	}

	// Check allowed paths (if any are configured, path must match at least one).
	if len(caps.AllowedPaths) > 0 {
		for _, allowed := range caps.AllowedPaths {
			if matchPath(path, allowed) {
				return CheckResult{Allowed: true}
			}
		}
		return CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("path %q not in allowed paths", path),
		}
	}

	return CheckResult{Allowed: true}
}

func (e *Engine) checkShell(caps ShellCap, params map[string]interface{}) CheckResult {
	if !caps.Enabled {
		return CheckResult{
			Allowed: false,
			Reason:  "shell execution disabled for this agent",
		}
	}

	command, _ := params["command"].(string)
	if command == "" {
		return CheckResult{Allowed: true}
	}

	// Extract the base command (first word).
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return CheckResult{Allowed: true}
	}
	baseCmd := filepath.Base(parts[0])

	// Check blocked commands.
	for _, blocked := range caps.BlockedCommands {
		if baseCmd == blocked {
			return CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("command %q is blocked", baseCmd),
			}
		}
	}

	// Check blocked patterns.
	for _, pattern := range caps.BlockedPatterns {
		if strings.Contains(command, pattern) {
			return CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("command matches blocked pattern %q", pattern),
			}
		}
	}

	// Check allowed commands (if configured, command must be in the list).
	if len(caps.AllowedCommands) > 0 {
		for _, allowed := range caps.AllowedCommands {
			if baseCmd == allowed {
				return CheckResult{Allowed: true}
			}
		}
		return CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("command %q not in allowed commands", baseCmd),
		}
	}

	return CheckResult{Allowed: true}
}

func (e *Engine) checkNetwork(caps NetworkCap, params map[string]interface{}) CheckResult {
	domain, _ := params["domain"].(string)
	if domain == "" {
		return CheckResult{Allowed: true}
	}

	// Check blocked domains.
	for _, blocked := range caps.BlockedDomains {
		if strings.Contains(domain, blocked) {
			return CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("domain %q is blocked", domain),
			}
		}
	}

	// Check allowed domains (if configured).
	if len(caps.AllowedDomains) > 0 {
		for _, allowed := range caps.AllowedDomains {
			if strings.Contains(domain, allowed) {
				return CheckResult{Allowed: true}
			}
		}
		return CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("domain %q not in allowed domains", domain),
		}
	}

	return CheckResult{Allowed: true}
}

func (e *Engine) checkMessaging(caps MessagingCap, params map[string]interface{}) CheckResult {
	channel, _ := params["channel"].(string)
	if channel == "" {
		return CheckResult{Allowed: true}
	}

	// Check blocked channels.
	for _, blocked := range caps.BlockedChannels {
		if channel == blocked {
			return CheckResult{
				Allowed: false,
				Reason:  fmt.Sprintf("channel %q is blocked", channel),
			}
		}
	}

	// Check allowed channels.
	if len(caps.AllowedChannels) > 0 {
		for _, allowed := range caps.AllowedChannels {
			if channel == allowed {
				return CheckResult{Allowed: true}
			}
		}
		return CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("channel %q not in allowed channels", channel),
		}
	}

	return CheckResult{Allowed: true}
}

func (e *Engine) checkFinancial(caps FinancialCap, params map[string]interface{}) CheckResult {
	amount, _ := params["amount"].(float64)

	if caps.MaxTransaction > 0 && amount > caps.MaxTransaction {
		return CheckResult{
			Allowed: false,
			Reason:  fmt.Sprintf("amount $%.2f exceeds max transaction limit $%.2f", amount, caps.MaxTransaction),
		}
	}

	// MaxTransaction of 0 means disabled.
	if caps.MaxTransaction == 0 && amount > 0 {
		return CheckResult{
			Allowed: false,
			Reason:  "financial transactions disabled for this agent",
		}
	}

	return CheckResult{Allowed: true}
}

func (e *Engine) checkSpawn(caps SpawnCap, params map[string]interface{}) CheckResult {
	if !caps.Enabled {
		return CheckResult{
			Allowed: false,
			Reason:  "agent spawning disabled",
		}
	}
	return CheckResult{Allowed: true}
}

// matchPath checks if a file path matches a glob pattern.
func matchPath(path, pattern string) bool {
	// Expand ~ to home directory concept.
	if strings.HasPrefix(pattern, "~") {
		// For matching purposes, treat ~ as a prefix match.
		pattern = strings.TrimPrefix(pattern, "~/")
	}

	matched, err := filepath.Match(pattern, path)
	if err != nil {
		// If the pattern is invalid, try simple prefix/contains matching.
		trimmedPattern := strings.TrimSuffix(pattern, "/**")
		trimmedPattern = strings.TrimSuffix(trimmedPattern, "/*")
		return strings.HasPrefix(path, trimmedPattern)
	}
	if matched {
		return true
	}

	// Also check if path is under a directory pattern.
	if strings.HasSuffix(pattern, "/**") {
		dirPattern := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(path, dirPattern)
	}

	return false
}

// AllCapabilities returns a snapshot of all agent capabilities.
func (e *Engine) AllCapabilities() map[string]AgentCapabilities {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make(map[string]AgentCapabilities, len(e.agents))
	for k, v := range e.agents {
		result[k] = v
	}
	return result
}
