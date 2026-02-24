package mdloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidationResult holds the outcome of a ValidateAll check.
type ValidationResult struct {
	Errors   []string
	Warnings []string
}

// OK returns true if there are no errors.
func (v *ValidationResult) OK() bool {
	return len(v.Errors) == 0
}

// Summary returns a human-readable summary of the validation result.
func (v *ValidationResult) Summary() string {
	var b strings.Builder
	if v.OK() {
		fmt.Fprintf(&b, "Validation passed (%d warnings)\n", len(v.Warnings))
	} else {
		fmt.Fprintf(&b, "Validation failed: %d errors, %d warnings\n", len(v.Errors), len(v.Warnings))
	}
	for _, e := range v.Errors {
		fmt.Fprintf(&b, "  ERROR: %s\n", e)
	}
	for _, w := range v.Warnings {
		fmt.Fprintf(&b, "  WARN:  %s\n", w)
	}
	return b.String()
}

// PolicyRef describes a policy to validate. Used by ValidateAll to check
// that AI-judge policies have their referenced POLICY.md files.
type PolicyRef struct {
	Name    string // policy name from config
	Type    string // "ai-judge" or "" (deterministic/CEL)
	Context string // path to POLICY.md (relative to policies dir), only for ai-judge
}

// DetectionRef describes a detection rule to validate. Used by ValidateAll
// to check that playbook-action detections have their corresponding MD files.
type DetectionRef struct {
	Name   string // detection type: "loop", "spiral", "cost_anomaly", "drift", etc.
	Action string // "playbook", "pause", "alert", "terminate"
}

// ValidateAll checks that all referenced Markdown files exist and that the
// directory structure is well-formed. It is used by `agentwarden doctor`
// and `agentwarden policy validate`.
//
// Checks performed:
//   - Every agent directory has AGENT.md
//   - Every agent directory has at least one version/vN/PROMPT.md
//   - EVOLVE.md existence is optional (warning if missing)
//   - Every AI-judge policy has its referenced POLICY.md
//   - Every detection with action "playbook" has a corresponding playbooks/<NAME>.md
func ValidateAll(
	agentsDir, policiesDir, playbooksDir string,
	policies []PolicyRef,
	detections []DetectionRef,
) *ValidationResult {
	result := &ValidationResult{}

	validateAgents(agentsDir, result)
	validatePolicies(policiesDir, policies, result)
	validatePlaybooks(playbooksDir, detections, result)

	return result
}

// validateAgents checks every subdirectory of agentsDir for required files.
func validateAgents(agentsDir string, result *ValidationResult) {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("agents directory does not exist: %s", agentsDir))
			return
		}
		result.Errors = append(result.Errors,
			fmt.Sprintf("cannot read agents directory %s: %s", agentsDir, err))
		return
	}

	agentCount := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		agentCount++
		agentID := e.Name()
		agentDir := filepath.Join(agentsDir, agentID)

		// Check AGENT.md
		agentMD := filepath.Join(agentDir, "AGENT.md")
		if _, err := os.Stat(agentMD); os.IsNotExist(err) {
			result.Errors = append(result.Errors,
				fmt.Sprintf("agent %q: missing AGENT.md (expected at %s)", agentID, agentMD))
		}

		// Check EVOLVE.md (optional, warning only)
		evolveMD := filepath.Join(agentDir, "EVOLVE.md")
		if _, err := os.Stat(evolveMD); os.IsNotExist(err) {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("agent %q: no EVOLVE.md found — evolution engine will use defaults", agentID))
		}

		// Check at least one version with PROMPT.md
		versionsDir := filepath.Join(agentDir, "versions")
		vEntries, err := os.ReadDir(versionsDir)
		if err != nil {
			if os.IsNotExist(err) {
				result.Errors = append(result.Errors,
					fmt.Sprintf("agent %q: missing versions/ directory", agentID))
			} else {
				result.Errors = append(result.Errors,
					fmt.Sprintf("agent %q: cannot read versions/ directory: %s", agentID, err))
			}
			continue
		}

		foundPrompt := false
		for _, ve := range vEntries {
			if !ve.IsDir() {
				continue
			}
			if !versionDirPattern.MatchString(ve.Name()) {
				continue
			}
			promptMD := filepath.Join(versionsDir, ve.Name(), "PROMPT.md")
			if _, err := os.Stat(promptMD); err == nil {
				foundPrompt = true
				break
			}
		}
		if !foundPrompt {
			result.Errors = append(result.Errors,
				fmt.Sprintf("agent %q: no version directory contains a PROMPT.md", agentID))
		}
	}

	if agentCount == 0 {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("no agent directories found in %s", agentsDir))
	}
}

// validatePolicies checks that every AI-judge policy has its POLICY.md.
func validatePolicies(policiesDir string, policies []PolicyRef, result *ValidationResult) {
	for _, p := range policies {
		if p.Type != "ai-judge" {
			continue
		}

		if p.Context == "" {
			result.Errors = append(result.Errors,
				fmt.Sprintf("policy %q: AI-judge policy has no context path configured", p.Name))
			continue
		}

		// The Context field is the subdirectory name within policiesDir.
		policyMD := filepath.Join(policiesDir, p.Context, "POLICY.md")
		if _, err := os.Stat(policyMD); os.IsNotExist(err) {
			result.Errors = append(result.Errors,
				fmt.Sprintf("policy %q: referenced POLICY.md not found at %s", p.Name, policyMD))
		}
	}
}

// validatePlaybooks checks that every detection with action "playbook" has
// a corresponding playbooks/<NAME>.md file.
func validatePlaybooks(playbooksDir string, detections []DetectionRef, result *ValidationResult) {
	for _, d := range detections {
		if d.Action != "playbook" {
			continue
		}

		filename := strings.ToUpper(d.Name) + ".md"
		playbookPath := filepath.Join(playbooksDir, filename)
		if _, err := os.Stat(playbookPath); os.IsNotExist(err) {
			result.Errors = append(result.Errors,
				fmt.Sprintf("detection %q: action is 'playbook' but %s not found", d.Name, playbookPath))
		}
	}
}
