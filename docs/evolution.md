# Self-Evolution Guide

AgentWarden's evolution engine is a Python sidecar that observes agent behavior over time, identifies failure patterns, and proposes configuration improvements -- all validated through shadow testing before promotion.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [The Evolution Loop](#the-evolution-loop)
- [Evolvable Components and Risk Levels](#evolvable-components-and-risk-levels)
- [Scoring](#scoring)
- [Failure Analysis](#failure-analysis)
- [Shadow Runner](#shadow-runner)
- [Auto-Rollback](#auto-rollback)
- [Configuration Reference](#configuration-reference)
- [Running the Sidecar](#running-the-sidecar)

---

## Overview

The evolution engine automates a cycle that most teams do manually:

1. Review agent performance logs
2. Identify recurring failures and inefficiencies
3. Propose changes to prompts, config, or policies
4. Test the changes in a safe environment
5. Deploy if they improve outcomes, roll back if they do not

The evolution engine runs this loop continuously, using LLM analysis to identify patterns and generate diffs, and shadow testing to validate changes before they affect production traffic.

**Key principle:** No change goes live without evidence that it improves performance. Shadow testing is mandatory for medium and high risk changes. Auto-rollback reverts any promotion that causes degradation.

---

## Architecture

```
                     AgentWarden Proxy (:6777)
                            |
                    Management API
                     /api/sessions
                     /api/traces
                     /api/agents/:id/stats
                     /api/agents/:id/versions
                            |
                  +---------+---------+
                  |                   |
            Evolution Sidecar    Dashboard
            (Python process)     (React SPA)
                  |
         +--------+--------+
         |        |        |
      Scorer   Analyzer  Shadow
               (LLM)     Runner
```

The evolution sidecar is a standalone Python process that communicates with AgentWarden exclusively through the management API. It does not intercept traffic or modify the proxy configuration directly.

### Components

| Component | Role |
|-----------|------|
| **Scorer** | Computes composite scores for sessions and aggregates them per agent version |
| **Analyzer** | Uses an LLM to identify failure patterns and propose improvements |
| **Diff Generator** | Produces concrete configuration diffs from improvement proposals |
| **Validator** | Checks diffs for syntax, semantic correctness, and risk level |
| **Shadow Runner** | Runs candidate configurations in parallel with the live version |
| **Promoter** | Promotes validated candidates and handles rollback |

---

## The Evolution Loop

Each evolution cycle follows six stages:

### 1. Score

Fetch recent sessions for the target agent and compute a composite score for each:

```
SessionScore = weighted(
    success_rate     * 0.30,
    cost_efficiency  * 0.25,
    task_completion  * 0.25,
    error_rate       * 0.10,  (inverted: lower is better)
    latency          * 0.10,  (inverted: lower is better)
)
```

Scores are aggregated into a `VersionScore` across the configured time window (default: 24 hours).

### 2. Analyze

Feed the aggregated scores and sample traces to an LLM to identify failure patterns:

- **Timeout patterns**: Sessions that timed out without completing
- **Policy violations**: Recurring policy denials
- **Loop detections**: Agents stuck in repeated action loops
- **Cost overruns**: Sessions that exceeded budget
- **Task failures**: Sessions that completed but with poor outcomes
- **Errors**: Upstream errors, malformed requests

Each pattern includes frequency, severity, affected sessions, and an LLM-identified root cause.

### 3. Propose

For each identified failure pattern, the LLM generates improvement proposals:

```
FailurePattern -> Improvement -> EvolutionDiff
```

Each `EvolutionDiff` specifies:

- **component**: What to change (`prompt`, `config`, `tools`, `policy`)
- **before**: Current value
- **after**: Proposed value
- **risk_level**: `low`, `medium`, `high`, `critical`
- **reasoning**: Why this change should help

Diffs are validated for syntax, semantic correctness, and constraint compliance before proceeding.

### 4. Shadow

Validated diffs create candidate agent versions that run in shadow mode alongside the live version:

- The live version handles actual traffic normally
- The shadow version processes the same inputs but its outputs are discarded
- Both versions are scored identically

Shadow tests run until `min_runs` is reached (default: 10 sessions).

### 5. Compare

After shadow testing completes, the engine compares candidate vs current scores:

```
improvement_ratio = (candidate_composite - current_composite) / current_composite
```

The comparison produces a recommendation:

| Recommendation | Criteria |
|---------------|----------|
| `promote` | `improvement_ratio >= success_threshold` AND constraints satisfied |
| `reject` | `improvement_ratio < 0` OR constraints violated |
| `extend_shadow` | Insufficient data for statistical significance |

### 6. Promote or Reject

- **Promote**: The candidate version replaces the current version. The old version is marked `retired`.
- **Reject**: The candidate is discarded. The failure patterns remain for the next cycle to analyze with different approaches.
- **Extend**: The shadow test continues with more runs.

---

## Evolvable Components and Risk Levels

| Component | Description | Typical Risk |
|-----------|-------------|-------------|
| `prompt` | System prompt modifications | Medium |
| `config` | Agent configuration (temperature, max_tokens, etc.) | Low |
| `tools` | Available tool set and tool descriptions | High |
| `policy` | Governance policy conditions and effects | Critical |

### Risk Level Definitions

| Level | Shadow Required | Approval Required | Auto-Promote |
|-------|----------------|-------------------|-------------|
| `low` | Yes (if configured) | No | Yes |
| `medium` | Yes | No | Yes (if threshold met) |
| `high` | Yes (extended) | Human review | No |
| `critical` | Yes (extended) | Always | Never |

---

## Scoring

### Session Score

Each session receives a composite score based on these metrics:

| Metric | Range | Description |
|--------|-------|-------------|
| `success_rate` | 0.0 - 1.0 | Fraction of actions that completed without errors |
| `cost_efficiency` | 0.0 - 1.0 | Normalized cost per successful action (lower cost = higher score) |
| `task_completion` | 0.0 - 1.0 | Estimated completion ratio for the session's task |
| `error_rate` | 0.0 - 1.0 | Fraction of actions that resulted in errors |
| `avg_latency_ms` | 0+ | Average action latency in milliseconds |
| `violation_count` | 0+ | Number of policy violations in the session |

### Version Score

Version scores aggregate session scores over a time window:

- `avg_composite`: Mean composite score across all sessions
- `avg_success_rate`, `avg_cost_usd`, `avg_error_rate`: Per-metric averages
- `p50_latency_ms`, `p95_latency_ms`: Latency percentiles
- `total_violations`: Sum of violations across all sessions

---

## Failure Analysis

The analyzer identifies six categories of failure patterns:

| Category | Description | Example |
|----------|-------------|---------|
| `timeout` | Session or action timed out | Agent stuck waiting for response |
| `policy_violation` | Action blocked by policy | Budget exceeded, tool blocked |
| `loop` | Detected repeated actions | Same API call made 10 times |
| `cost_overrun` | Session cost exceeded threshold | $50 session on a $10 budget |
| `error` | Upstream or system error | 429 rate limit, 500 server error |
| `task_failure` | Session completed but with poor outcome | Low task completion score |

Each pattern includes:

- **frequency**: How many times it occurred in the analysis window
- **severity**: 0.0 (cosmetic) to 1.0 (critical)
- **affected_sessions**: Session IDs where the pattern appeared
- **root_cause**: LLM-identified root cause analysis
- **sample_trace_ids**: Representative traces for debugging

---

## Shadow Runner

The shadow runner validates candidate configurations by running them in parallel with the live version.

### How Shadow Testing Works

1. When a real request arrives for the target agent, the shadow runner intercepts it
2. The request is processed by both the live version and the candidate version
3. The live version's response is returned to the caller (production traffic is unaffected)
4. Both responses are scored independently
5. After `min_runs` sessions, scores are compared

### Shadow Configuration

```yaml
evolution:
  shadow:
    required: true           # Require shadow testing before promotion
    min_runs: 10             # Minimum shadow sessions
    success_threshold: 0.05  # 5% improvement required for promotion
```

### Safety Guarantees

- Shadow responses are never returned to the caller
- Shadow runs do not count toward session costs (they use the same input but discard output)
- Shadow runs are isolated -- a crash in the shadow path does not affect production
- The shadow runner respects `max_concurrent_shadows` to limit resource usage

---

## Auto-Rollback

If a promoted version degrades performance, automatic rollback reverts to the previous version.

### How Rollback Works

1. After promotion, the engine continues scoring the new version
2. If the rollback trigger condition is met, the version is reverted
3. The rolled-back version is marked with status `rolled_back`
4. An alert is sent with details about why the rollback occurred
5. The failure is fed back into the next analysis cycle

### Configuration

```yaml
evolution:
  rollback:
    auto: true                     # Enable automatic rollback
    trigger: "error_rate > 0.15"   # Roll back when error rate exceeds 15%
```

### Rollback Triggers

The `trigger` field supports simple metric comparisons:

- `error_rate > 0.15` -- Roll back if error rate exceeds 15%
- `avg_cost_usd > 10.0` -- Roll back if average session cost exceeds $10
- `success_rate < 0.80` -- Roll back if success rate drops below 80%

---

## Configuration Reference

### agentwarden.yaml (Proxy Side)

```yaml
evolution:
  enabled: false
  scoring:
    metrics:
      - success_rate
      - cost_efficiency
      - task_completion
      - error_rate
    window: 24h
  constraints:
    - "cost_efficiency must not decrease by more than 10%"
    - "error_rate must not increase"
  shadow:
    required: true
    min_runs: 10
    success_threshold: 0.05
  rollback:
    auto: true
    trigger: "error_rate > 0.15"
  triggers:
    - type: scheduled
      cron: "0 */6 * * *"
      cooldown: 1h
    - type: metric_threshold
      condition: "error_rate > 0.10"
      cooldown: 30m
```

### Evolution Sidecar Config (Python)

The sidecar is configured via `EvolutionConfig`:

| Field | Default | Description |
|-------|---------|-------------|
| `api_url` | `http://localhost:6777` | AgentWarden management API base URL |
| `model` | `gpt-4o` | LLM model for analysis and diff generation |
| `scoring_window` | `24h` | Time window for scoring |
| `min_shadow_runs` | `10` | Minimum shadow runs before promotion |
| `promotion_threshold` | `0.05` | Minimum improvement ratio for promotion |
| `max_concurrent_shadows` | `1` | Max simultaneous shadow tests |
| `poll_interval` | `5m` | How often the engine checks for new data |
| `auto_approve_low_risk` | `true` | Auto-approve low-risk diffs passing shadow tests |
| `min_sessions_for_analysis` | `5` | Minimum sessions needed before analyzing |
| `max_diffs_per_cycle` | `3` | Maximum diffs proposed per cycle |
| `openai_api_key` | `None` | API key (falls back to `OPENAI_API_KEY` env var) |

---

## Running the Sidecar

### Install

```bash
cd evolution
pip install -e ".[dev]"
```

### Run

```bash
# Start with defaults (connects to localhost:6777)
evolution run --agent-id my-agent

# Custom config
evolution run \
  --agent-id my-agent \
  --api-url http://agentwarden:6777 \
  --model gpt-4o \
  --poll-interval 300 \
  --scoring-window 86400

# One-shot analysis (no continuous loop)
evolution analyze --agent-id my-agent
```

### Docker

```bash
docker run \
  --network host \
  -e OPENAI_API_KEY=$OPENAI_API_KEY \
  ghcr.io/agentwarden/evolution:latest \
  run --agent-id my-agent
```

### With Docker Compose

Add the evolution sidecar to your `docker-compose.yml`:

```yaml
services:
  agentwarden:
    image: ghcr.io/agentwarden/agentwarden
    ports:
      - "6777:6777"
    volumes:
      - ./data:/data
      - ./agentwarden.yaml:/etc/agentwarden/config.yaml:ro

  evolution:
    image: ghcr.io/agentwarden/evolution
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
    command: run --agent-id my-agent --api-url http://agentwarden:6777
    depends_on:
      - agentwarden
```
