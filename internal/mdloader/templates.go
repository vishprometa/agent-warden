package mdloader

import "fmt"

// AgentMDTemplate returns a starter AGENT.md for the given agent ID.
// This file defines the agent's identity, capabilities, and constraints
// that the evolution engine and policy evaluator use as context.
func AgentMDTemplate(agentID string) string {
	return fmt.Sprintf(`# Agent: %s

## Identity

A brief description of what this agent does and what domain it operates in.

## Capabilities

- Tool use (list the tools this agent has access to)
- Data access (databases, APIs, file systems)
- Action types (read-only, read-write, external calls)

## Constraints

- Maximum cost per session: $10.00
- Maximum actions per minute: 100
- Prohibited actions: (list any hard-blocked operations)
- Required approvals: (list actions requiring human approval)

## Behavioral Guidelines

- Be concise and action-oriented
- Always verify before destructive operations
- Prefer read-only exploration before mutations
- Escalate uncertainty rather than guessing

## Context

- Upstream provider: (e.g. OpenAI, Anthropic, Google)
- Model: (e.g. gpt-4o, claude-sonnet-4)
- Deployment: (e.g. production, staging)
- Owner: (team or individual responsible)
`, agentID)
}

// EvolveMDTemplate returns a starter EVOLVE.md for the given agent ID.
// This file tells the evolution engine what to optimize, what to avoid,
// and how aggressively to explore improvements.
func EvolveMDTemplate(agentID string) string {
	return fmt.Sprintf(`# Evolution Rules: %s

## Optimization Goals

Ranked by priority:

1. **Task completion rate** — the agent should successfully complete its assigned tasks
2. **Cost efficiency** — minimize token usage and API calls without sacrificing quality
3. **Error reduction** — reduce policy violations, tool failures, and timeout rates
4. **Latency** — faster responses where possible, but never at the expense of correctness

## What May Be Changed

- System prompt wording and structure
- Tool selection hints and ordering guidance
- Retry and fallback strategies
- Output format instructions

## What Must NOT Be Changed

- Core identity and role (defined in AGENT.md)
- Security constraints and prohibited actions
- Approval requirements
- Maximum budget limits

## Evolution Constraints

- **Shadow testing required**: All prompt changes must pass shadow testing before promotion
- **Minimum shadow runs**: 20
- **Success threshold**: Candidate must score >= 0.95x the current version's composite score
- **Cooldown**: Wait at least 6 hours between evolution cycles
- **Rollback trigger**: Auto-rollback if composite score drops > 10%% within 1 hour of promotion

## Failure Patterns to Watch

- Repeated tool call failures (same tool, same error)
- Cost spikes (session cost > 3x rolling average)
- Loop detection triggers (agent stuck in action cycles)
- Conversation spirals (repetitive LLM output)

## Notes

Add any agent-specific evolution notes here. The evolution engine reads this
file as context when analyzing failure patterns and proposing improvements.
`, agentID)
}

// PromptMDTemplate returns a starter PROMPT.md (v1) for the given agent ID.
// This is the actual system prompt sent to the LLM, versioned and evolved.
func PromptMDTemplate(agentID string) string {
	return fmt.Sprintf(`# System Prompt: %s (v1)

You are %s, an AI agent operating under AgentWarden governance.

## Your Role

Describe the agent's primary function and domain expertise here.

## Instructions

1. Carefully analyze the user's request before taking action
2. Use the available tools to gather information before making changes
3. Verify your understanding by summarizing what you plan to do
4. Execute actions step by step, checking results after each step
5. Report completion with a clear summary of what was done

## Tools Available

- List the tools this agent can use
- Describe when to use each tool
- Note any ordering dependencies between tools

## Output Format

Respond in clear, structured text. Use markdown formatting when appropriate.
Always include:
- What you understood the request to be
- What actions you took
- What the outcome was
- Any warnings or follow-up items

## Constraints

- Never perform destructive operations without explicit confirmation
- Stay within the scope of your defined role
- If uncertain, ask for clarification rather than guessing
- Respect rate limits and cost budgets
`, agentID, agentID)
}

// PolicyMDTemplate returns a starter POLICY.md for the given policy name.
// This file provides the semantic context that the AI-judge LLM uses when
// evaluating whether an agent action should be allowed or denied.
func PolicyMDTemplate(policyName string) string {
	return fmt.Sprintf(`# Policy: %s

## Purpose

Describe what this policy protects and why it exists.

## Evaluation Criteria

When evaluating an agent action against this policy, consider:

1. **Intent**: What is the agent trying to accomplish?
2. **Risk**: What could go wrong if this action is allowed?
3. **Scope**: Is the action proportional to the task?
4. **Reversibility**: Can the action be undone if it causes harm?
5. **Precedent**: Is this consistent with past allowed actions?

## Allow When

- The action is within the agent's defined scope
- The risk is low and the action is reversible
- The action follows established patterns for this agent

## Deny When

- The action exceeds the agent's defined scope
- The action poses irreversible risk to production systems
- The action pattern matches known failure modes
- The cost or resource consumption is disproportionate

## Examples

### Allowed

> Agent requests read access to the users table to answer a support question.
> **Verdict: ALLOW** — read-only access within scope, low risk.

### Denied

> Agent attempts to DROP a production table to "clean up" unused data.
> **Verdict: DENY** — destructive, irreversible, disproportionate to task.

## Escalation

If the AI-judge cannot confidently determine allow/deny, the action should
be escalated to a human approver via the approval gate.
`, policyName)
}

// PolicyYAMLTemplate returns a starter policy.yaml configuration file
// for the given policy name.
func PolicyYAMLTemplate(policyName string) string {
	return fmt.Sprintf(`# Policy configuration for: %s
# This file is referenced from agentwarden.yaml under the policies[] section.

name: %s
type: ai-judge
effect: deny
message: "Action denied by AI-judge policy: %s"
model: claude-sonnet-4
context: %s   # path within policies_dir, contains POLICY.md
timeout: 10s
timeout_effect: deny  # deny on timeout (fail closed)
`, policyName, policyName, policyName, policyName)
}

// PlaybookTemplate returns a starter playbook MD for the given detection type.
// The playbook is fed to an LLM when the detection fires with action "playbook",
// giving the LLM structured instructions for analyzing and remediating the issue.
//
// Supported detection types: "loop", "spiral", "budget_breach", "drift".
// Unknown types return a generic playbook.
func PlaybookTemplate(detectionType string) string {
	switch detectionType {
	case "loop":
		return loopPlaybook()
	case "spiral":
		return spiralPlaybook()
	case "budget_breach":
		return budgetBreachPlaybook()
	case "drift":
		return driftPlaybook()
	default:
		return genericPlaybook(detectionType)
	}
}

func loopPlaybook() string {
	return `# Playbook: Loop Detection

## Trigger

This playbook is executed when the loop detector identifies an agent stuck in
a repeated action cycle — the same tool call signature appearing N times within
a sliding window.

## Analysis Steps

1. **Identify the loop signature**: What tool or action is being repeated?
2. **Check the tool results**: Are the repeated calls returning errors, or are
   they succeeding but the agent is not recognizing completion?
3. **Examine the context**: Is the agent missing information that would let it
   break out of the loop? Is it misinterpreting a success response?
4. **Check for external dependencies**: Is a downstream API timing out or
   returning inconsistent results?

## Remediation Options

Choose ONE of the following based on your analysis:

### Option A: Inject Guidance
If the agent is stuck because it lacks context, inject a system message:
- Summarize what the agent has already done
- Point out the repetition pattern
- Suggest an alternative approach or tool

### Option B: Pause and Alert
If the loop indicates a genuine problem (broken tool, bad state):
- Pause the session
- Emit an alert with the loop signature and last N tool results
- Wait for human intervention

### Option C: Terminate
If the loop has consumed excessive resources or shows no path to resolution:
- Terminate the session with a clear explanation
- Log the failure pattern for evolution engine analysis

## Response Format

Respond with a JSON object:
` + "```json\n" + `{
  "action": "inject" | "pause" | "terminate",
  "reason": "Brief explanation of your analysis",
  "message": "Message to inject (for inject action) or include in alert/termination"
}
` + "```\n"
}

func spiralPlaybook() string {
	return `# Playbook: Conversation Spiral Detection

## Trigger

This playbook is executed when the spiral detector identifies that an agent's
LLM outputs have become highly repetitive — the semantic similarity between
consecutive responses exceeds the configured threshold.

## Analysis Steps

1. **Review the last 5 responses**: Are they semantically identical, or is there
   subtle progression that the similarity metric missed?
2. **Check the user/system inputs**: Is the agent receiving the same input
   repeatedly, or is it generating repetitive output from varied input?
3. **Identify the cause**:
   - Model degeneration (temperature too low, repetition penalty needed)
   - Missing stop condition (agent does not know when to stop)
   - Ambiguous instructions (agent cannot determine what "done" looks like)
4. **Assess resource consumption**: How much has this spiral cost so far?

## Remediation Options

### Option A: Inject a Pattern Break
Inject a system message that:
- Acknowledges the repetition
- Provides a concrete next step or asks a clarifying question
- Resets the agent's framing of the task

### Option B: Adjust Parameters
If the spiral appears to be a model behavior issue:
- Suggest increasing temperature slightly
- Suggest adding a frequency penalty
- Flag for evolution engine to adjust prompt

### Option C: Terminate with Summary
If the spiral has consumed significant resources:
- Summarize what the agent accomplished before the spiral
- Terminate with a clear explanation
- Log for evolution engine analysis

## Response Format

Respond with a JSON object:
` + "```json\n" + `{
  "action": "inject" | "adjust" | "terminate",
  "reason": "Brief explanation of your analysis",
  "message": "Guidance message or parameter adjustment suggestion"
}
` + "```\n"
}

func budgetBreachPlaybook() string {
	return `# Playbook: Budget Breach

## Trigger

This playbook is executed when a session's cumulative cost exceeds the
configured budget threshold, or when cost velocity (cost per minute) spikes
beyond the anomaly multiplier.

## Analysis Steps

1. **Calculate cost breakdown**: Which actions consumed the most budget?
   (LLM calls vs tool calls vs API requests)
2. **Assess progress**: How much of the task has been completed? Is the agent
   close to finishing, or has it barely started?
3. **Check for waste**: Are there redundant API calls, unnecessarily large
   context windows, or repeated failed operations inflating cost?
4. **Compare to baseline**: How does this session's cost compare to the
   rolling average for this agent?

## Remediation Options

### Option A: Budget Warning
If the agent is making good progress and is near completion:
- Inject a cost-awareness message with remaining budget
- Allow the agent to continue with a tighter per-action budget
- Set a hard cutoff at 1.5x the original budget

### Option B: Pause for Review
If cost is high but the task is valuable:
- Pause the session
- Alert the operator with a cost breakdown
- Wait for approval to continue with additional budget

### Option C: Terminate
If cost is disproportionate to task value:
- Terminate the session immediately
- Provide a cost breakdown in the termination message
- Log the pattern for evolution engine optimization

## Response Format

Respond with a JSON object:
` + "```json\n" + `{
  "action": "warn" | "pause" | "terminate",
  "reason": "Brief explanation of your analysis",
  "message": "Warning message, pause justification, or termination explanation",
  "cost_breakdown": {
    "llm_calls": 0.00,
    "tool_calls": 0.00,
    "total": 0.00
  }
}
` + "```\n"
}

func driftPlaybook() string {
	return `# Playbook: Behavioral Drift Detection

## Trigger

This playbook is executed when the drift detector identifies that an agent's
behavior has shifted significantly from its established baseline — changes in
tool usage patterns, response lengths, error rates, or task completion times
that exceed configured thresholds.

## Analysis Steps

1. **Characterize the drift**: Which metrics have shifted? By how much?
2. **Correlate with changes**: Was there a recent prompt version change,
   model update, or configuration modification?
3. **Evaluate impact**: Is the drift positive (improvement) or negative
   (degradation)?
4. **Check for external factors**: Has the input distribution changed?
   Are downstream services behaving differently?

## Remediation Options

### Option A: Monitor
If the drift is minor or potentially positive:
- Continue monitoring with a tighter alerting threshold
- Flag for evolution engine to evaluate whether the drift improves scores
- Log the drift metrics for trend analysis

### Option B: Investigate
If the drift is significant but the cause is unclear:
- Pause evolution cycles for this agent
- Alert the operator with drift metrics and recent changes
- Recommend manual review of the last N sessions

### Option C: Rollback
If the drift correlates with a recent change and is clearly negative:
- Trigger auto-rollback to the previous version
- Alert the operator that a rollback occurred
- Log the failed change for evolution engine learning

## Response Format

Respond with a JSON object:
` + "```json\n" + `{
  "action": "monitor" | "investigate" | "rollback",
  "reason": "Brief explanation of your analysis",
  "drift_metrics": {
    "metric": "value and direction of change"
  },
  "message": "Recommended next steps"
}
` + "```\n"
}

func genericPlaybook(detectionType string) string {
	return fmt.Sprintf(`# Playbook: %s

## Trigger

This playbook is executed when the %s detector fires with action "playbook".

## Analysis Steps

1. Review the detection event details
2. Examine the session trace for context
3. Assess the severity and potential impact
4. Determine the appropriate remediation

## Remediation Options

### Option A: Inject Guidance
Provide the agent with additional context or instructions to resolve the issue.

### Option B: Pause
Pause the session and alert an operator for manual review.

### Option C: Terminate
Terminate the session if the issue cannot be resolved automatically.

## Response Format

Respond with a JSON object:
`+"```json\n"+`{
  "action": "inject" | "pause" | "terminate",
  "reason": "Brief explanation of your analysis",
  "message": "Guidance, alert details, or termination explanation"
}
`+"```\n", detectionType, detectionType)
}
