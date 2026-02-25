package evolution

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Proposer uses an LLM to generate new PROMPT.md candidates based on
// the analyzer's findings. It produces a modified system prompt that
// addresses identified failure patterns while respecting EVOLVE.md constraints.
type Proposer struct {
	llm       *LLMClient
	agentsDir string
}

// ProposalInput bundles all context the proposer needs.
type ProposalInput struct {
	AgentID         string
	AgentMD         string
	EvolveMD        string
	CurrentPromptMD string
	CurrentVersion  string
	Analysis        *AnalysisResult
}

// Proposal is the LLM-generated candidate for a new PROMPT.md version.
type Proposal struct {
	NewPromptMD      string // full new PROMPT.md content
	Diff             string // human-readable diff summary
	Reasoning        string // why these changes were made
	CandidateVersion string // e.g. "v4-candidate"
}

// NewProposer creates a Proposer with the given LLM client and agents directory.
func NewProposer(llm *LLMClient, agentsDir string) *Proposer {
	return &Proposer{
		llm:       llm,
		agentsDir: agentsDir,
	}
}

// Propose generates a new PROMPT.md candidate based on analysis results.
// It sends the current prompt, analysis, and constraints to the LLM and
// parses the structured response into a Proposal.
func (p *Proposer) Propose(ctx context.Context, input ProposalInput) (*Proposal, error) {
	systemPrompt := `You are an agent prompt engineer for AgentWarden. Your job is to produce an improved version of an agent's system prompt (PROMPT.md) based on failure analysis.

Rules:
1. You MUST respect all constraints listed in EVOLVE.md
2. Changes should be minimal and targeted — fix identified issues without rewriting unrelated sections
3. Preserve the agent's core identity from AGENT.md
4. Each change must address a specific failure pattern from the analysis

Respond with EXACTLY this structure:

REASONING:
[Why you made each change, mapped to specific failure patterns]

DIFF_SUMMARY:
[Human-readable summary of what changed, line by line]

NEW_PROMPT_MD:
[The complete new PROMPT.md content — include the ENTIRE file, not just changes]`

	var userMsg strings.Builder
	userMsg.WriteString("## AGENT.md\n")
	userMsg.WriteString(input.AgentMD)
	userMsg.WriteString("\n\n## EVOLVE.md\n")
	userMsg.WriteString(input.EvolveMD)
	userMsg.WriteString("\n\n## Current PROMPT.md (")
	userMsg.WriteString(input.CurrentVersion)
	userMsg.WriteString(")\n")
	userMsg.WriteString(input.CurrentPromptMD)
	userMsg.WriteString("\n\n## Analysis Results\n")

	if input.Analysis != nil {
		userMsg.WriteString("\n### Failure Patterns\n")
		for _, pattern := range input.Analysis.FailurePatterns {
			fmt.Fprintf(&userMsg, "- %s\n", pattern)
		}
		userMsg.WriteString("\n### Recommendations\n")
		for _, rec := range input.Analysis.Recommendations {
			fmt.Fprintf(&userMsg, "- %s\n", rec)
		}
		fmt.Fprintf(&userMsg, "\n### Priority: %s\n", input.Analysis.Priority)
	}

	response, err := p.llm.Chat(ctx, systemPrompt, userMsg.String())
	if err != nil {
		return nil, fmt.Errorf("llm proposal failed: %w", err)
	}

	proposal := parseProposalResponse(response)
	proposal.CandidateVersion = nextCandidateVersion(input.CurrentVersion)

	return proposal, nil
}

// parseProposalResponse extracts structured fields from the LLM response.
func parseProposalResponse(response string) *Proposal {
	proposal := &Proposal{}

	// Extract each section by looking for the headers.
	sections := []struct {
		header string
		target *string
	}{
		{"REASONING:", &proposal.Reasoning},
		{"DIFF_SUMMARY:", &proposal.Diff},
		{"NEW_PROMPT_MD:", &proposal.NewPromptMD},
	}

	for i, sec := range sections {
		startIdx := strings.Index(response, sec.header)
		if startIdx == -1 {
			continue
		}

		contentStart := startIdx + len(sec.header)

		// Find where this section ends (at the next section header or end of response).
		endIdx := len(response)
		for j := i + 1; j < len(sections); j++ {
			nextIdx := strings.Index(response, sections[j].header)
			if nextIdx != -1 && nextIdx < endIdx {
				endIdx = nextIdx
				break
			}
		}

		*sec.target = strings.TrimSpace(response[contentStart:endIdx])
	}

	return proposal
}

// nextCandidateVersion derives the candidate version string from the current version.
// e.g. "v3" -> "v4-candidate", "v1" -> "v2-candidate"
func nextCandidateVersion(currentVersion string) string {
	// Strip the "v" prefix and any existing suffix.
	cleaned := strings.TrimPrefix(currentVersion, "v")
	cleaned = strings.Split(cleaned, "-")[0]

	num, err := strconv.Atoi(cleaned)
	if err != nil {
		// If we can't parse it, just append -candidate.
		return currentVersion + "-candidate"
	}

	return fmt.Sprintf("v%d-candidate", num+1)
}

// WriteCandidatePromptMD writes the candidate PROMPT.md to disk at
// agents/<id>/versions/<version>/PROMPT.md, creating directories as needed.
func (p *Proposer) WriteCandidatePromptMD(agentID string, version string, content string) error {
	dir := filepath.Join(p.agentsDir, agentID, "versions", version)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create version directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, "PROMPT.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write PROMPT.md at %s: %w", path, err)
	}

	return nil
}
