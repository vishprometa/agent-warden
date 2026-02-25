package detection

import (
	"log/slog"
	"sync"

	"github.com/agentwarden/agentwarden/internal/config"
)

// Event represents a detected anomaly.
type Event struct {
	Type      string // loop, cost_anomaly, spiral, drift
	SessionID string
	AgentID   string
	Action    string // recommended action: pause, alert, terminate
	Message   string
	Details   map[string]interface{}
}

// EventHandler is called when an anomaly is detected.
type EventHandler func(event Event)

// Engine orchestrates all detection subsystems.
type Engine struct {
	mu       sync.RWMutex
	config   config.DetectionConfig
	loop     *LoopDetector
	anomaly  *CostAnomalyDetector
	spiral   *SpiralDetector
	velocity *VelocityDetector
	handler  EventHandler
	logger   *slog.Logger
}

// NewEngine creates a new detection engine.
func NewEngine(cfg config.DetectionConfig, handler EventHandler, logger *slog.Logger) *Engine {
	return &Engine{
		config:   cfg,
		loop:     NewLoopDetector(cfg.Loop),
		anomaly:  NewCostAnomalyDetector(cfg.CostAnomaly),
		spiral:   NewSpiralDetector(cfg.Spiral),
		velocity: NewVelocityDetector(cfg.Velocity),
		handler:  handler,
		logger:   logger,
	}
}

// ActionEvent represents an action to be analyzed by detectors.
type ActionEvent struct {
	SessionID  string
	AgentID    string
	ActionType string
	ActionName string
	Signature  string // hash of action type + name + key params
	CostUSD    float64
	Content    string // LLM output content for spiral detection
}

// Analyze runs all enabled detectors against an action event.
func (e *Engine) Analyze(event ActionEvent) {
	e.mu.RLock()
	cfg := e.config
	e.mu.RUnlock()

	// Loop detection
	if cfg.Loop.Enabled {
		if detected := e.loop.Check(event); detected != nil {
			e.logger.Warn("loop detected",
				"session_id", event.SessionID,
				"signature", event.Signature,
				"count", detected.Details["count"],
			)
			if e.handler != nil {
				e.handler(*detected)
			}
		}
	}

	// Cost anomaly detection
	if cfg.CostAnomaly.Enabled {
		if detected := e.anomaly.Check(event); detected != nil {
			e.logger.Warn("cost anomaly detected",
				"session_id", event.SessionID,
				"velocity", detected.Details["velocity"],
			)
			if e.handler != nil {
				e.handler(*detected)
			}
		}
	}

	// Spiral detection
	if cfg.Spiral.Enabled && event.Content != "" {
		if detected := e.spiral.Check(event); detected != nil {
			e.logger.Warn("conversation spiral detected",
				"session_id", event.SessionID,
				"similarity", detected.Details["similarity"],
			)
			if e.handler != nil {
				e.handler(*detected)
			}
		}
	}

	// Velocity detection (runaway agent breaker)
	if cfg.Velocity.Enabled {
		if detected := e.velocity.Check(event); detected != nil {
			e.logger.Error("ACTION VELOCITY BREACH",
				"session_id", event.SessionID,
				"velocity", detected.Details["velocity"],
			)
			if e.handler != nil {
				e.handler(*detected)
			}
		}
	}
}

// ResetSession clears all detector state for a session.
func (e *Engine) ResetSession(sessionID string) {
	e.loop.ResetSession(sessionID)
	e.anomaly.ResetSession(sessionID)
	e.spiral.ResetSession(sessionID)
	e.velocity.ResetSession(sessionID)
}

// UpdateConfig updates the detection configuration.
func (e *Engine) UpdateConfig(cfg config.DetectionConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = cfg
	e.loop = NewLoopDetector(cfg.Loop)
	e.anomaly = NewCostAnomalyDetector(cfg.CostAnomaly)
	e.spiral = NewSpiralDetector(cfg.Spiral)
	e.velocity = NewVelocityDetector(cfg.Velocity)
}
