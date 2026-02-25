package mdloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidationResult_OK(t *testing.T) {
	tests := []struct {
		name   string
		result ValidationResult
		want   bool
	}{
		{
			name:   "no errors or warnings",
			result: ValidationResult{},
			want:   true,
		},
		{
			name:   "warnings only",
			result: ValidationResult{Warnings: []string{"warning 1", "warning 2"}},
			want:   true,
		},
		{
			name:   "errors only",
			result: ValidationResult{Errors: []string{"error 1"}},
			want:   false,
		},
		{
			name: "both errors and warnings",
			result: ValidationResult{
				Errors:   []string{"error 1"},
				Warnings: []string{"warning 1"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.OK(); got != tt.want {
				t.Errorf("ValidationResult.OK() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidationResult_Summary(t *testing.T) {
	tests := []struct {
		name     string
		result   ValidationResult
		contains []string
	}{
		{
			name:   "no errors or warnings",
			result: ValidationResult{},
			contains: []string{
				"Validation passed",
				"0 warnings",
			},
		},
		{
			name:   "warnings only",
			result: ValidationResult{Warnings: []string{"missing EVOLVE.md", "no agents found"}},
			contains: []string{
				"Validation passed",
				"2 warnings",
				"WARN:  missing EVOLVE.md",
				"WARN:  no agents found",
			},
		},
		{
			name:   "errors only",
			result: ValidationResult{Errors: []string{"missing AGENT.md"}},
			contains: []string{
				"Validation failed",
				"1 errors",
				"0 warnings",
				"ERROR: missing AGENT.md",
			},
		},
		{
			name: "both errors and warnings",
			result: ValidationResult{
				Errors:   []string{"missing AGENT.md", "missing PROMPT.md"},
				Warnings: []string{"missing EVOLVE.md"},
			},
			contains: []string{
				"Validation failed",
				"2 errors",
				"1 warnings",
				"ERROR: missing AGENT.md",
				"ERROR: missing PROMPT.md",
				"WARN:  missing EVOLVE.md",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.result.Summary()
			for _, expected := range tt.contains {
				if !strings.Contains(summary, expected) {
					t.Errorf("Summary() missing expected text %q\nGot:\n%s", expected, summary)
				}
			}
		})
	}
}

func TestValidateAll_ValidConfig(t *testing.T) {
	// Setup: create a valid directory structure
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	policiesDir := filepath.Join(tmpDir, "policies")
	playbooksDir := filepath.Join(tmpDir, "playbooks")

	// Create a valid agent
	agentDir := filepath.Join(agentsDir, "test-agent")
	versionsDir := filepath.Join(agentDir, "versions", "v1")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("# Agent"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "EVOLVE.md"), []byte("# Evolve"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionsDir, "PROMPT.md"), []byte("# Prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a valid AI-judge policy
	policyDir := filepath.Join(policiesDir, "safety")
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(policyDir, "POLICY.md"), []byte("# Policy"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a valid playbook
	if err := os.MkdirAll(playbooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(playbooksDir, "LOOP.md"), []byte("# Loop"), 0644); err != nil {
		t.Fatal(err)
	}

	policies := []PolicyRef{
		{Name: "safety-check", Type: "ai-judge", Context: "safety"},
		{Name: "budget-check", Type: "cel", Context: ""},
	}
	detections := []DetectionRef{
		{Name: "loop", Action: "playbook"},
		{Name: "cost_anomaly", Action: "alert"},
	}

	result := ValidateAll(agentsDir, policiesDir, playbooksDir, policies, detections)

	if !result.OK() {
		t.Errorf("ValidateAll() should pass for valid config, got:\n%s", result.Summary())
	}
	if len(result.Errors) != 0 {
		t.Errorf("ValidateAll() should have no errors, got: %v", result.Errors)
	}
	// One warning expected: no EVOLVE.md warning is no longer present since we created it
	// Actually, no warnings expected for a fully valid config
}

func TestValidateAll_MissingAgentMD(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	// Create agent directory but no AGENT.md
	agentDir := filepath.Join(agentsDir, "test-agent")
	versionsDir := filepath.Join(agentDir, "versions", "v1")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionsDir, "PROMPT.md"), []byte("# Prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ValidateAll(agentsDir, "", "", nil, nil)

	if result.OK() {
		t.Error("ValidateAll() should fail when AGENT.md is missing")
	}
	if len(result.Errors) == 0 {
		t.Error("ValidateAll() should have errors for missing AGENT.md")
	}
	if !strings.Contains(result.Summary(), "missing AGENT.md") {
		t.Errorf("Summary should mention missing AGENT.md, got:\n%s", result.Summary())
	}
}

func TestValidateAll_MissingEvolveMD(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	// Create agent directory with AGENT.md and PROMPT.md but no EVOLVE.md
	agentDir := filepath.Join(agentsDir, "test-agent")
	versionsDir := filepath.Join(agentDir, "versions", "v1")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("# Agent"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionsDir, "PROMPT.md"), []byte("# Prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ValidateAll(agentsDir, "", "", nil, nil)

	if !result.OK() {
		t.Errorf("ValidateAll() should pass when only EVOLVE.md is missing (warning only), got:\n%s", result.Summary())
	}
	if len(result.Warnings) == 0 {
		t.Error("ValidateAll() should have warning for missing EVOLVE.md")
	}
	if !strings.Contains(result.Summary(), "no EVOLVE.md found") {
		t.Errorf("Summary should mention missing EVOLVE.md as warning, got:\n%s", result.Summary())
	}
}

func TestValidateAll_MissingVersionsDir(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	// Create agent directory with AGENT.md but no versions/ directory
	agentDir := filepath.Join(agentsDir, "test-agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("# Agent"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ValidateAll(agentsDir, "", "", nil, nil)

	if result.OK() {
		t.Error("ValidateAll() should fail when versions/ directory is missing")
	}
	if len(result.Errors) == 0 {
		t.Error("ValidateAll() should have errors for missing versions/ directory")
	}
	if !strings.Contains(result.Summary(), "missing versions/ directory") {
		t.Errorf("Summary should mention missing versions/ directory, got:\n%s", result.Summary())
	}
}

func TestValidateAll_MissingPromptMD(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	// Create agent directory with versions/ but no PROMPT.md
	agentDir := filepath.Join(agentsDir, "test-agent")
	versionsDir := filepath.Join(agentDir, "versions", "v1")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("# Agent"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ValidateAll(agentsDir, "", "", nil, nil)

	if result.OK() {
		t.Error("ValidateAll() should fail when no version contains PROMPT.md")
	}
	if len(result.Errors) == 0 {
		t.Error("ValidateAll() should have errors for missing PROMPT.md")
	}
	if !strings.Contains(result.Summary(), "no version directory contains a PROMPT.md") {
		t.Errorf("Summary should mention missing PROMPT.md, got:\n%s", result.Summary())
	}
}

func TestValidateAll_NoAgentsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "nonexistent-agents")

	result := ValidateAll(agentsDir, "", "", nil, nil)

	if !result.OK() {
		t.Errorf("ValidateAll() should pass with warning when agents directory doesn't exist, got:\n%s", result.Summary())
	}
	if len(result.Warnings) == 0 {
		t.Error("ValidateAll() should have warning for missing agents directory")
	}
	if !strings.Contains(result.Summary(), "agents directory does not exist") {
		t.Errorf("Summary should mention agents directory doesn't exist, got:\n%s", result.Summary())
	}
}

func TestValidateAll_NoAgents(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	result := ValidateAll(agentsDir, "", "", nil, nil)

	if !result.OK() {
		t.Errorf("ValidateAll() should pass with warning when no agents exist, got:\n%s", result.Summary())
	}
	if len(result.Warnings) == 0 {
		t.Error("ValidateAll() should have warning for no agents found")
	}
	if !strings.Contains(result.Summary(), "no agent directories found") {
		t.Errorf("Summary should mention no agents found, got:\n%s", result.Summary())
	}
}

func TestValidateAll_AIJudgePolicyMissingContext(t *testing.T) {
	tmpDir := t.TempDir()

	policies := []PolicyRef{
		{Name: "safety-check", Type: "ai-judge", Context: ""},
	}

	result := ValidateAll("", tmpDir, "", policies, nil)

	if result.OK() {
		t.Error("ValidateAll() should fail when AI-judge policy has no context")
	}
	if len(result.Errors) == 0 {
		t.Error("ValidateAll() should have errors for AI-judge policy missing context")
	}
	if !strings.Contains(result.Summary(), "has no context path configured") {
		t.Errorf("Summary should mention missing context path, got:\n%s", result.Summary())
	}
}

func TestValidateAll_AIJudgePolicyMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	policiesDir := filepath.Join(tmpDir, "policies")
	if err := os.MkdirAll(policiesDir, 0755); err != nil {
		t.Fatal(err)
	}

	policies := []PolicyRef{
		{Name: "safety-check", Type: "ai-judge", Context: "safety"},
	}

	result := ValidateAll("", policiesDir, "", policies, nil)

	if result.OK() {
		t.Error("ValidateAll() should fail when AI-judge POLICY.md is missing")
	}
	if len(result.Errors) == 0 {
		t.Error("ValidateAll() should have errors for missing POLICY.md")
	}
	if !strings.Contains(result.Summary(), "referenced POLICY.md not found") {
		t.Errorf("Summary should mention missing POLICY.md, got:\n%s", result.Summary())
	}
}

func TestValidateAll_CELPolicyNoValidation(t *testing.T) {
	tmpDir := t.TempDir()

	policies := []PolicyRef{
		{Name: "budget-check", Type: "cel", Context: ""},
		{Name: "rate-limit", Type: "", Context: ""},
	}

	result := ValidateAll("", tmpDir, "", policies, nil)

	// CEL and deterministic policies don't need file validation
	if !result.OK() {
		t.Errorf("ValidateAll() should pass for non-AI-judge policies, got:\n%s", result.Summary())
	}
}

func TestValidateAll_PlaybookActionMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	playbooksDir := filepath.Join(tmpDir, "playbooks")
	if err := os.MkdirAll(playbooksDir, 0755); err != nil {
		t.Fatal(err)
	}

	detections := []DetectionRef{
		{Name: "loop", Action: "playbook"},
	}

	result := ValidateAll("", "", playbooksDir, nil, detections)

	if result.OK() {
		t.Error("ValidateAll() should fail when playbook MD file is missing")
	}
	if len(result.Errors) == 0 {
		t.Error("ValidateAll() should have errors for missing playbook file")
	}
	if !strings.Contains(result.Summary(), "LOOP.md not found") {
		t.Errorf("Summary should mention missing LOOP.md, got:\n%s", result.Summary())
	}
}

func TestValidateAll_PlaybookActionExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	playbooksDir := filepath.Join(tmpDir, "playbooks")
	if err := os.MkdirAll(playbooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(playbooksDir, "LOOP.md"), []byte("# Loop"), 0644); err != nil {
		t.Fatal(err)
	}

	detections := []DetectionRef{
		{Name: "loop", Action: "playbook"},
	}

	result := ValidateAll("", "", playbooksDir, nil, detections)

	if !result.OK() {
		t.Errorf("ValidateAll() should pass when playbook MD file exists, got:\n%s", result.Summary())
	}
}

func TestValidateAll_NonPlaybookActionNoValidation(t *testing.T) {
	tmpDir := t.TempDir()

	detections := []DetectionRef{
		{Name: "cost_anomaly", Action: "alert"},
		{Name: "drift", Action: "pause"},
		{Name: "spiral", Action: "terminate"},
	}

	result := ValidateAll("", "", tmpDir, nil, detections)

	// Non-playbook actions don't need file validation
	if !result.OK() {
		t.Errorf("ValidateAll() should pass for non-playbook actions, got:\n%s", result.Summary())
	}
}

func TestValidateAll_MultipleAgents(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	// Create agent 1 (valid)
	agent1Dir := filepath.Join(agentsDir, "agent1")
	versions1Dir := filepath.Join(agent1Dir, "versions", "v1")
	if err := os.MkdirAll(versions1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent1Dir, "AGENT.md"), []byte("# Agent 1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versions1Dir, "PROMPT.md"), []byte("# Prompt 1"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create agent 2 (missing AGENT.md)
	agent2Dir := filepath.Join(agentsDir, "agent2")
	versions2Dir := filepath.Join(agent2Dir, "versions", "v1")
	if err := os.MkdirAll(versions2Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versions2Dir, "PROMPT.md"), []byte("# Prompt 2"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ValidateAll(agentsDir, "", "", nil, nil)

	if result.OK() {
		t.Error("ValidateAll() should fail when one agent is invalid")
	}
	if !strings.Contains(result.Summary(), `agent "agent2"`) {
		t.Errorf("Summary should mention agent2 error, got:\n%s", result.Summary())
	}
	// Should have 2 warnings (both agents missing EVOLVE.md)
	if len(result.Warnings) != 2 {
		t.Errorf("Expected 2 warnings for missing EVOLVE.md, got %d", len(result.Warnings))
	}
}

func TestValidateAll_MultipleVersions(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")

	// Create agent with multiple versions
	agentDir := filepath.Join(agentsDir, "test-agent")
	v1Dir := filepath.Join(agentDir, "versions", "v1")
	v2Dir := filepath.Join(agentDir, "versions", "v2")
	if err := os.MkdirAll(v1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(v2Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("# Agent"), 0644); err != nil {
		t.Fatal(err)
	}
	// Only v2 has PROMPT.md
	if err := os.WriteFile(filepath.Join(v2Dir, "PROMPT.md"), []byte("# Prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	result := ValidateAll(agentsDir, "", "", nil, nil)

	// Should pass because at least one version has PROMPT.md
	if !result.OK() {
		t.Errorf("ValidateAll() should pass when at least one version has PROMPT.md, got:\n%s", result.Summary())
	}
}

func TestValidateAll_ComplexScenario(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	policiesDir := filepath.Join(tmpDir, "policies")
	playbooksDir := filepath.Join(tmpDir, "playbooks")

	// Create valid agent
	agentDir := filepath.Join(agentsDir, "valid-agent")
	versionsDir := filepath.Join(agentDir, "versions", "v1")
	if err := os.MkdirAll(versionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "AGENT.md"), []byte("# Agent"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionsDir, "PROMPT.md"), []byte("# Prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create invalid agent (missing versions/)
	invalidAgentDir := filepath.Join(agentsDir, "invalid-agent")
	if err := os.MkdirAll(invalidAgentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidAgentDir, "AGENT.md"), []byte("# Agent"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create valid AI-judge policy
	policyDir := filepath.Join(policiesDir, "safety")
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(policyDir, "POLICY.md"), []byte("# Policy"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create valid playbook
	if err := os.MkdirAll(playbooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(playbooksDir, "LOOP.md"), []byte("# Loop"), 0644); err != nil {
		t.Fatal(err)
	}

	policies := []PolicyRef{
		{Name: "safety-check", Type: "ai-judge", Context: "safety"},
		{Name: "missing-context", Type: "ai-judge", Context: ""},
		{Name: "budget-check", Type: "cel", Context: ""},
	}
	detections := []DetectionRef{
		{Name: "loop", Action: "playbook"},
		{Name: "missing-playbook", Action: "playbook"},
		{Name: "cost_anomaly", Action: "alert"},
	}

	result := ValidateAll(agentsDir, policiesDir, playbooksDir, policies, detections)

	if result.OK() {
		t.Error("ValidateAll() should fail with multiple errors")
	}

	summary := result.Summary()

	// Should have 3 errors:
	// 1. invalid-agent missing versions/
	// 2. missing-context policy has no context
	// 3. missing-playbook detection has no MISSING-PLAYBOOK.md
	if len(result.Errors) != 3 {
		t.Errorf("Expected 3 errors, got %d: %v", len(result.Errors), result.Errors)
	}

	// Should have 2 warnings (both agents missing EVOLVE.md)
	if len(result.Warnings) != 2 {
		t.Errorf("Expected 2 warnings, got %d: %v", len(result.Warnings), result.Warnings)
	}

	// Check error messages
	expectedErrors := []string{
		"missing versions/ directory",
		"has no context path configured",
		"MISSING-PLAYBOOK.md not found",
	}
	for _, expected := range expectedErrors {
		if !strings.Contains(summary, expected) {
			t.Errorf("Summary missing expected error %q\nGot:\n%s", expected, summary)
		}
	}
}
