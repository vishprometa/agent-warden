package openclaw

// Config holds OpenClaw adapter configuration.
type Config struct {
	// Enabled controls whether the OpenClaw adapter is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Mode determines how AgentWarden integrates with OpenClaw:
	//   "inline"     — WebSocket reverse proxy (transparent, no OpenClaw changes)
	//   "sidecar"    — Receive events via webhook (requires OpenClaw event hook config)
	//   "event-hook" — Use OpenClaw's native event hook system
	Mode string `yaml:"mode" json:"mode"`

	// GatewayURL is the upstream OpenClaw gateway WebSocket URL.
	// Example: ws://localhost:4000
	GatewayURL string `yaml:"gateway_url" json:"gateway_url"`

	// AuthToken is the authentication token for the OpenClaw gateway.
	AuthToken string `yaml:"auth_token" json:"auth_token"`

	// ProxyPath is the HTTP path where the WebSocket proxy listens.
	// Agents connect to AgentWarden at this path instead of the real gateway.
	// Default: /gateway
	ProxyPath string `yaml:"proxy_path" json:"proxy_path"`

	// Intercept controls which event types are intercepted and evaluated.
	// If empty, all events are intercepted.
	Intercept []string `yaml:"intercept" json:"intercept"`
}

// DefaultConfig returns sensible defaults for the OpenClaw adapter.
func DefaultConfig() Config {
	return Config{
		Mode:      "inline",
		ProxyPath: "/gateway",
		Intercept: []string{
			"tool_calls",
			"skill_installs",
			"message_sends",
			"agent_spawns",
			"financial_transfers",
			"config_changes",
		},
	}
}
