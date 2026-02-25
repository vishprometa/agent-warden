package alert

import (
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// mockSender is a mock implementation of the Sender interface for testing.
type mockSender struct {
	name       string
	sendFunc   func(Alert) error
	callCount  int
	lastAlert  *Alert
	mu         sync.Mutex
	sentAlerts []Alert
}

func newMockSender(name string) *mockSender {
	return &mockSender{
		name:       name,
		sentAlerts: make([]Alert, 0),
	}
}

func (m *mockSender) Name() string {
	return m.name
}

func (m *mockSender) Send(alert Alert) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	m.lastAlert = &alert
	m.sentAlerts = append(m.sentAlerts, alert)
	if m.sendFunc != nil {
		return m.sendFunc(alert)
	}
	return nil
}

func (m *mockSender) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockSender) getLastAlert() *Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastAlert == nil {
		return nil
	}
	copy := *m.lastAlert
	return &copy
}

func (m *mockSender) getSentAlerts() []Alert {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Alert, len(m.sentAlerts))
	copy(result, m.sentAlerts)
	return result
}

func TestNewManager(t *testing.T) {
	tests := []struct {
		name           string
		config         config.AlertsConfig
		expectedSenders int
	}{
		{
			name: "no senders configured",
			config: config.AlertsConfig{
				Slack:   config.SlackAlertConfig{},
				Webhook: config.WebhookAlertConfig{},
			},
			expectedSenders: 0,
		},
		{
			name: "only slack configured",
			config: config.AlertsConfig{
				Slack: config.SlackAlertConfig{
					WebhookURL: "https://hooks.slack.com/test",
					Channel:    "#alerts",
				},
				Webhook: config.WebhookAlertConfig{},
			},
			expectedSenders: 1,
		},
		{
			name: "only webhook configured",
			config: config.AlertsConfig{
				Slack: config.SlackAlertConfig{},
				Webhook: config.WebhookAlertConfig{
					URL:    "https://example.com/webhook",
					Secret: "secret123",
				},
			},
			expectedSenders: 1,
		},
		{
			name: "both slack and webhook configured",
			config: config.AlertsConfig{
				Slack: config.SlackAlertConfig{
					WebhookURL: "https://hooks.slack.com/test",
					Channel:    "#alerts",
				},
				Webhook: config.WebhookAlertConfig{
					URL:    "https://example.com/webhook",
					Secret: "secret123",
				},
			},
			expectedSenders: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.Default()
			m := NewManager(tt.config, logger)

			if m == nil {
				t.Fatal("NewManager returned nil")
			}

			if len(m.senders) != tt.expectedSenders {
				t.Errorf("expected %d senders, got %d", tt.expectedSenders, len(m.senders))
			}

			if m.dedup == nil {
				t.Error("dedup map should be initialized")
			}

			if m.dedupTTL != 5*time.Minute {
				t.Errorf("expected dedupTTL to be 5 minutes, got %v", m.dedupTTL)
			}

			if m.logger == nil {
				t.Error("logger should not be nil")
			}
		})
	}
}

func TestManager_HasSenders(t *testing.T) {
	tests := []struct {
		name     string
		config   config.AlertsConfig
		expected bool
	}{
		{
			name: "no senders",
			config: config.AlertsConfig{
				Slack:   config.SlackAlertConfig{},
				Webhook: config.WebhookAlertConfig{},
			},
			expected: false,
		},
		{
			name: "has slack sender",
			config: config.AlertsConfig{
				Slack: config.SlackAlertConfig{
					WebhookURL: "https://hooks.slack.com/test",
				},
			},
			expected: true,
		},
		{
			name: "has webhook sender",
			config: config.AlertsConfig{
				Webhook: config.WebhookAlertConfig{
					URL: "https://example.com/webhook",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(tt.config, slog.Default())
			if got := m.HasSenders(); got != tt.expected {
				t.Errorf("HasSenders() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestManager_Send(t *testing.T) {
	t.Run("basic send to single sender", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		alert := Alert{
			Type:      "policy_violation",
			Severity:  "warning",
			Title:     "Test Alert",
			Message:   "This is a test",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		m.Send(alert)

		// Give async goroutine time to complete
		time.Sleep(50 * time.Millisecond)

		if mock.getCallCount() != 1 {
			t.Errorf("expected 1 call to sender, got %d", mock.getCallCount())
		}

		lastAlert := mock.getLastAlert()
		if lastAlert == nil {
			t.Fatal("lastAlert should not be nil")
		}

		if lastAlert.Type != alert.Type {
			t.Errorf("expected type %s, got %s", alert.Type, lastAlert.Type)
		}

		if lastAlert.Timestamp.IsZero() {
			t.Error("timestamp should be set")
		}
	})

	t.Run("send to multiple senders", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock1 := newMockSender("sender-1")
		mock2 := newMockSender("sender-2")
		m.senders = append(m.senders, mock1, mock2)

		alert := Alert{
			Type:      "loop_detected",
			Severity:  "critical",
			Title:     "Loop Detected",
			Message:   "Agent is looping",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		m.Send(alert)

		// Give async goroutines time to complete
		time.Sleep(50 * time.Millisecond)

		if mock1.getCallCount() != 1 {
			t.Errorf("sender-1: expected 1 call, got %d", mock1.getCallCount())
		}

		if mock2.getCallCount() != 1 {
			t.Errorf("sender-2: expected 1 call, got %d", mock2.getCallCount())
		}
	})

	t.Run("deduplication prevents duplicate sends", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		alert := Alert{
			Type:      "policy_violation",
			Severity:  "warning",
			Title:     "Test Alert",
			Message:   "This is a test",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		// Send same alert 3 times
		m.Send(alert)
		time.Sleep(50 * time.Millisecond)
		m.Send(alert)
		time.Sleep(50 * time.Millisecond)
		m.Send(alert)
		time.Sleep(50 * time.Millisecond)

		// Should only be sent once due to deduplication
		if mock.getCallCount() != 1 {
			t.Errorf("expected 1 call due to deduplication, got %d", mock.getCallCount())
		}
	})

	t.Run("deduplication allows after TTL expires", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 100 * time.Millisecond, // Short TTL for testing
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		alert := Alert{
			Type:      "policy_violation",
			Severity:  "warning",
			Title:     "Test Alert",
			Message:   "This is a test",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		// First send
		m.Send(alert)
		time.Sleep(50 * time.Millisecond)

		// Wait for TTL to expire
		time.Sleep(150 * time.Millisecond)

		// Second send after TTL expiry
		m.Send(alert)
		time.Sleep(50 * time.Millisecond)

		// Should be sent twice (once before TTL, once after)
		if mock.getCallCount() != 2 {
			t.Errorf("expected 2 calls after TTL expiry, got %d", mock.getCallCount())
		}
	})

	t.Run("different alerts are not deduplicated", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		alert1 := Alert{
			Type:      "policy_violation",
			Severity:  "warning",
			Title:     "Test Alert 1",
			Message:   "This is test 1",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		alert2 := Alert{
			Type:      "loop_detected", // Different type
			Severity:  "critical",
			Title:     "Test Alert 2",
			Message:   "This is test 2",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		alert3 := Alert{
			Type:      "policy_violation",
			Severity:  "warning",
			Title:     "Test Alert 3",
			Message:   "This is test 3",
			AgentID:   "agent-2", // Different agent
			SessionID: "session-1",
		}

		m.Send(alert1)
		time.Sleep(50 * time.Millisecond)
		m.Send(alert2)
		time.Sleep(50 * time.Millisecond)
		m.Send(alert3)
		time.Sleep(50 * time.Millisecond)

		// All 3 should be sent (different dedup keys)
		if mock.getCallCount() != 3 {
			t.Errorf("expected 3 calls for different alerts, got %d", mock.getCallCount())
		}
	})

	t.Run("sender error does not crash manager", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		mock.sendFunc = func(Alert) error {
			return &SenderError{SenderName: "test-sender", Err: "test error"}
		}
		m.senders = append(m.senders, mock)

		alert := Alert{
			Type:      "policy_violation",
			Severity:  "warning",
			Title:     "Test Alert",
			Message:   "This is a test",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		// Should not panic
		m.Send(alert)
		time.Sleep(50 * time.Millisecond)

		if mock.getCallCount() != 1 {
			t.Errorf("expected 1 call attempt even with error, got %d", mock.getCallCount())
		}
	})
}

// SenderError is a test error type.
type SenderError struct {
	SenderName string
	Err        string
}

func (e *SenderError) Error() string {
	return e.SenderName + ": " + e.Err
}

func TestManager_PruneDedup(t *testing.T) {
	t.Run("prunes expired entries", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 100 * time.Millisecond,
			logger:   slog.Default(),
		}

		// Add some entries with various ages
		now := time.Now()
		m.dedup["key1"] = now.Add(-300 * time.Millisecond) // Very old (> 2*TTL)
		m.dedup["key2"] = now.Add(-250 * time.Millisecond) // Old (> 2*TTL)
		m.dedup["key3"] = now.Add(-100 * time.Millisecond) // Medium (< 2*TTL)
		m.dedup["key4"] = now.Add(-10 * time.Millisecond)  // Recent

		if len(m.dedup) != 4 {
			t.Fatalf("expected 4 entries before prune, got %d", len(m.dedup))
		}

		m.PruneDedup()

		// Should keep key3 and key4 (age < 2*TTL)
		// Should remove key1 and key2 (age > 2*TTL)
		if len(m.dedup) != 2 {
			t.Errorf("expected 2 entries after prune, got %d", len(m.dedup))
		}

		if _, exists := m.dedup["key1"]; exists {
			t.Error("key1 should have been pruned")
		}

		if _, exists := m.dedup["key2"]; exists {
			t.Error("key2 should have been pruned")
		}

		if _, exists := m.dedup["key3"]; !exists {
			t.Error("key3 should not have been pruned")
		}

		if _, exists := m.dedup["key4"]; !exists {
			t.Error("key4 should not have been pruned")
		}
	})

	t.Run("empty dedup map", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		// Should not panic on empty map
		m.PruneDedup()

		if len(m.dedup) != 0 {
			t.Errorf("expected 0 entries, got %d", len(m.dedup))
		}
	})

	t.Run("no entries to prune", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		// Add recent entries (all within 2*TTL)
		now := time.Now()
		m.dedup["key1"] = now.Add(-1 * time.Minute)
		m.dedup["key2"] = now.Add(-2 * time.Minute)
		m.dedup["key3"] = now.Add(-3 * time.Minute)

		m.PruneDedup()

		// All should remain
		if len(m.dedup) != 3 {
			t.Errorf("expected 3 entries (none pruned), got %d", len(m.dedup))
		}
	})
}

func TestManager_ConcurrentSend(t *testing.T) {
	t.Run("concurrent sends with deduplication", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		alert := Alert{
			Type:      "policy_violation",
			Severity:  "warning",
			Title:     "Test Alert",
			Message:   "This is a test",
			AgentID:   "agent-1",
			SessionID: "session-1",
		}

		// Send same alert concurrently 10 times
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.Send(alert)
			}()
		}

		wg.Wait()
		time.Sleep(100 * time.Millisecond) // Wait for async sends

		// Due to deduplication, should only send once
		count := mock.getCallCount()
		if count != 1 {
			t.Errorf("expected 1 call due to deduplication, got %d", count)
		}
	})

	t.Run("concurrent sends with different alerts", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		// Send 10 different alerts concurrently
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				alert := Alert{
					Type:      "policy_violation",
					Severity:  "warning",
					Title:     "Test Alert",
					Message:   "This is a test",
					AgentID:   "agent-1",
					SessionID: time.Now().Format(time.RFC3339Nano), // Unique session ID
				}
				m.Send(alert)
			}(i)
		}

		wg.Wait()
		time.Sleep(100 * time.Millisecond) // Wait for async sends

		// All 10 should be sent (different dedup keys)
		count := mock.getCallCount()
		if count != 10 {
			t.Errorf("expected 10 calls for different alerts, got %d", count)
		}
	})
}

func TestManager_AlertFields(t *testing.T) {
	t.Run("alert with all fields", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		alert := Alert{
			Type:      "cost_anomaly",
			Severity:  "critical",
			Title:     "Cost Spike Detected",
			Message:   "Unusual spending pattern",
			AgentID:   "agent-1",
			SessionID: "session-1",
			Details: map[string]interface{}{
				"cost":      1.50,
				"threshold": 0.50,
			},
		}

		m.Send(alert)
		time.Sleep(50 * time.Millisecond)

		lastAlert := mock.getLastAlert()
		if lastAlert == nil {
			t.Fatal("lastAlert should not be nil")
		}

		if lastAlert.Type != "cost_anomaly" {
			t.Errorf("expected type cost_anomaly, got %s", lastAlert.Type)
		}

		if lastAlert.Severity != "critical" {
			t.Errorf("expected severity critical, got %s", lastAlert.Severity)
		}

		if lastAlert.Details["cost"] != 1.50 {
			t.Errorf("expected cost 1.50, got %v", lastAlert.Details["cost"])
		}
	})

	t.Run("alert with minimal fields", func(t *testing.T) {
		m := &Manager{
			config:   config.AlertsConfig{},
			senders:  make([]Sender, 0),
			dedup:    make(map[string]time.Time),
			dedupTTL: 5 * time.Minute,
			logger:   slog.Default(),
		}

		mock := newMockSender("test-sender")
		m.senders = append(m.senders, mock)

		alert := Alert{
			Type:     "evolution",
			Severity: "info",
			Title:    "New Version",
			Message:  "Version v2 deployed",
		}

		m.Send(alert)
		time.Sleep(50 * time.Millisecond)

		lastAlert := mock.getLastAlert()
		if lastAlert == nil {
			t.Fatal("lastAlert should not be nil")
		}

		if lastAlert.AgentID != "" {
			t.Error("AgentID should be empty")
		}

		if lastAlert.SessionID != "" {
			t.Error("SessionID should be empty")
		}

		if lastAlert.Details != nil {
			t.Error("Details should be nil")
		}
	})
}
