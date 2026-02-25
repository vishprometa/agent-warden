package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// SlackSender sends alerts to Slack via incoming webhook.
type SlackSender struct {
	webhookURL string
	channel    string
	client     *http.Client
}

// NewSlackSender creates a new Slack alert sender.
func NewSlackSender(cfg config.SlackAlertConfig) *SlackSender {
	return &SlackSender{
		webhookURL: cfg.WebhookURL,
		channel:    cfg.Channel,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SlackSender) Name() string { return "slack" }

// Send posts an alert to Slack.
func (s *SlackSender) Send(alert Alert) error {
	emoji := severityEmoji(alert.Severity)
	color := severityColor(alert.Severity)

	payload := map[string]interface{}{
		"channel": s.channel,
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  fmt.Sprintf("%s AgentWarden: %s", emoji, alert.Title),
				"text":   alert.Message,
				"fields": buildSlackFields(alert),
				"ts":     alert.Timestamp.Unix(),
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal slack payload: %w", err)
	}

	resp, err := s.client.Post(s.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send slack webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}

	return nil
}

func buildSlackFields(alert Alert) []map[string]interface{} {
	fields := []map[string]interface{}{
		{"title": "Type", "value": alert.Type, "short": true},
		{"title": "Severity", "value": alert.Severity, "short": true},
	}
	if alert.AgentID != "" {
		fields = append(fields, map[string]interface{}{"title": "Agent", "value": alert.AgentID, "short": true})
	}
	if alert.SessionID != "" {
		fields = append(fields, map[string]interface{}{"title": "Session", "value": alert.SessionID, "short": true})
	}
	return fields
}

func severityEmoji(severity string) string {
	switch severity {
	case "critical":
		return "🔴"
	case "warning":
		return "🟡"
	default:
		return "🔵"
	}
}

func severityColor(severity string) string {
	switch severity {
	case "critical":
		return "#dc3545"
	case "warning":
		return "#ffc107"
	default:
		return "#17a2b8"
	}
}
