package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

// newUpgrader creates a WebSocket upgrader. When allowAllOrigins is false,
// only same-origin requests are accepted (Origin header must match Host).
func newUpgrader(allowAllOrigins bool) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if allowAllOrigins {
				return true
			}
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // non-browser clients don't send Origin
			}
			// Accept if Origin host matches the request Host header.
			host := r.Host
			return strings.Contains(origin, host)
		},
	}
}

// WebSocketHub manages WebSocket connections for live trace feed.
type WebSocketHub struct {
	mu       sync.RWMutex
	clients  map[*websocket.Conn]bool
	upgrader websocket.Upgrader
	logger   *slog.Logger
	done     chan struct{}
}

// NewWebSocketHub creates a new WebSocket hub.
func NewWebSocketHub(logger *slog.Logger, allowAllOrigins bool) *WebSocketHub {
	return &WebSocketHub{
		clients:  make(map[*websocket.Conn]bool),
		upgrader: newUpgrader(allowAllOrigins),
		logger:   logger,
		done:     make(chan struct{}),
	}
}

// Run starts the hub (handles cleanup).
func (h *WebSocketHub) Run() {
	<-h.done
}

// Close shuts down the hub and all connections.
func (h *WebSocketHub) Close() {
	close(h.done)
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		_ = conn.Close()
		delete(h.clients, conn)
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket.
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return
	}

	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	h.logger.Debug("websocket client connected", "remote", conn.RemoteAddr())

	// Read pump — keeps connection alive, handles client disconnect
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			_ = conn.Close()
			h.logger.Debug("websocket client disconnected", "remote", conn.RemoteAddr())
		}()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

// Broadcast sends a trace event to all connected WebSocket clients.
func (h *WebSocketHub) Broadcast(data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{
		"type": "trace",
		"data": data,
	})
	if err != nil {
		h.logger.Error("failed to marshal websocket message", "error", err)
		return
	}

	// Collect dead connections under RLock, then clean up under WLock.
	// This avoids spawning goroutines that try to acquire WLock while
	// RLock is held (which was a race condition).
	h.mu.RLock()
	var dead []*websocket.Conn
	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			h.logger.Debug("failed to write to websocket client", "error", err)
			dead = append(dead, conn)
		}
	}
	h.mu.RUnlock()

	if len(dead) > 0 {
		h.mu.Lock()
		for _, c := range dead {
			delete(h.clients, c)
			_ = c.Close()
		}
		h.mu.Unlock()
	}
}

// ClientCount returns the number of connected clients.
func (h *WebSocketHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
