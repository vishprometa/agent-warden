// Package openclaw implements the AgentWarden adapter for the OpenClaw
// autonomous agent framework. It acts as a transparent WebSocket reverse
// proxy between OpenClaw agents and the OpenClaw gateway, intercepting
// all actions and routing them through AgentWarden's governance pipeline.
//
// No modifications to OpenClaw are required. The user simply points
// OpenClaw's gateway URL to AgentWarden's proxy port.
package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agentwarden/agentwarden/internal/adapter"
	"github.com/agentwarden/agentwarden/internal/approval"
	"github.com/agentwarden/agentwarden/internal/policy"
	"github.com/gorilla/websocket"
)

// newWSUpgrader creates a WebSocket upgrader. When allowAllOrigins is false,
// only same-origin requests are accepted.
func newWSUpgrader(allowAllOrigins bool) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if allowAllOrigins {
				return true
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			return strings.Contains(origin, r.Host)
		},
	}
}

// GatewayAdapter proxies WebSocket traffic between OpenClaw agents and
// the real OpenClaw gateway. Every message is inspected, translated into
// an AgentWarden ActionContext, and evaluated against policies before
// being forwarded upstream.
type GatewayAdapter struct {
	mu            sync.RWMutex
	config        Config
	evaluator     func(policy.ActionContext) policy.PolicyResult
	approvalQueue *approval.Queue
	conns         map[string]*agentConn // sessionID → connection pair
	wsUpgrader    websocket.Upgrader
	logger        *slog.Logger
	cancel        context.CancelFunc
}

// agentConn tracks a proxied WebSocket connection pair.
type agentConn struct {
	agentID     string
	sessionID   string
	agent       *websocket.Conn // agent → AgentWarden
	upstream    *websocket.Conn // AgentWarden → OpenClaw gateway
	cleanupOnce sync.Once
}

// NewGatewayAdapter creates a new OpenClaw gateway adapter.
func NewGatewayAdapter(cfg Config, allowAllOrigins bool, approvalQ *approval.Queue, logger *slog.Logger) *GatewayAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &GatewayAdapter{
		config:        cfg,
		conns:         make(map[string]*agentConn),
		approvalQueue: approvalQ,
		wsUpgrader:    newWSUpgrader(allowAllOrigins),
		logger:        logger.With("component", "adapter.openclaw"),
	}
}

// Name implements adapter.Adapter.
func (a *GatewayAdapter) Name() string { return "openclaw" }

// Start implements adapter.Adapter.
func (a *GatewayAdapter) Start(ctx context.Context, evaluator func(policy.ActionContext) policy.PolicyResult) error {
	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.evaluator = evaluator

	a.logger.Info("OpenClaw gateway adapter started",
		"mode", a.config.Mode,
		"gateway_url", a.config.GatewayURL,
	)

	// Keep the adapter alive until context is cancelled.
	<-ctx.Done()
	return nil
}

// Stop implements adapter.Adapter.
func (a *GatewayAdapter) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, conn := range a.conns {
		_ = conn.agent.Close()
		if conn.upstream != nil {
			_ = conn.upstream.Close()
		}
	}
	a.conns = make(map[string]*agentConn)

	a.logger.Info("OpenClaw gateway adapter stopped")
	return nil
}

// KillAll implements adapter.Adapter.
func (a *GatewayAdapter) KillAll() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for id, conn := range a.conns {
		_ = conn.agent.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "kill switch activated"),
			time.Now().Add(time.Second),
		)
		_ = conn.agent.Close()
		if conn.upstream != nil {
			_ = conn.upstream.Close()
		}
		delete(a.conns, id)
	}

	a.logger.Error("KillAll: all OpenClaw connections terminated")
}

// KillAgent implements adapter.Adapter.
func (a *GatewayAdapter) KillAgent(agentID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for id, conn := range a.conns {
		if conn.agentID == agentID {
			_ = conn.agent.Close()
			if conn.upstream != nil {
				_ = conn.upstream.Close()
			}
			delete(a.conns, id)
		}
	}

	a.logger.Error("KillAgent: connections terminated", "agent_id", agentID)
}

// KillSession implements adapter.Adapter.
func (a *GatewayAdapter) KillSession(sessionID string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if conn, ok := a.conns[sessionID]; ok {
		_ = conn.agent.Close()
		if conn.upstream != nil {
			_ = conn.upstream.Close()
		}
		delete(a.conns, sessionID)
	}

	a.logger.Error("KillSession: connection terminated", "session_id", sessionID)
}

// ConnectedAgents implements adapter.Adapter.
func (a *GatewayAdapter) ConnectedAgents() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.conns)
}

// HandleWebSocket is the HTTP handler for incoming agent WebSocket connections.
// Mount this at the gateway proxy path (e.g., /gateway).
func (a *GatewayAdapter) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade the agent's connection.
	agentWS, err := a.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Error("failed to upgrade agent websocket", "error", err)
		return
	}

	// Connect to the real OpenClaw gateway upstream.
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	upstreamHeaders := http.Header{}
	if a.config.AuthToken != "" {
		upstreamHeaders.Set("Authorization", "Bearer "+a.config.AuthToken)
	}

	upstreamWS, _, err := dialer.Dial(a.config.GatewayURL, upstreamHeaders)
	if err != nil {
		a.logger.Error("failed to connect to upstream gateway", "error", err, "url", a.config.GatewayURL)
		_ = agentWS.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "upstream gateway unavailable"))
		_ = agentWS.Close()
		return
	}

	// Extract agent/session IDs from headers or first message.
	agentID := r.Header.Get("X-OpenClaw-Agent-Id")
	sessionID := r.Header.Get("X-OpenClaw-Session-Id")
	if sessionID == "" {
		sessionID = fmt.Sprintf("oc_%d", time.Now().UnixNano())
	}

	conn := &agentConn{
		agentID:   agentID,
		sessionID: sessionID,
		agent:     agentWS,
		upstream:  upstreamWS,
	}

	a.mu.Lock()
	a.conns[sessionID] = conn
	a.mu.Unlock()

	a.logger.Info("OpenClaw agent connected",
		"agent_id", agentID,
		"session_id", sessionID,
	)

	// Bidirectional proxy with governance interception.
	go a.proxyAgentToUpstream(conn)
	go a.proxyUpstreamToAgent(conn)
}

// proxyAgentToUpstream reads messages from the agent, evaluates them
// against policies, and forwards allowed messages to the upstream gateway.
func (a *GatewayAdapter) proxyAgentToUpstream(conn *agentConn) {
	defer a.cleanupConn(conn)

	for {
		msgType, data, err := conn.agent.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				a.logger.Debug("agent connection closed", "session_id", conn.sessionID, "error", err)
			}
			return
		}

		// Parse the message to extract action information.
		var msg map[string]interface{}
		if err := json.Unmarshal(data, &msg); err != nil {
			// Not JSON — forward as-is (binary/ping frames).
			if writeErr := conn.upstream.WriteMessage(msgType, data); writeErr != nil {
				return
			}
			continue
		}

		// Update agent ID if present in message.
		if id, ok := msg["agent_id"].(string); ok && id != "" {
			conn.agentID = id
		}

		// Translate to ActionContext and evaluate.
		actionCtx := TranslateEvent(msg, conn.agentID, conn.sessionID)
		if a.evaluator != nil && actionCtx.Action.Type != "" {
			result := a.evaluator(actionCtx)

			switch result.Effect {
			case "deny", "terminate":
				// Send denial back to agent.
				denial := map[string]interface{}{
					"type":    "governance_denied",
					"effect":  result.Effect,
					"policy":  result.PolicyName,
					"message": result.Message,
				}
				denialData, _ := json.Marshal(denial)
				_ = conn.agent.WriteMessage(websocket.TextMessage, denialData)

				a.logger.Warn("action blocked by policy",
					"session_id", conn.sessionID,
					"action_type", actionCtx.Action.Type,
					"policy", result.PolicyName,
					"effect", result.Effect,
				)

				if result.Effect == "terminate" {
					_ = conn.agent.WriteControl(
						websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.ClosePolicyViolation, result.Message),
						time.Now().Add(time.Second),
					)
					return
				}
				continue // Don't forward denied actions.

			case "throttle":
				if result.Delay > 0 {
					time.Sleep(result.Delay)
				}
				// Fall through to forward after delay.

			case "approve":
				if a.approvalQueue != nil {
					approvalReq := &approval.Request{
						ID:         fmt.Sprintf("aq_%s_%d", conn.sessionID, time.Now().UnixNano()),
						SessionID:  conn.sessionID,
						PolicyName: result.PolicyName,
						ActionSummary: map[string]interface{}{
							"action_type": actionCtx.Action.Type,
							"action_name": actionCtx.Action.Name,
						},
						Timeout:       5 * time.Minute,
						TimeoutEffect: "deny",
					}
					ctx := context.Background()
					approved, err := a.approvalQueue.Submit(ctx, approvalReq)
					if err != nil || !approved {
						denial := map[string]interface{}{
							"type":    "governance_denied",
							"effect":  "approve_rejected",
							"policy":  result.PolicyName,
							"message": "action was not approved",
						}
						denialData, _ := json.Marshal(denial)
						_ = conn.agent.WriteMessage(websocket.TextMessage, denialData)
						continue
					}
				}
			}
		}

		// Forward allowed message to upstream.
		if err := conn.upstream.WriteMessage(msgType, data); err != nil {
			return
		}
	}
}

// proxyUpstreamToAgent forwards messages from the upstream gateway back to the agent.
func (a *GatewayAdapter) proxyUpstreamToAgent(conn *agentConn) {
	defer a.cleanupConn(conn)

	for {
		msgType, data, err := conn.upstream.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				a.logger.Debug("upstream connection closed", "session_id", conn.sessionID, "error", err)
			}
			return
		}

		if err := conn.agent.WriteMessage(msgType, data); err != nil {
			return
		}
	}
}

// cleanupConn removes a connection from the tracking map. It is safe to call
// from multiple goroutines — the actual cleanup runs exactly once.
func (a *GatewayAdapter) cleanupConn(conn *agentConn) {
	conn.cleanupOnce.Do(func() {
		a.mu.Lock()
		delete(a.conns, conn.sessionID)
		a.mu.Unlock()

		_ = conn.agent.Close()
		if conn.upstream != nil {
			_ = conn.upstream.Close()
		}
	})
}

// Ensure GatewayAdapter implements the adapter.Adapter interface.
var _ adapter.Adapter = (*GatewayAdapter)(nil)
