package config

import (
	"time"
)

// Config is the top-level AgentWarden configuration.
type Config struct {
	Server       ServerConfig    `yaml:"server"`
	Storage      StorageConfig   `yaml:"storage"`
	Policies     []PolicyConfig  `yaml:"policies"`
	Detection    DetectionConfig `yaml:"detection"`
	Evolution    EvolutionConfig `yaml:"evolution"`
	Alerts       AlertsConfig    `yaml:"alerts"`
	AgentsDir    string          `yaml:"agents_dir"`
	PoliciesDir  string          `yaml:"policies_dir"`
	PlaybooksDir string          `yaml:"playbooks_dir"`
}

type ServerConfig struct {
	Port      int    `yaml:"port"`
	GRPCPort  int    `yaml:"grpc_port"`
	Dashboard bool   `yaml:"dashboard"`
	LogLevel  string `yaml:"log_level"`
	CORS      bool   `yaml:"cors"`
	FailMode  string `yaml:"fail_mode"` // "closed" = deny on error, "open" = allow on error
}

type StorageConfig struct {
	Driver     string          `yaml:"driver"`
	Path       string          `yaml:"path"`
	Connection string          `yaml:"connection"`
	Retention  time.Duration   `yaml:"retention"`
	Redaction  []RedactionRule `yaml:"redaction"`
}

type RedactionRule struct {
	Pattern     string   `yaml:"pattern"`
	Replacement string   `yaml:"replacement"`
	Fields      []string `yaml:"fields"`
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
	Context       string        `yaml:"context"` // path to POLICY.md file for ai-judge policies
	Approvers     []string      `yaml:"approvers"`
	Timeout       time.Duration `yaml:"timeout"`
	TimeoutEffect string        `yaml:"timeout_effect"`
}

type DetectionConfig struct {
	Loop        LoopDetectionConfig   `yaml:"loop"`
	CostAnomaly CostAnomalyConfig     `yaml:"cost_anomaly"`
	Spiral      SpiralDetectionConfig `yaml:"spiral"`
}

type LoopDetectionConfig struct {
	Enabled        bool          `yaml:"enabled"`
	Threshold      int           `yaml:"threshold"`
	Window         time.Duration `yaml:"window"`
	Action         string        `yaml:"action"` // pause, alert, terminate, playbook
	FallbackAction string        `yaml:"fallback_action"`
	PlaybookModel  string        `yaml:"playbook_model"`
}

type CostAnomalyConfig struct {
	Enabled        bool    `yaml:"enabled"`
	Multiplier     float64 `yaml:"multiplier"`
	Action         string  `yaml:"action"`
	FallbackAction string  `yaml:"fallback_action"`
	PlaybookModel  string  `yaml:"playbook_model"`
}

type SpiralDetectionConfig struct {
	Enabled             bool    `yaml:"enabled"`
	SimilarityThreshold float64 `yaml:"similarity_threshold"`
	Window              int     `yaml:"window"`
	Action              string  `yaml:"action"`
	FallbackAction      string  `yaml:"fallback_action"`
	PlaybookModel       string  `yaml:"playbook_model"`
}

type EvolutionConfig struct {
	Enabled     bool               `yaml:"enabled"`
	Model       string             `yaml:"model"`
	Scoring     ScoringConfig      `yaml:"scoring"`
	Constraints []string           `yaml:"constraints"`
	Shadow      ShadowConfig       `yaml:"shadow"`
	Rollback    RollbackConfig     `yaml:"rollback"`
	Triggers    []EvolutionTrigger `yaml:"triggers"`
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
			GRPCPort:  6778,
			Dashboard: true,
			LogLevel:  "info",
			CORS:      false,
			FailMode:  "closed",
		},
		AgentsDir:    "./agents",
		PoliciesDir:  "./policies",
		PlaybooksDir: "./playbooks",
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
		Evolution: EvolutionConfig{
			Model: "claude-sonnet-4",
		},
	}
}
