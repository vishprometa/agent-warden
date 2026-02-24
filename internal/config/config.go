package config

import (
	"time"
)

// Config is the top-level AgentWarden configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Upstream  UpstreamConfig  `yaml:"upstream"`
	Storage   StorageConfig   `yaml:"storage"`
	Policies  []PolicyConfig  `yaml:"policies"`
	Detection DetectionConfig `yaml:"detection"`
	Evolution EvolutionConfig `yaml:"evolution"`
	Alerts    AlertsConfig    `yaml:"alerts"`
}

type ServerConfig struct {
	Port      int    `yaml:"port"`
	Dashboard bool   `yaml:"dashboard"`
	LogLevel  string `yaml:"log_level"`
	CORS      bool   `yaml:"cors"`
}

type UpstreamConfig struct {
	Default         string            `yaml:"default"`
	Providers       map[string]string `yaml:"providers"`
	PassthroughAuth bool              `yaml:"passthrough_auth"`
	Timeout         time.Duration     `yaml:"timeout"`
	Retries         int               `yaml:"retries"`
}

type StorageConfig struct {
	Driver     string        `yaml:"driver"`
	Path       string        `yaml:"path"`
	Connection string        `yaml:"connection"`
	Retention  time.Duration `yaml:"retention"`
}

type PolicyConfig struct {
	Name          string        `yaml:"name"`
	Condition     string        `yaml:"condition"`
	Effect        string        `yaml:"effect"` // allow, deny, terminate, throttle, approve
	Message       string        `yaml:"message"`
	Type          string        `yaml:"type"`  // "" (deterministic/CEL) or "ai-judge"
	Delay         time.Duration `yaml:"delay"` // for throttle
	Prompt        string        `yaml:"prompt"`
	Model         string        `yaml:"model"`
	Approvers     []string      `yaml:"approvers"`
	Timeout       time.Duration `yaml:"timeout"`
	TimeoutEffect string        `yaml:"timeout_effect"`
}

type DetectionConfig struct {
	Loop        LoopDetectionConfig    `yaml:"loop"`
	CostAnomaly CostAnomalyConfig     `yaml:"cost_anomaly"`
	Spiral      SpiralDetectionConfig  `yaml:"spiral"`
}

type LoopDetectionConfig struct {
	Enabled   bool          `yaml:"enabled"`
	Threshold int           `yaml:"threshold"`
	Window    time.Duration `yaml:"window"`
	Action    string        `yaml:"action"` // pause, alert, terminate
}

type CostAnomalyConfig struct {
	Enabled    bool    `yaml:"enabled"`
	Multiplier float64 `yaml:"multiplier"`
	Action     string  `yaml:"action"`
}

type SpiralDetectionConfig struct {
	Enabled             bool    `yaml:"enabled"`
	SimilarityThreshold float64 `yaml:"similarity_threshold"`
	Window              int     `yaml:"window"`
	Action              string  `yaml:"action"`
}

type EvolutionConfig struct {
	Enabled     bool                `yaml:"enabled"`
	Scoring     ScoringConfig       `yaml:"scoring"`
	Constraints []string            `yaml:"constraints"`
	Shadow      ShadowConfig        `yaml:"shadow"`
	Rollback    RollbackConfig      `yaml:"rollback"`
	Triggers    []EvolutionTrigger  `yaml:"triggers"`
}

type ScoringConfig struct {
	Metrics []string      `yaml:"metrics"`
	Window  time.Duration `yaml:"window"`
}

type ShadowConfig struct {
	Required         bool    `yaml:"required"`
	MinRuns          int     `yaml:"min_runs"`
	SuccessThreshold float64 `yaml:"success_threshold"`
}

type RollbackConfig struct {
	Auto    bool   `yaml:"auto"`
	Trigger string `yaml:"trigger"`
}

type EvolutionTrigger struct {
	Type      string        `yaml:"type"` // scheduled, metric_threshold
	Cron      string        `yaml:"cron"`
	Condition string        `yaml:"condition"`
	Cooldown  time.Duration `yaml:"cooldown"`
}

type AlertsConfig struct {
	Slack   SlackAlertConfig   `yaml:"slack"`
	Webhook WebhookAlertConfig `yaml:"webhook"`
}

type SlackAlertConfig struct {
	WebhookURL string `yaml:"webhook_url"`
	Channel    string `yaml:"channel"`
}

type WebhookAlertConfig struct {
	URL    string `yaml:"url"`
	Secret string `yaml:"secret"`
}

// DefaultConfig returns a config with sensible defaults for zero-config startup.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:      6777,
			Dashboard: true,
			LogLevel:  "info",
			CORS:      false,
		},
		Upstream: UpstreamConfig{
			Default: "https://api.openai.com/v1",
			Providers: map[string]string{
				"openai":    "https://api.openai.com/v1",
				"anthropic": "https://api.anthropic.com/v1",
				"gemini":    "https://generativelanguage.googleapis.com/v1beta",
			},
			PassthroughAuth: true,
			Timeout:         30 * time.Second,
			Retries:         2,
		},
		Storage: StorageConfig{
			Driver:    "sqlite",
			Path:      "./agentwarden.db",
			Retention: 30 * 24 * time.Hour,
		},
		Detection: DetectionConfig{
			Loop: LoopDetectionConfig{
				Enabled:   true,
				Threshold: 5,
				Window:    60 * time.Second,
				Action:    "pause",
			},
			CostAnomaly: CostAnomalyConfig{
				Enabled:    true,
				Multiplier: 10,
				Action:     "alert",
			},
			Spiral: SpiralDetectionConfig{
				Enabled:             true,
				SimilarityThreshold: 0.9,
				Window:              5,
				Action:              "alert",
			},
		},
	}
}
