package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for dev; restrict in production
	},
}

// WebSocketHub manages WebSocket connections for live trace feed.
type WebSocketHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
	logger  *slog.Logger
	done    chan struct{}
}

// NewWebSocketHub creates a new WebSocket hub.
func NewWebSocketHub(logger *slog.Logger) *WebSocketHub {
	return &WebSocketHub{
		clients: make(map[*websocket.Conn]bool),
		logger:  logger,
		done:    make(chan struct{}),
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
		conn.Close()
		delete(h.clients, conn)
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket.
func (h *WebSocketHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
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
			conn.Close()
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

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			h.logger.Debug("failed to write to websocket client", "error", err)
			go func(c *websocket.Conn) {
				h.mu.Lock()
				delete(h.clients, c)
				h.mu.Unlock()
				c.Close()
			}(conn)
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *WebSocketHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
