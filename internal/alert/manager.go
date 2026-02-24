package alert

import (
	"log/slog"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// Alert represents a notification to be sent.
type Alert struct {
	Type      string                 `json:"type"`      // policy_violation, budget_breach, loop_detected, cost_anomaly, spiral, evolution
	Severity  string                 `json:"severity"`  // info, warning, critical
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	AgentID   string                 `json:"agent_id,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// Manager orchestrates alert delivery with deduplication.
type Manager struct {
	mu       sync.Mutex
	config   config.AlertsConfig
	senders  []Sender
	dedup    map[string]time.Time // dedupKey → lastSent
	dedupTTL time.Duration
	logger   *slog.Logger
}

// Sender is an interface for alert delivery channels.
type Sender interface {
	Send(alert Alert) error
	Name() string
}

// NewManager creates a new alert manager.
func NewManager(cfg config.AlertsConfig, logger *slog.Logger) *Manager {
	m := &Manager{
		config:   cfg,
		senders:  make([]Sender, 0),
		dedup:    make(map[string]time.Time),
		dedupTTL: 5 * time.Minute,
		logger:   logger,
	}

	// Register configured senders
	if cfg.Slack.WebhookURL != "" {
		m.senders = append(m.senders, NewSlackSender(cfg.Slack))
	}
	if cfg.Webhook.URL != "" {
		m.senders = append(m.senders, NewWebhookSender(cfg.Webhook))
	}

	return m
}

// Send dispatches an alert to all configured channels with deduplication.
func (m *Manager) Send(alert Alert) {
	alert.Timestamp = time.Now()

	// Deduplication check
	dedupKey := alert.Type + "|" + alert.AgentID + "|" + alert.SessionID
	m.mu.Lock()
	if lastSent, ok := m.dedup[dedupKey]; ok && time.Since(lastSent) < m.dedupTTL {
		m.mu.Unlock()
		m.logger.Debug("alert deduplicated", "type", alert.Type, "key", dedupKey)
		return
	}
	m.dedup[dedupKey] = time.Now()
	m.mu.Unlock()

	// Dispatch to all senders (async)
	for _, sender := range m.senders {
		go func(s Sender) {
			if err := s.Send(alert); err != nil {
				m.logger.Error("failed to send alert",
					"sender", s.Name(),
					"type", alert.Type,
					"error", err,
				)
			}
		}(sender)
	}
}

// PruneDedup removes old dedup entries. Call periodically.
func (m *Manager) PruneDedup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for key, ts := range m.dedup {
		if now.Sub(ts) > m.dedupTTL*2 {
			delete(m.dedup, key)
		}
	}
}

// HasSenders returns true if any alert channels are configured.
func (m *Manager) HasSenders() bool {
	return len(m.senders) > 0
}
