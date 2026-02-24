package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_LoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "agentwarden.yaml")

	yamlContent := `
server:
  port: 8080
  grpc_port: 6778
  dashboard: true
  log_level: debug
  cors: true
  fail_mode: closed

agents_dir: ./agents
policies_dir: ./policies
playbooks_dir: ./playbooks

storage:
  driver: sqlite
  path: ./test.db
  retention: 168h

policies:
  - name: budget-limit
    condition: "session.cost > 10.0"
    effect: terminate
    message: "Over budget"

detection:
  loop:
    enabled: true
    threshold: 10
    window: 120s
    action: pause
  cost_anomaly:
    enabled: false
    multiplier: 5
    action: alert
  spiral:
    enabled: true
    similarity_threshold: 0.85
    window: 4
    action: alert
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader := NewLoader()
	if err := loader.Load(configPath); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg := loader.Get()

	// Server
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.LogLevel != "debug" {
		t.Errorf("Server.LogLevel = %q, want \"debug\"", cfg.Server.LogLevel)
	}
	if !cfg.Server.CORS {
		t.Error("Server.CORS = false, want true")
	}

	// v2 config fields
	if cfg.Server.GRPCPort != 6778 {
		t.Errorf("Server.GRPCPort = %d, want 6778", cfg.Server.GRPCPort)
	}
	if cfg.Server.FailMode != "closed" {
		t.Errorf("Server.FailMode = %q, want \"closed\"", cfg.Server.FailMode)
	}
	if cfg.AgentsDir != "./agents" {
		t.Errorf("AgentsDir = %q, want \"./agents\"", cfg.AgentsDir)
	}
	if cfg.PoliciesDir != "./policies" {
		t.Errorf("PoliciesDir = %q, want \"./policies\"", cfg.PoliciesDir)
	}
	if cfg.PlaybooksDir != "./playbooks" {
		t.Errorf("PlaybooksDir = %q, want \"./playbooks\"", cfg.PlaybooksDir)
	}

	// Policies
	if len(cfg.Policies) != 1 {
		t.Fatalf("Policies length = %d, want 1", len(cfg.Policies))
	}
	if cfg.Policies[0].Name != "budget-limit" {
		t.Errorf("Policies[0].Name = %q, want \"budget-limit\"", cfg.Policies[0].Name)
	}
	if cfg.Policies[0].Effect != "terminate" {
		t.Errorf("Policies[0].Effect = %q, want \"terminate\"", cfg.Policies[0].Effect)
	}

	// Detection
	if cfg.Detection.Loop.Threshold != 10 {
		t.Errorf("Detection.Loop.Threshold = %d, want 10", cfg.Detection.Loop.Threshold)
	}
	if !cfg.Detection.Spiral.Enabled {
		t.Error("Detection.Spiral.Enabled = false, want true")
	}
	if cfg.Detection.Spiral.SimilarityThreshold != 0.85 {
		t.Errorf("Detection.Spiral.SimilarityThreshold = %f, want 0.85",
			cfg.Detection.Spiral.SimilarityThreshold)
	}
}

func TestLoader_DefaultConfig(t *testing.T) {
	loader := NewLoader()
	cfg := loader.Get()

	// Verify defaults
	if cfg.Server.Port != 6777 {
		t.Errorf("default Server.Port = %d, want 6777", cfg.Server.Port)
	}
	if cfg.Server.GRPCPort != 6778 {
		t.Errorf("default Server.GRPCPort = %d, want 6778", cfg.Server.GRPCPort)
	}
	if cfg.Server.FailMode != "closed" {
		t.Errorf("default Server.FailMode = %q, want \"closed\"", cfg.Server.FailMode)
	}
	if cfg.AgentsDir != "./agents" {
		t.Errorf("default AgentsDir = %q, want \"./agents\"", cfg.AgentsDir)
	}
	if cfg.Storage.Driver != "sqlite" {
		t.Errorf("default Storage.Driver = %q, want \"sqlite\"", cfg.Storage.Driver)
	}
	if cfg.Detection.Loop.Threshold != 5 {
		t.Errorf("default Detection.Loop.Threshold = %d, want 5", cfg.Detection.Loop.Threshold)
	}
	if !cfg.Detection.Loop.Enabled {
		t.Error("default Detection.Loop.Enabled = false, want true")
	}
}

func TestLoader_LoadNonExistentFile(t *testing.T) {
	loader := NewLoader()
	err := loader.Load("/nonexistent/path/to/config.yaml")
	if err == nil {
		t.Error("Load() with nonexistent file should return error")
	}
}

func TestLoader_LoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "bad.yaml")

	if err := os.WriteFile(configPath, []byte(`{{{invalid yaml`), 0644); err != nil {
		t.Fatalf("failed to write bad config: %v", err)
	}

	loader := NewLoader()
	err := loader.Load(configPath)
	if err == nil {
		t.Error("Load() with invalid YAML should return error")
	}
}

func TestLoader_FilePath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "agentwarden.yaml")
	if err := os.WriteFile(configPath, []byte("server:\n  port: 9999\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loader := NewLoader()
	if loader.FilePath() != "" {
		t.Errorf("FilePath() before Load() = %q, want empty", loader.FilePath())
	}

	if err := loader.Load(configPath); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loader.FilePath() != configPath {
		t.Errorf("FilePath() = %q, want %q", loader.FilePath(), configPath)
	}
}

func TestLoader_Reload(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "agentwarden.yaml")

	// Write initial config
	if err := os.WriteFile(configPath, []byte("server:\n  port: 8080\n"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loader := NewLoader()
	if err := loader.Load(configPath); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loader.Get().Server.Port != 8080 {
		t.Errorf("initial port = %d, want 8080", loader.Get().Server.Port)
	}

	// Overwrite with new config
	if err := os.WriteFile(configPath, []byte("server:\n  port: 9999\n"), 0644); err != nil {
		t.Fatalf("failed to overwrite config: %v", err)
	}

	if err := loader.Reload(); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}

	if loader.Get().Server.Port != 9999 {
		t.Errorf("reloaded port = %d, want 9999", loader.Get().Server.Port)
	}
}

func TestLoader_ReloadWithoutLoad(t *testing.T) {
	loader := NewLoader()
	err := loader.Reload()
	if err == nil {
		t.Error("Reload() without prior Load() should return error")
	}
}

func TestSubstituteEnvVars(t *testing.T) {
	// Set test environment variable
	os.Setenv("TEST_AW_PORT", "9999")
	os.Setenv("TEST_AW_SECRET", "my-secret")
	defer os.Unsetenv("TEST_AW_PORT")
	defer os.Unsetenv("TEST_AW_SECRET")

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple substitution",
			input: "port: ${TEST_AW_PORT}",
			want:  "port: 9999",
		},
		{
			name:  "multiple substitutions",
			input: "port: ${TEST_AW_PORT}\nsecret: ${TEST_AW_SECRET}",
			want:  "port: 9999\nsecret: my-secret",
		},
		{
			name:  "undefined variable",
			input: "value: ${UNDEFINED_TEST_VAR_XYZ}",
			want:  "value: ",
		},
		{
			name:  "default value syntax",
			input: "value: ${UNDEFINED_TEST_VAR_XYZ:-default-val}",
			want:  "value: default-val",
		},
		{
			name:  "default value not used when env var set",
			input: "port: ${TEST_AW_PORT:-1234}",
			want:  "port: 9999",
		},
		{
			name:  "no env vars",
			input: "port: 8080",
			want:  "port: 8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := substituteEnvVars(tt.input)
			if got != tt.want {
				t.Errorf("substituteEnvVars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSubstituteEnvVars_InConfigLoad(t *testing.T) {
	os.Setenv("TEST_AW_CFG_PORT", "7777")
	defer os.Unsetenv("TEST_AW_CFG_PORT")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "agentwarden.yaml")

	yamlContent := `
server:
  port: ${TEST_AW_CFG_PORT}
  log_level: info
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	loader := NewLoader()
	if err := loader.Load(configPath); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	cfg := loader.Get()
	if cfg.Server.Port != 7777 {
		t.Errorf("Server.Port with env var = %d, want 7777", cfg.Server.Port)
	}
}

func TestGenerateDefault(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "agentwarden.yaml")

	if err := GenerateDefault(configPath); err != nil {
		t.Fatalf("GenerateDefault() error: %v", err)
	}

	// File should exist
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read generated config: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("generated config is empty")
	}

	// Verify it's valid YAML by loading it
	loader := NewLoader()
	if err := loader.Load(configPath); err != nil {
		t.Fatalf("generated config is not valid YAML: %v", err)
	}

	cfg := loader.Get()
	if cfg.Server.Port != 6777 {
		t.Errorf("generated config port = %d, want 6777", cfg.Server.Port)
	}
}
