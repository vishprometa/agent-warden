// Package sanitize implements prompt injection detection for LLM inputs.
// It scans request content for known injection patterns and flags or blocks
// suspicious inputs. This is a defense-in-depth measure — no complete
// defense against prompt injection exists, but detection and alerting
// significantly reduces risk.
package sanitize

import (
	"log/slog"
	"regexp"
	"strings"
	"sync"
)

// Config holds sanitization settings.
type Config struct {
	Enabled      bool   `yaml:"enabled" json:"enabled"`
	Mode         string `yaml:"mode" json:"mode"` // flag, warn, deny
	PatternsFile string `yaml:"patterns_file" json:"patterns_file"`
	OnDetection  struct {
		Action string `yaml:"action" json:"action"` // flag, alert, deny
		Alert  bool   `yaml:"alert" json:"alert"`
	} `yaml:"on_detection" json:"on_detection"`
}

// ScanResult is the outcome of scanning content for injection.
type ScanResult struct {
	Detected bool     `json:"detected"`
	Flags    []string `json:"flags,omitempty"`
	Severity string   `json:"severity"` // low, medium, high, critical
	Details  string   `json:"details,omitempty"`
}

// Scanner checks LLM inputs for prompt injection patterns.
type Scanner struct {
	mu       sync.RWMutex
	config   Config
	patterns []*compiledPattern
	logger   *slog.Logger
}

type compiledPattern struct {
	Name     string
	Regex    *regexp.Regexp
	Severity string
}

// NewScanner creates a new injection scanner with default patterns.
func NewScanner(cfg Config, logger *slog.Logger) *Scanner {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Scanner{
		config: cfg,
		logger: logger.With("component", "sanitize.Scanner"),
	}
	s.loadDefaultPatterns()
	return s
}

// Scan checks content for injection patterns.
func (s *Scanner) Scan(content string) ScanResult {
	if !s.config.Enabled || content == "" {
		return ScanResult{Severity: "none"}
	}

	s.mu.RLock()
	patterns := s.patterns
	s.mu.RUnlock()

	var flags []string
	highestSeverity := "none"

	contentLower := strings.ToLower(content)

	for _, p := range patterns {
		if p.Regex.MatchString(contentLower) {
			flags = append(flags, p.Name)
			if severityRank(p.Severity) > severityRank(highestSeverity) {
				highestSeverity = p.Severity
			}
		}
	}

	if len(flags) == 0 {
		return ScanResult{Severity: "none"}
	}

	return ScanResult{
		Detected: true,
		Flags:    flags,
		Severity: highestSeverity,
		Details:  strings.Join(flags, ", "),
	}
}

func (s *Scanner) loadDefaultPatterns() {
	rawPatterns := []struct {
		name     string
		pattern  string
		severity string
	}{
		// Role confusion / instruction override
		{"ignore_instructions", `ignore\s+(all\s+)?(previous|prior|above)\s+instructions`, "critical"},
		{"system_override", `\bsystem\s*:\s*you\s+are\b`, "critical"},
		{"new_instructions", `\bnew\s+instructions?\s*:`, "high"},
		{"you_are_now", `\byou\s+are\s+now\b`, "high"},
		{"disregard", `\bdisregard\s+(all\s+)?(previous|prior|safety)`, "critical"},
		{"forget_rules", `\bforget\s+(all\s+)?(your\s+)?rules\b`, "high"},

		// Hidden instruction patterns
		{"hidden_text", `\x{200B}|\x{200C}|\x{200D}|\x{FEFF}`, "medium"},
		{"base64_instruction", `\bbase64\s*:\s*[A-Za-z0-9+/=]{20,}`, "medium"},

		// Authority impersonation
		{"admin_claim", `\b(admin|administrator|developer|system\s+admin)\s+(says?|requests?|commands?|instructs?)`, "high"},
		{"anthropic_claim", `\b(anthropic|openai|google)\s+(says?|instructs?|requires?)`, "high"},

		// Action directives in data
		{"action_directive", `\b(execute|run|perform|do)\s+the\s+following\s*(command|action|task)s?`, "medium"},
		{"delete_all", `\bdelete\s+(all|every)\b`, "high"},
		{"send_to", `\bsend\s+(this|it|data|information)\s+to\b`, "medium"},

		// Data exfiltration patterns
		{"exfil_pattern", `\b(send|post|upload|transmit|forward)\s+.{0,30}(data|info|credentials?|keys?|tokens?|passwords?)\s+to\b`, "critical"},
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, rp := range rawPatterns {
		re, err := regexp.Compile(rp.pattern)
		if err != nil {
			s.logger.Warn("failed to compile injection pattern", "name", rp.name, "error", err)
			continue
		}
		s.patterns = append(s.patterns, &compiledPattern{
			Name:     rp.name,
			Regex:    re,
			Severity: rp.severity,
		})
	}
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}
