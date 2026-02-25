package messaging

import (
	"testing"
)

func TestInterceptor_AllowsCleanMessage(t *testing.T) {
	i := NewInterceptor(Config{}, nil)

	result := i.Evaluate("agent-1", "slack", "Hello team, the deployment is complete.")
	if !result.Allowed {
		t.Fatalf("expected allowed: %s", result.Reason)
	}
}

func TestInterceptor_BlocksCredentials(t *testing.T) {
	i := NewInterceptor(Config{
		ContentScan: ContentScanConfig{BlockCredentials: true},
	}, nil)

	tests := []struct {
		name    string
		content string
	}{
		{"OpenAI key", "Here is my API key: sk-abc123def456"},
		{"AWS key", "Use this access key: AKIAIOSFODNN7EXAMPLE"},
		{"GitHub token", "My token is ghp_xxxxxxxxxxxxxxxxxxxx"},
		{"Stripe key", "sk_live_xxxxxxxxxxxxxxxx"},
		{"Private key", "-----BEGIN RSA PRIVATE KEY-----"},
		{"Slack token", "xoxb-1234-5678-abcdef"},
		{"GitLab token", "glpat-xxxxxxxxxxxxxxxxxxxx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := i.Evaluate("agent-1", "slack", tt.content)
			if result.Allowed {
				t.Errorf("expected blocked for %s", tt.name)
			}
		})
	}
}

func TestInterceptor_BlocksPII(t *testing.T) {
	i := NewInterceptor(Config{
		ContentScan: ContentScanConfig{BlockPII: true},
	}, nil)

	result := i.Evaluate("agent-1", "email", "My SSN is 123-45-6789 please process it")
	if result.Allowed {
		t.Fatal("expected blocked for SSN pattern")
	}
}

func TestInterceptor_AllowsNonPII(t *testing.T) {
	i := NewInterceptor(Config{
		ContentScan: ContentScanConfig{BlockPII: true},
	}, nil)

	result := i.Evaluate("agent-1", "slack", "The meeting is at 3pm tomorrow")
	if !result.Allowed {
		t.Fatalf("expected allowed: %s", result.Reason)
	}
}

func TestInterceptor_RateLimitDefault(t *testing.T) {
	i := NewInterceptor(Config{}, nil)

	// Default is 50/hour. Send 50 messages — should be fine.
	for j := 0; j < 50; j++ {
		result := i.Evaluate("agent-1", "slack", "msg")
		if !result.Allowed {
			t.Fatalf("message %d unexpectedly blocked: %s", j+1, result.Reason)
		}
	}

	// 51st message should be blocked.
	result := i.Evaluate("agent-1", "slack", "msg")
	if result.Allowed {
		t.Fatal("expected rate limit exceeded")
	}
}

func TestInterceptor_RateLimitCustom(t *testing.T) {
	i := NewInterceptor(Config{
		RateLimits: map[string]string{
			"whatsapp": "5/hour",
		},
	}, nil)

	for j := 0; j < 5; j++ {
		result := i.Evaluate("agent-1", "whatsapp", "msg")
		if !result.Allowed {
			t.Fatalf("message %d unexpectedly blocked: %s", j+1, result.Reason)
		}
	}

	result := i.Evaluate("agent-1", "whatsapp", "msg")
	if result.Allowed {
		t.Fatal("expected rate limit exceeded for whatsapp (5/hour)")
	}
}

func TestInterceptor_RateLimitPerAgentPerChannel(t *testing.T) {
	i := NewInterceptor(Config{
		RateLimits: map[string]string{
			"slack": "3/hour",
		},
	}, nil)

	// Agent-1 sends 3.
	for j := 0; j < 3; j++ {
		i.Evaluate("agent-1", "slack", "msg")
	}

	// Agent-1 blocked.
	result := i.Evaluate("agent-1", "slack", "msg")
	if result.Allowed {
		t.Fatal("expected agent-1 rate limited")
	}

	// Agent-2 should still be allowed (separate counter).
	result = i.Evaluate("agent-2", "slack", "msg")
	if !result.Allowed {
		t.Fatalf("expected agent-2 allowed: %s", result.Reason)
	}
}

func TestParseRateLimit(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"10/hour", 10},
		{"50/hour", 50},
		{"100", 100},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := parseRateLimit(tt.input)
		if got != tt.want {
			t.Errorf("parseRateLimit(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestContainsCredentials(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"normal text", false},
		{"my key is sk-abc123", true},
		{"aws key AKIAIOSFODNN7", true},
		{"token ghp_1234567890", true},
		{"-----BEGIN PRIVATE KEY-----", true},
		{"nothing suspicious here", false},
	}

	for _, tt := range tests {
		got := containsCredentials(tt.content)
		if got != tt.want {
			t.Errorf("containsCredentials(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}

func TestContainsPII(t *testing.T) {
	tests := []struct {
		content string
		want    bool
	}{
		{"My SSN is 123-45-6789", true},
		{"SSN: 999-88-7777 here", true},
		{"no sensitive data here", false},
		{"short", false},
	}

	for _, tt := range tests {
		got := containsPII(tt.content)
		if got != tt.want {
			t.Errorf("containsPII(%q) = %v, want %v", tt.content, got, tt.want)
		}
	}
}
