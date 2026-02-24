# Configuration Reference

Complete reference for the `agentwarden.yaml` configuration file.

## Table of Contents

- [Overview](#overview)
- [Environment Variable Substitution](#environment-variable-substitution)
- [Config File Locations](#config-file-locations)
- [Server](#server)
- [Upstream](#upstream)
- [Storage](#storage)
- [Policies](#policies)
- [Detection](#detection)
- [Alerts](#alerts)
- [Evolution](#evolution)
- [Full Example](#full-example)

---

## Overview

AgentWarden is configured via a YAML file, typically named `agentwarden.yaml`. Generate a starter config with:

```bash
agentwarden init
```

Start with a specific config file:

```bash
agentwarden start --config /path/to/agentwarden.yaml
```

If no config file is specified, AgentWarden searches these locations in order:

1. `./agentwarden.yaml`
2. `./agentwarden.yml`
3. `~/.config/agentwarden/config.yaml`

If no config file is found, AgentWarden starts with built-in defaults (zero-config mode).

---

## Environment Variable Substitution

All string values in the config file support environment variable substitution using the `${VAR}` syntax:

```yaml
alerts:
  slack:
    webhook_url: ${SLACK_WEBHOOK_URL}
```

Default values are supported with the `:-` separator:

```yaml
server:
  port: ${AGENTWARDEN_PORT:-6777}

storage:
  path: ${DATA_DIR:-./data}/agentwarden.db
```

If the environment variable is not set and no default is provided, the value resolves to an empty string.

---

## Config File Locations

AgentWarden searches for config files in this order:

| Priority | Path |
|----------|------|
| 1 | Explicit `--config` flag |
| 2 | `./agentwarden.yaml` |
| 3 | `./agentwarden.yml` |
| 4 | `~/.config/agentwarden/config.yaml` |

---

## Server

Controls the HTTP server that hosts the proxy, dashboard, and management API on a single port.

```yaml
server:
  port: 6777
  dashboard: true
  log_level: info
  cors: false
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `6777` | TCP port for the combined proxy/dashboard/API server |
| `dashboard` | bool | `true` | Serve the embedded React monitoring dashboard at `/dashboard` |
| `log_level` | string | `info` | Log verbosity. One of: `debug`, `info`, `warn`, `error` |
| `cors` | bool | `false` | Enable CORS headers (Access-Control-Allow-Origin: *). Use for development only. |

The `--dev` flag on `agentwarden start` sets `cors: true` and `log_level: debug`.

---

## Upstream

Configures how AgentWarden routes requests to upstream LLM providers.

```yaml
upstream:
  default: https://api.openai.com/v1
  providers:
    openai: https://api.openai.com/v1
    anthropic: https://api.anthropic.com/v1
    gemini: https://generativelanguage.googleapis.com/v1beta
  passthrough_auth: true
  timeout: 30s
  retries: 2
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default` | string | `https://api.openai.com/v1` | Fallback upstream URL when no provider match is found |
| `providers` | map[string]string | OpenAI, Anthropic, Gemini | Named providers mapped to their base URLs |
| `passthrough_auth` | bool | `true` | Forward the `Authorization` header to the upstream provider unchanged |
| `timeout` | duration | `30s` | HTTP timeout for upstream requests. Applies to both streaming and non-streaming. |
| `retries` | int | `2` | Maximum retry attempts for failed upstream requests |

### Provider Routing

AgentWarden automatically routes requests to the correct upstream based on the model name in the request body. The routing logic uses model name prefixes:

| Model Prefix | Provider |
|-------------|----------|
| `gpt-`, `o1-`, `o3-`, `o4-`, `chatgpt-` | openai |
| `claude-` | anthropic |
| `gemini-`, `gemma-` | gemini |
| `text-embedding-`, `text-moderation-`, `dall-e-`, `whisper-`, `tts-` | openai |

If a model name contains a provider key as a substring (e.g., `openai/gpt-4`), routing matches on that. If no match is found, the `default` upstream is used.

---

## Storage

Configures where AgentWarden persists traces, sessions, agents, and audit logs.

```yaml
storage:
  driver: sqlite
  path: ./data/agentwarden.db
  connection: ""
  retention: 720h
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `driver` | string | `sqlite` | Storage backend. Currently only `sqlite` is supported. |
| `path` | string | `./agentwarden.db` | File path for the SQLite database |
| `connection` | string | `""` | Connection string (reserved for future PostgreSQL support) |
| `retention` | duration | `720h` (30 days) | How long to retain trace data. Traces older than this are pruned. |

The SQLite database stores six tables: `traces`, `sessions`, `agents`, `agent_versions`, `approvals`, and `violations`.

---

## Policies

An ordered list of governance policies evaluated against every intercepted action. Policies are evaluated in order; the first `deny` or `terminate` result short-circuits evaluation.

```yaml
policies:
  - name: session-budget
    condition: "session.cost > 10.00"
    effect: terminate
    message: "Session killed: exceeded $10 budget"

  - name: block-shell-exec
    condition: 'action.type == "tool.call" && action.name == "shell_exec"'
    effect: deny
    message: "Shell execution is not allowed"
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique name for this policy |
| `condition` | string | Yes (for CEL) | CEL expression that evaluates to a boolean |
| `effect` | string | Yes | What happens when the condition matches. One of: `allow`, `deny`, `terminate`, `throttle`, `approve` |
| `message` | string | Yes | Human-readable message returned to the agent and shown in the dashboard |
| `type` | string | No | Policy type. Empty string (default) for CEL-based. `ai-judge` for LLM-evaluated. |
| `delay` | duration | No | Delay duration for `throttle` effect (e.g., `5s`) |
| `prompt` | string | No | LLM prompt for `ai-judge` type policies |
| `model` | string | No | LLM model for `ai-judge` type policies |
| `approvers` | list[string] | No | Email addresses or identifiers for `approve` effect |
| `timeout` | duration | No | Timeout for approval requests |
| `timeout_effect` | string | No | Effect to apply when an approval times out (`deny` or `allow`) |

For detailed policy authoring guidance, see the [Policy Authoring Guide](policies.md).

---

## Detection

Configures the three anomaly detection subsystems that run on every intercepted action.

```yaml
detection:
  loop:
    enabled: true
    threshold: 5
    window: 60s
    action: pause
  cost_anomaly:
    enabled: true
    multiplier: 10
    action: alert
  spiral:
    enabled: true
    similarity_threshold: 0.9
    window: 5
    action: alert
```

### Loop Detection

Detects when the same action (same type + name + model) is repeated within a sliding window.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable loop detection |
| `threshold` | int | `5` | Number of identical actions within the window to trigger detection |
| `window` | duration | `60s` | Sliding time window for counting identical actions |
| `action` | string | `pause` | Response when a loop is detected: `pause`, `alert`, `terminate` |

### Cost Anomaly Detection

Detects when the cost-per-action rate spikes compared to a baseline established from earlier actions in the same session.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable cost anomaly detection |
| `multiplier` | float | `10` | Fire when the recent cost rate exceeds the baseline rate by this factor |
| `action` | string | `alert` | Response when an anomaly is detected: `pause`, `alert`, `terminate` |

The detector requires at least 3 data points before it can establish a baseline. It compares the average cost-per-action in the last 30 seconds against the average cost-per-action from earlier in the session.

### Spiral Detection

Detects when an LLM produces highly similar consecutive outputs, indicating a non-converging conversation loop. Uses word-frequency-based cosine similarity.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable spiral detection |
| `similarity_threshold` | float | `0.9` | Cosine similarity threshold (0.0 to 1.0). Higher values require more similar outputs. |
| `window` | int | `5` | Number of consecutive outputs to compare |
| `action` | string | `alert` | Response when a spiral is detected: `pause`, `alert`, `terminate` |

---

## Alerts

Configures notification channels for policy violations and anomaly detections. Alerts are deduplicated with a 5-minute TTL per (type, agent, session) combination.

```yaml
alerts:
  slack:
    webhook_url: ${SLACK_WEBHOOK_URL}
    channel: "#agent-alerts"
  webhook:
    url: https://your-app.com/webhooks/agentwarden
    secret: ${WEBHOOK_SECRET}
```

### Slack

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `webhook_url` | string | `""` | Slack incoming webhook URL. Leave empty to disable. |
| `channel` | string | `""` | Channel override (optional; uses the webhook default if empty) |

### Webhook

Generic HTTP webhook for integration with any alerting system.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | `""` | HTTP endpoint to POST alert payloads to. Leave empty to disable. |
| `secret` | string | `""` | HMAC-SHA256 signing secret. If set, the payload is signed and the signature is sent in the `X-AgentWarden-Signature` header. |

Alert payload format:

```json
{
  "type": "loop",
  "severity": "warning",
  "title": "Detection: loop",
  "message": "Loop detected: action \"llm.chat|chat_completion|gpt-4o\" repeated 6 times in 60s",
  "agent_id": "research-agent",
  "session_id": "ses_abc123xyz",
  "details": {
    "signature": "llm.chat|chat_completion|gpt-4o",
    "count": 6,
    "threshold": 5
  },
  "timestamp": "2026-02-25T10:30:00Z"
}
```

---

## Evolution

Configures the self-evolution engine, a separate Python sidecar that analyzes agent performance and proposes configuration improvements. Disabled by default.

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

### Top-Level

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable the evolution engine |

### Scoring

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `metrics` | list[string] | `[]` | Metrics to include in composite scoring |
| `window` | duration | `24h` | Time window over which to evaluate agent performance |

### Constraints

A list of natural-language constraints that proposed diffs must satisfy. The evolution engine validates proposed changes against these constraints before running shadow tests.

### Shadow

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `required` | bool | `true` | Require shadow testing before promoting a candidate |
| `min_runs` | int | `10` | Minimum number of shadow runs before a candidate can be promoted |
| `success_threshold` | float | `0.05` | Minimum improvement ratio (candidate vs current) required for promotion |

### Rollback

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auto` | bool | `true` | Automatically roll back if a promoted version degrades performance |
| `trigger` | string | `""` | Condition expression that triggers an automatic rollback |

### Triggers

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Trigger type: `scheduled` (cron-based) or `metric_threshold` (reactive) |
| `cron` | string | Cron expression for scheduled triggers (e.g., `0 */6 * * *` = every 6 hours) |
| `condition` | string | Metric condition for threshold triggers |
| `cooldown` | duration | Minimum time between consecutive evolution cycles |

For the full evolution guide, see [Self-Evolution Guide](evolution.md).

---

## Full Example

```yaml
server:
  port: 6777
  dashboard: true
  log_level: info
  cors: false

upstream:
  default: https://api.openai.com/v1
  providers:
    openai: https://api.openai.com/v1
    anthropic: https://api.anthropic.com/v1
    gemini: https://generativelanguage.googleapis.com/v1beta
  passthrough_auth: true
  timeout: 120s
  retries: 2

storage:
  driver: sqlite
  path: ./data/agentwarden.db
  retention: 720h

policies:
  - name: session-budget
    condition: "session.cost > 10.00"
    effect: terminate
    message: "Session killed: exceeded $10 budget"

  - name: rate-limit
    condition: "session.action_count > 200"
    effect: throttle
    message: "Too many actions in session"
    delay: 5s

  - name: block-shell-exec
    condition: 'action.type == "tool.call" && action.name == "shell_exec"'
    effect: deny
    message: "Shell execution is not allowed"

  - name: block-prod-db-writes
    condition: >
      action.type == "db.query"
      && action.target.contains("production")
    effect: deny
    message: "Production database writes are blocked"

  - name: db-write-approval
    condition: 'action.type == "db.query"'
    effect: approve
    message: "Database write requires human approval"
    approvers: ["admin@example.com"]
    timeout: 5m
    timeout_effect: deny

detection:
  loop:
    enabled: true
    threshold: 5
    window: 60s
    action: pause
  cost_anomaly:
    enabled: true
    multiplier: 10
    action: alert
  spiral:
    enabled: true
    similarity_threshold: 0.9
    window: 5
    action: alert

alerts:
  slack:
    webhook_url: ${SLACK_WEBHOOK_URL}
    channel: "#agent-alerts"
  webhook:
    url: ${WEBHOOK_URL}
    secret: ${WEBHOOK_SECRET}

evolution:
  enabled: false
```
