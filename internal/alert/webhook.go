package alert

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/agentwarden/agentwarden/internal/config"
)

// WebhookSender sends alerts to a generic webhook endpoint.
type WebhookSender struct {
	url    string
	secret string
	client *http.Client
}

// NewWebhookSender creates a new generic webhook sender.
func NewWebhookSender(cfg config.WebhookAlertConfig) *WebhookSender {
	return &WebhookSender{
		url:    cfg.URL,
		secret: cfg.Secret,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WebhookSender) Name() string { return "webhook" }

// Send posts an alert to the webhook URL.
func (w *WebhookSender) Send(alert Alert) error {
	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "AgentWarden/1.0")

	// Sign payload if secret is configured
	if w.secret != "" {
		sig := computeHMAC(body, []byte(w.secret))
		req.Header.Set("X-AgentWarden-Signature", sig)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	return nil
}

func computeHMAC(data, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}
