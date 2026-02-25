package auth

import (
	"testing"
	"time"
)

func TestTokenManager_CreateAndValidate(t *testing.T) {
	m := NewTokenManager(time.Hour, nil)

	token, err := m.CreateToken(RoleAgent, "agent-1", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if token.Secret == "" {
		t.Fatal("expected non-empty secret")
	}
	if token.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if token.Role != RoleAgent {
		t.Errorf("role = %q, want %q", token.Role, RoleAgent)
	}

	// Validate.
	validated, err := m.ValidateToken(token.Secret, "")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if validated.ID != token.ID {
		t.Errorf("validated ID = %q, want %q", validated.ID, token.ID)
	}
}

func TestTokenManager_InvalidToken(t *testing.T) {
	m := NewTokenManager(time.Hour, nil)

	_, err := m.ValidateToken("bogus-token", "")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestTokenManager_ExpiredToken(t *testing.T) {
	m := NewTokenManager(10*time.Millisecond, nil)

	token, err := m.CreateToken(RoleAgent, "", "")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)

	_, err = m.ValidateToken(token.Secret, "")
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestTokenManager_IPBinding(t *testing.T) {
	m := NewTokenManager(time.Hour, nil)

	token, err := m.CreateToken(RoleAgent, "", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	// Valid from correct IP.
	_, err = m.ValidateToken(token.Secret, "10.0.0.1")
	if err != nil {
		t.Fatalf("expected valid from correct IP: %v", err)
	}

	// Invalid from wrong IP.
	_, err = m.ValidateToken(token.Secret, "10.0.0.2")
	if err == nil {
		t.Fatal("expected error for wrong IP")
	}
}

func TestTokenManager_NoIPBinding(t *testing.T) {
	m := NewTokenManager(time.Hour, nil)

	token, err := m.CreateToken(RoleAgent, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// No IP binding — any IP should work.
	_, err = m.ValidateToken(token.Secret, "192.168.1.1")
	if err != nil {
		t.Fatalf("expected valid from any IP: %v", err)
	}
}

func TestTokenManager_Revoke(t *testing.T) {
	m := NewTokenManager(time.Hour, nil)

	token, err := m.CreateToken(RoleAgent, "", "")
	if err != nil {
		t.Fatal(err)
	}

	m.RevokeToken(token.Secret)

	_, err = m.ValidateToken(token.Secret, "")
	if err == nil {
		t.Fatal("expected error after revoke")
	}
}

func TestTokenManager_CleanExpired(t *testing.T) {
	m := NewTokenManager(10*time.Millisecond, nil)

	// Create a few tokens.
	for i := 0; i < 5; i++ {
		m.CreateToken(RoleAgent, "", "")
	}

	time.Sleep(50 * time.Millisecond)

	cleaned := m.CleanExpired()
	if cleaned != 5 {
		t.Errorf("cleaned = %d, want 5", cleaned)
	}

	if m.ActiveTokenCount() != 0 {
		t.Errorf("active count = %d, want 0", m.ActiveTokenCount())
	}
}

func TestTokenManager_ActiveTokenCount(t *testing.T) {
	m := NewTokenManager(time.Hour, nil)

	if m.ActiveTokenCount() != 0 {
		t.Errorf("initial count = %d, want 0", m.ActiveTokenCount())
	}

	m.CreateToken(RoleAgent, "", "")
	m.CreateToken(RoleOperator, "", "")
	m.CreateToken(RoleAdmin, "", "")

	if m.ActiveTokenCount() != 3 {
		t.Errorf("count = %d, want 3", m.ActiveTokenCount())
	}
}

func TestTokenManager_DefaultTTL(t *testing.T) {
	m := NewTokenManager(0, nil) // should default to 1 hour

	token, err := m.CreateToken(RoleAgent, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Token should expire approximately 1 hour from now.
	if token.ExpiresAt.Before(time.Now().Add(59 * time.Minute)) {
		t.Error("expected token to expire in approximately 1 hour")
	}
}

func TestToken_IsExpired(t *testing.T) {
	token := Token{ExpiresAt: time.Now().Add(-time.Minute)}
	if !token.IsExpired() {
		t.Error("expected expired")
	}

	token = Token{ExpiresAt: time.Now().Add(time.Hour)}
	if token.IsExpired() {
		t.Error("expected not expired")
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		role   Role
		action string
		want   bool
	}{
		// Admin can do everything.
		{RoleAdmin, "evaluate", true},
		{RoleAdmin, "config.change", true},
		{RoleAdmin, "token.create", true},

		// Operator can do most things.
		{RoleOperator, "evaluate", true},
		{RoleOperator, "config.change", false},
		{RoleOperator, "token.create", false},

		// Agent is limited.
		{RoleAgent, "evaluate", true},
		{RoleAgent, "trace", true},
		{RoleAgent, "session.start", true},
		{RoleAgent, "session.end", true},
		{RoleAgent, "config.change", false},
		{RoleAgent, "kill", false},

		// Unknown role.
		{Role("unknown"), "evaluate", false},
	}

	for _, tt := range tests {
		got := HasPermission(tt.role, tt.action)
		if got != tt.want {
			t.Errorf("HasPermission(%q, %q) = %v, want %v", tt.role, tt.action, got, tt.want)
		}
	}
}
