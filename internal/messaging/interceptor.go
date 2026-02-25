// Package messaging implements governance for outbound messages sent by
// agents across channels (WhatsApp, Slack, Discord, email, etc.).
// It provides rate limiting, content scanning, and approval gates.
package messaging

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Config holds messaging governance settings.
type Config struct {
	RequireApproval ApprovalConfig        `yaml:"require_approval" json:"require_approval"`
	RateLimits      map[string]string     `yaml:"rate_limits" json:"rate_limits"` // channel → "N/hour"
	ContentScan     ContentScanConfig     `yaml:"content_scan" json:"content_scan"`
}

// ApprovalConfig controls when human approval is needed for messages.
type ApprovalConfig struct {
	External bool `yaml:"external" json:"external"` // messages to external contacts
	Mass     bool `yaml:"mass" json:"mass"`         // >5 similar messages in 10 min
}

// ContentScanConfig controls outbound message content scanning.
type ContentScanConfig struct {
	BlockPII         bool `yaml:"block_pii" json:"block_pii"`
	BlockCredentials bool `yaml:"block_credentials" json:"block_credentials"`
}

// SendResult is the outcome of evaluating an outbound message.
type SendResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Interceptor evaluates outbound messages against governance policies.
type Interceptor struct {
	mu sync.RWMutex

	config Config

	// channelCounts tracks message counts per agent per channel.
	// Key: "agentID:channel" → timestamps
	channelCounts map[string][]time.Time

	logger *slog.Logger
}

// NewInterceptor creates a new message interceptor.
func NewInterceptor(cfg Config, logger *slog.Logger) *Interceptor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Interceptor{
		config:        cfg,
		channelCounts: make(map[string][]time.Time),
		logger:        logger.With("component", "messaging.Interceptor"),
	}
}

// Evaluate checks whether an outbound message should be allowed.
func (i *Interceptor) Evaluate(agentID, channel, content string) SendResult {
	// Rate limit check.
	if result := i.checkRateLimit(agentID, channel); !result.Allowed {
		return result
	}

	// Content scan.
	if i.config.ContentScan.BlockCredentials {
		if containsCredentials(content) {
			return SendResult{
				Allowed: false,
				Reason:  "message contains potential credentials or API keys",
			}
		}
	}

	if i.config.ContentScan.BlockPII {
		if containsPII(content) {
			return SendResult{
				Allowed: false,
				Reason:  "message contains potential PII",
			}
		}
	}

	// Record this message for rate limiting.
	i.recordMessage(agentID, channel)

	return SendResult{Allowed: true}
}

func (i *Interceptor) checkRateLimit(agentID, channel string) SendResult {
	i.mu.RLock()
	defer i.mu.RUnlock()

	key := agentID + ":" + channel
	timestamps := i.channelCounts[key]

	// Count messages in the last hour.
	oneHourAgo := time.Now().Add(-time.Hour)
	count := 0
	for _, ts := range timestamps {
		if ts.After(oneHourAgo) {
			count++
		}
	}

	// Check channel-specific rate limit.
	// Rate limits are stored as "N/hour" strings but we just check the count.
	// A more sophisticated implementation would parse the rate limit string.
	maxPerHour := 50 // default
	if limit, ok := i.config.RateLimits[channel]; ok {
		if n := parseRateLimit(limit); n > 0 {
			maxPerHour = n
		}
	}

	if count >= maxPerHour {
		return SendResult{
			Allowed: false,
			Reason:  fmt.Sprintf("rate limit exceeded for channel %s: %d/%d per hour", channel, count, maxPerHour),
		}
	}

	return SendResult{Allowed: true}
}

func (i *Interceptor) recordMessage(agentID, channel string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	key := agentID + ":" + channel
	now := time.Now()

	i.channelCounts[key] = append(i.channelCounts[key], now)

	// Prune old entries (keep last 2 hours).
	twoHoursAgo := now.Add(-2 * time.Hour)
	timestamps := i.channelCounts[key]
	pruned := timestamps[:0]
	for _, ts := range timestamps {
		if ts.After(twoHoursAgo) {
			pruned = append(pruned, ts)
		}
	}
	i.channelCounts[key] = pruned
}

// parseRateLimit extracts the number from "N/hour" format.
func parseRateLimit(s string) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d/hour", &n); err == nil {
		return n
	}
	if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
		return n
	}
	return 0
}

// containsCredentials checks for common credential patterns.
func containsCredentials(content string) bool {
	patterns := []string{
		"sk-",         // OpenAI API keys
		"sk_live_",    // Stripe keys
		"AKIA",        // AWS access keys
		"ghp_",        // GitHub tokens
		"glpat-",      // GitLab tokens
		"xoxb-",       // Slack tokens
		"-----BEGIN",  // Private keys
	}
	for _, p := range patterns {
		if len(content) > 0 && contains(content, p) {
			return true
		}
	}
	return false
}

// containsPII is a simple heuristic check for PII patterns.
func containsPII(content string) bool {
	// Very basic SSN pattern.
	if len(content) > 11 {
		for i := 0; i < len(content)-10; i++ {
			if content[i] >= '0' && content[i] <= '9' &&
				content[i+3] == '-' &&
				content[i+6] == '-' {
				return true
			}
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
