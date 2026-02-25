// Package auth implements authentication and authorization for AgentWarden.
// It provides rotating API tokens with short TTLs, RBAC, and mTLS support
// to address the CVE-2026-25253 class of gateway token theft attacks.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Role defines the access level for API tokens.
type Role string

const (
	RoleAgent    Role = "agent"    // can only evaluate actions
	RoleOperator Role = "operator" // can manage agents, sessions, approvals
	RoleAdmin    Role = "admin"    // full access including config changes
)

// Token represents an API token with metadata.
type Token struct {
	ID        string    `json:"id"`
	Secret    string    `json:"-"` // never serialized
	Role      Role      `json:"role"`
	AgentID   string    `json:"agent_id,omitempty"` // bound to specific agent (optional)
	SourceIP  string    `json:"source_ip,omitempty"` // IP binding (optional)
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsExpired returns whether the token has expired.
func (t Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// TokenManager handles API token creation, validation, and rotation.
type TokenManager struct {
	mu     sync.RWMutex
	tokens map[string]Token // secret → token
	ttl    time.Duration
	logger *slog.Logger
}

// NewTokenManager creates a new token manager.
func NewTokenManager(ttl time.Duration, logger *slog.Logger) *TokenManager {
	if ttl <= 0 {
		ttl = time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &TokenManager{
		tokens: make(map[string]Token),
		ttl:    ttl,
		logger: logger.With("component", "auth.TokenManager"),
	}
}

// CreateToken generates a new API token.
func (m *TokenManager) CreateToken(role Role, agentID, sourceIP string) (Token, error) {
	secret, err := generateSecret()
	if err != nil {
		return Token{}, fmt.Errorf("failed to generate token: %w", err)
	}

	id, err := generateSecret()
	if err != nil {
		return Token{}, fmt.Errorf("failed to generate token ID: %w", err)
	}

	token := Token{
		ID:        id[:16],
		Secret:    secret,
		Role:      role,
		AgentID:   agentID,
		SourceIP:  sourceIP,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(m.ttl),
	}

	m.mu.Lock()
	m.tokens[secret] = token
	m.mu.Unlock()

	m.logger.Info("token created",
		"token_id", token.ID,
		"role", role,
		"agent_id", agentID,
		"expires_at", token.ExpiresAt,
	)

	return token, nil
}

// ValidateToken checks if a token secret is valid and returns the token.
func (m *TokenManager) ValidateToken(secret, sourceIP string) (Token, error) {
	m.mu.RLock()
	token, ok := m.tokens[secret]
	m.mu.RUnlock()

	if !ok {
		return Token{}, fmt.Errorf("invalid token")
	}

	if token.IsExpired() {
		// Clean up expired token.
		m.mu.Lock()
		delete(m.tokens, secret)
		m.mu.Unlock()
		return Token{}, fmt.Errorf("token expired")
	}

	// Check IP binding.
	if token.SourceIP != "" && token.SourceIP != sourceIP {
		m.logger.Warn("token used from wrong IP",
			"token_id", token.ID,
			"expected_ip", token.SourceIP,
			"actual_ip", sourceIP,
		)
		return Token{}, fmt.Errorf("token not valid from this IP")
	}

	return token, nil
}

// RevokeToken removes a token.
func (m *TokenManager) RevokeToken(secret string) {
	m.mu.Lock()
	if token, ok := m.tokens[secret]; ok {
		m.logger.Info("token revoked", "token_id", token.ID)
		delete(m.tokens, secret)
	}
	m.mu.Unlock()
}

// CleanExpired removes all expired tokens.
func (m *TokenManager) CleanExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for secret, token := range m.tokens {
		if token.IsExpired() {
			delete(m.tokens, secret)
			count++
		}
	}
	return count
}

// ActiveTokenCount returns the number of active (non-expired) tokens.
func (m *TokenManager) ActiveTokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, token := range m.tokens {
		if !token.IsExpired() {
			count++
		}
	}
	return count
}

// HasPermission checks if a role has permission for an action.
func HasPermission(role Role, action string) bool {
	switch role {
	case RoleAdmin:
		return true
	case RoleOperator:
		return action != "config.change" && action != "token.create"
	case RoleAgent:
		return action == "evaluate" || action == "trace" || action == "session.start" || action == "session.end"
	default:
		return false
	}
}

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
