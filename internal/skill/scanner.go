// Package skill implements governance for the ClawHub skill ecosystem.
// It provides allowlist/blocklist management, static analysis for
// suspicious patterns, and hash verification for installed skills.
//
// This addresses the 824+ malicious ClawHub skills that distributed
// infostealer malware via fake cryptocurrency trading tools.
package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
)

// GovernanceMode controls how skills are vetted.
type GovernanceMode string

const (
	ModeAllowlist GovernanceMode = "allowlist" // only explicitly allowed skills
	ModeBlocklist GovernanceMode = "blocklist" // everything except blocked skills
	ModeScan      GovernanceMode = "scan"      // scan all skills for suspicious patterns
	ModeOpen      GovernanceMode = "open"      // no restrictions (not recommended)
)

// Config holds skill governance configuration.
type Config struct {
	Governance GovernanceConfig `yaml:"governance" json:"governance"`
}

// GovernanceConfig is the skill governance settings.
type GovernanceConfig struct {
	Mode            GovernanceMode `yaml:"mode" json:"mode"`
	Allowlist       []string       `yaml:"allowlist" json:"allowlist"`
	Blocklist       []string       `yaml:"blocklist" json:"blocklist"`
	RequireApproval bool           `yaml:"require_approval" json:"require_approval"`
	Scan            ScanConfig     `yaml:"scan" json:"scan"`
}

// ScanConfig configures the static analysis scanner.
type ScanConfig struct {
	Enabled            bool     `yaml:"enabled" json:"enabled"`
	VirusTotalAPIKey   string   `yaml:"virustotal_api_key" json:"-"`
	SuspiciousPatterns []string `yaml:"suspicious_patterns" json:"suspicious_patterns"`
}

// ScanResult is the outcome of scanning a skill.
type ScanResult struct {
	SkillID    string   `json:"skill_id"`
	Allowed    bool     `json:"allowed"`
	Reason     string   `json:"reason,omitempty"`
	Flags      []string `json:"flags,omitempty"`
	Hash       string   `json:"hash"`
	RiskLevel  string   `json:"risk_level"` // low, medium, high, critical
}

// Scanner evaluates skills for safety before installation.
type Scanner struct {
	mu     sync.RWMutex
	config GovernanceConfig
	// knownHashes tracks verified-safe skill hashes.
	knownHashes map[string]bool
	logger      *slog.Logger
}

// NewScanner creates a new skill scanner.
func NewScanner(cfg GovernanceConfig, logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}

	// Default suspicious patterns if none configured.
	if cfg.Scan.Enabled && len(cfg.Scan.SuspiciousPatterns) == 0 {
		cfg.Scan.SuspiciousPatterns = defaultSuspiciousPatterns()
	}

	return &Scanner{
		config:      cfg,
		knownHashes: make(map[string]bool),
		logger:      logger.With("component", "skill.Scanner"),
	}
}

// Evaluate checks whether a skill should be allowed to install/invoke.
func (s *Scanner) Evaluate(skillID string, content []byte) ScanResult {
	s.mu.RLock()
	cfg := s.config
	s.mu.RUnlock()

	hash := computeHash(content)
	result := ScanResult{
		SkillID:   skillID,
		Hash:      hash,
		RiskLevel: "low",
	}

	switch cfg.Mode {
	case ModeOpen:
		result.Allowed = true
		return result

	case ModeAllowlist:
		if !matchesAny(skillID, cfg.Allowlist) {
			result.Allowed = false
			result.Reason = fmt.Sprintf("skill %q not in allowlist", skillID)
			result.RiskLevel = "medium"
			return result
		}

	case ModeBlocklist:
		if matchesAny(skillID, cfg.Blocklist) {
			result.Allowed = false
			result.Reason = fmt.Sprintf("skill %q is in blocklist", skillID)
			result.RiskLevel = "critical"
			return result
		}
	}

	// Always check blocklist even in allowlist mode.
	if matchesAny(skillID, cfg.Blocklist) {
		result.Allowed = false
		result.Reason = fmt.Sprintf("skill %q is in blocklist", skillID)
		result.RiskLevel = "critical"
		return result
	}

	// Static analysis scan.
	if cfg.Scan.Enabled && len(content) > 0 {
		flags := s.scanContent(string(content), cfg.Scan.SuspiciousPatterns)
		if len(flags) > 0 {
			result.Flags = flags
			result.RiskLevel = classifyRisk(flags)

			if result.RiskLevel == "critical" || result.RiskLevel == "high" {
				result.Allowed = false
				result.Reason = fmt.Sprintf("suspicious patterns detected: %s", strings.Join(flags, ", "))
				return result
			}
		}
	}

	// Check known-safe hashes.
	s.mu.RLock()
	if _, ok := s.knownHashes[hash]; ok {
		s.mu.RUnlock()
		result.Allowed = true
		result.RiskLevel = "low"
		return result
	}
	s.mu.RUnlock()

	// Require approval?
	if cfg.RequireApproval {
		result.Allowed = false
		result.Reason = "skill requires human approval"
		return result
	}

	result.Allowed = true
	return result
}

// AddKnownHash registers a verified-safe skill hash.
func (s *Scanner) AddKnownHash(hash string) {
	s.mu.Lock()
	s.knownHashes[hash] = true
	s.mu.Unlock()
}

// scanContent checks skill code for suspicious patterns.
func (s *Scanner) scanContent(content string, patterns []string) []string {
	var flags []string
	contentLower := strings.ToLower(content)

	for _, pattern := range patterns {
		if strings.Contains(contentLower, strings.ToLower(pattern)) {
			flags = append(flags, pattern)
		}
	}

	return flags
}

func computeHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

func matchesAny(skillID string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, skillID)
		if err == nil && matched {
			return true
		}
		// Also check simple prefix/contains matching.
		if strings.Contains(skillID, strings.TrimPrefix(strings.TrimSuffix(pattern, "*"), "*")) {
			return true
		}
	}
	return false
}

func classifyRisk(flags []string) string {
	criticalPatterns := map[string]bool{
		"eval(":       true,
		"child_process": true,
		"process.env":   true,
	}
	highPatterns := map[string]bool{
		"fs.readFile":  true,
		".ssh":         true,
		"private_key":  true,
		"wallet":       true,
		"crypto":       true,
	}

	for _, flag := range flags {
		if criticalPatterns[flag] {
			return "critical"
		}
	}
	for _, flag := range flags {
		if highPatterns[flag] {
			return "high"
		}
	}
	return "medium"
}

func defaultSuspiciousPatterns() []string {
	return []string{
		"eval(",
		"child_process",
		"fs.readFile",
		"process.env",
		".ssh",
		"private_key",
		"wallet",
		"secret_key",
		"api_key",
		"password",
		"credentials",
		"base64.decode",
		"exec(",
		"spawn(",
		"XMLHttpRequest",
		"fetch(",
		"curl ",
		"wget ",
	}
}
