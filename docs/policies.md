# Policy Authoring Guide

Policies are the core governance mechanism in AgentWarden. They define rules that are evaluated against every intercepted action, enabling budget enforcement, rate limiting, dangerous action blocking, and human approval gates.

## Table of Contents

- [Policy Structure](#policy-structure)
- [CEL Expression Reference](#cel-expression-reference)
- [Available Variables](#available-variables)
- [Effects](#effects)
- [Policy Types](#policy-types)
- [Budget Policies](#budget-policies)
- [Rate Limiting](#rate-limiting)
- [Dangerous Action Blocking](#dangerous-action-blocking)
- [Approval Routing](#approval-routing)
- [Policy Evaluation Order](#policy-evaluation-order)
- [Common Policy Patterns](#common-policy-patterns)

---

## Policy Structure

Every policy is defined as a YAML object in the `policies` list:

```yaml
policies:
  - name: my-policy          # Unique policy name
    condition: "<CEL expr>"   # Boolean expression evaluated per action
    effect: deny              # What to do when condition is true
    message: "Blocked"        # Human-readable explanation
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier for the policy. Used in logs, traces, and the dashboard. |
| `condition` | string | Yes* | CEL expression that must evaluate to a boolean. Required for CEL and budget policies. |
| `effect` | string | Yes | The action taken when the condition matches. See [Effects](#effects). |
| `message` | string | Yes | Explanation returned to the agent and recorded in traces. |
| `type` | string | No | `""` (CEL, default), `ai-judge` |
| `delay` | duration | No | Delay for `throttle` effect (e.g., `5s`, `1m`) |
| `prompt` | string | No | LLM prompt for `ai-judge` policies |
| `model` | string | No | LLM model for `ai-judge` policies |
| `approvers` | list | No | List of approver identifiers for `approve` effect |
| `timeout` | duration | No | Timeout for `approve` effect before `timeout_effect` applies |
| `timeout_effect` | string | No | Effect when approval times out: `deny` or `allow` |

---

## CEL Expression Reference

AgentWarden uses [CEL (Common Expression Language)](https://github.com/google/cel-go), the same expression language used by Google Cloud IAM and Kubernetes admission webhooks. CEL expressions are compiled at startup and evaluated in the hot path with zero allocations.

### Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `==` | Equality | `action.type == "tool.call"` |
| `!=` | Inequality | `action.name != "safe_tool"` |
| `>`, `>=`, `<`, `<=` | Comparison | `session.cost > 5.0` |
| `&&` | Logical AND | `action.type == "tool.call" && action.name == "shell_exec"` |
| `\|\|` | Logical OR | `action.type == "file.write" \|\| action.type == "code.exec"` |
| `!` | Logical NOT | `!(agent.id == "trusted-agent")` |
| `in` | List membership | `action.name in ["rm", "drop", "delete"]` |

### String Functions

| Function | Description | Example |
|----------|-------------|---------|
| `.contains(s)` | Substring check | `action.target.contains("production")` |
| `.startsWith(s)` | Prefix check | `action.name.startsWith("unsafe_")` |
| `.endsWith(s)` | Suffix check | `action.target.endsWith(".env")` |
| `.matches(re)` | Regex match | `action.name.matches("^(DROP\|DELETE\|TRUNCATE)")` |
| `size(s)` | String length | `size(action.target) > 100` |

### Map Access

```
action.params.key           # Access a parameter value
action.params["key"]        # Same, bracket notation
has(action.params.key)      # Check if a key exists (avoids nil panics)
```

### Type Coercion

```
int(session.cost)           # Cast to integer
double(session.action_count)  # Cast to double
string(action.type)         # Cast to string
```

---

## Available Variables

These variables are available in every CEL policy condition:

### action.*

Information about the specific action being evaluated.

| Variable | Type | Description |
|----------|------|-------------|
| `action.type` | string | Action category. One of: `llm.chat`, `llm.embedding`, `tool.call`, `api.request`, `db.query`, `file.write`, `code.exec`, `mcp.tool` |
| `action.name` | string | Specific action name: model name for LLM calls, tool name for tool calls, etc. |
| `action.params` | map[string]any | Action-specific parameters extracted from the request body |
| `action.target` | string | The resource being acted upon (URL, file path, database, etc.) |

### session.*

Accumulated state for the current agent session.

| Variable | Type | Description |
|----------|------|-------------|
| `session.id` | string | Session identifier |
| `session.agent_id` | string | Agent that owns this session |
| `session.cost` | double | Total accumulated cost in USD for this session |
| `session.action_count` | int | Total number of actions in this session |

### agent.*

Identity of the agent making the request.

| Variable | Type | Description |
|----------|------|-------------|
| `agent.id` | string | Agent identifier (from `X-AgentWarden-Agent-Id` header) |
| `agent.name` | string | Agent display name (defaults to agent ID) |

---

## Effects

When a policy condition evaluates to `true`, the specified effect is applied:

| Effect | HTTP Status | Behavior |
|--------|-------------|----------|
| `allow` | 200 (passthrough) | Explicitly allow the action. Useful as a whitelist before deny rules. |
| `deny` | 403 Forbidden | Block the action. The request is not forwarded upstream. The session remains active. |
| `terminate` | 503 Service Unavailable | Block the action AND terminate the entire session. All subsequent requests for this session are rejected. |
| `throttle` | 200 (delayed) | Allow the action but introduce a delay (specified by `delay` field). Used for rate limiting. |
| `approve` | Pending | Park the request and wait for human approval. The request is held until an approver acts via the dashboard or API. |

### Error Response Format

When a request is denied or terminated, the response body contains:

```json
{
  "error": {
    "code": "policy_denied",
    "message": "Shell execution is not allowed",
    "policy": "block-shell-exec",
    "effect": "denied"
  },
  "trace_id": "01HQZX7ABC123DEF456"
}
```

---

## Policy Types

### Deterministic (CEL)

The default policy type. A CEL expression is compiled at startup and evaluated against every action with zero allocation overhead. This is the recommended type for most policies.

```yaml
- name: block-shell
  condition: 'action.type == "tool.call" && action.name == "shell_exec"'
  effect: deny
  message: "Shell execution is blocked"
```

### AI Judge

Uses an LLM to evaluate whether an action should be allowed. This is useful for policies that require semantic understanding, such as evaluating whether a generated SQL query is safe.

```yaml
- name: sql-safety-judge
  type: ai-judge
  prompt: |
    You are a database security expert. Evaluate whether this SQL query
    is safe to execute against a production database.
    Query: {{action.params.query}}
    Respond with ALLOW or DENY and a brief reason.
  model: gpt-4o-mini
  effect: deny
  message: "AI judge determined this query is unsafe"
```

**Note:** AI judge policies are currently a future extension point. They are parsed and loaded but evaluated as `allow` in the current release.

### Approval

Requires human review before the action can proceed. The request is parked in the approval queue until an approver responds via the dashboard or API.

```yaml
- name: db-write-approval
  condition: 'action.type == "db.query"'
  effect: approve
  message: "Database modifications require human approval"
  approvers: ["dba@example.com"]
  timeout: 5m
  timeout_effect: deny
```

---

## Budget Policies

Budget policies use `session.cost` to enforce spending limits:

```yaml
# Hard stop at $10
- name: hard-budget
  condition: "session.cost > 10.0"
  effect: terminate
  message: "Session terminated: exceeded $10 budget"

# Warning at $5 (deny without terminating)
- name: soft-budget
  condition: "session.cost > 5.0"
  effect: deny
  message: "Session over $5 budget, new requests denied"
```

Cost is calculated using per-model token pricing. AgentWarden ships with pricing data for 20+ models from OpenAI, Anthropic, Google, Meta, Mistral, and DeepSeek. Unknown models use a fallback rate of $1.00/M input and $3.00/M output.

---

## Rate Limiting

Use `session.action_count` to limit the number of actions per session:

```yaml
# Max 200 actions per session
- name: action-limit
  condition: "session.action_count > 200"
  effect: deny
  message: "Session action limit reached"

# Throttle after 100 actions (add delay instead of blocking)
- name: throttle-heavy-use
  condition: "session.action_count > 100"
  effect: throttle
  delay: 2s
  message: "Throttling: session has over 100 actions"
```

---

## Dangerous Action Blocking

Block specific tool calls, file operations, or database queries:

```yaml
# Block shell execution
- name: no-shell
  condition: 'action.type == "tool.call" && action.name == "shell_exec"'
  effect: deny
  message: "Shell execution is blocked"

# Block code execution
- name: no-code-exec
  condition: 'action.type == "code.exec"'
  effect: deny
  message: "Code execution is not allowed"

# Block writes to sensitive files
- name: no-env-write
  condition: 'action.type == "file.write" && action.target.endsWith(".env")'
  effect: deny
  message: "Writing to .env files is blocked"

# Block destructive SQL
- name: no-drop-tables
  condition: >
    action.type == "db.query"
    && action.name.matches("^(DROP|TRUNCATE|DELETE)")
  effect: deny
  message: "Destructive SQL operations are blocked"
```

---

## Approval Routing

Route high-risk actions through human approval with Slack notifications:

```yaml
# Require approval for production deployments
- name: prod-deploy-approval
  condition: >
    action.type == "tool.call"
    && action.name == "deploy"
    && action.target.contains("production")
  effect: approve
  message: "Production deployment requires human approval"
  approvers: ["platform-team@example.com"]
  timeout: 10m
  timeout_effect: deny
```

When an approval-gated action is intercepted:

1. The request is parked in the approval queue
2. An alert is sent to configured channels (Slack, webhook)
3. Approvers can approve or deny via the dashboard (`/dashboard`) or API (`POST /api/approvals/:id/approve`)
4. If the timeout expires, the `timeout_effect` is applied

---

## Policy Evaluation Order

Policies are evaluated in the order they appear in the config file. Evaluation follows a pipeline:

1. **First pass**: All policies are evaluated in order
2. **Short-circuit on deny/terminate**: The first `deny` or `terminate` stops evaluation
3. **Short-circuit on approve**: The first `approve` stops evaluation and parks the request
4. **Throttle accumulation**: All `throttle` results are collected; the longest delay wins
5. **Default allow**: If no policy fires, the action is allowed

**Best practice**: Place your most important policies first:

```yaml
policies:
  # 1. Budget limits (terminate)
  - name: hard-budget
    condition: "session.cost > 10.0"
    effect: terminate
    message: "Budget exceeded"

  # 2. Security blocks (deny)
  - name: no-shell
    condition: 'action.type == "tool.call" && action.name == "shell_exec"'
    effect: deny
    message: "Shell blocked"

  # 3. Rate limits (throttle)
  - name: rate-limit
    condition: "session.action_count > 100"
    effect: throttle
    delay: 5s
    message: "Rate limited"

  # 4. Approval gates (approve)
  - name: db-approval
    condition: 'action.type == "db.query"'
    effect: approve
    message: "DB access requires approval"
```

---

## Common Policy Patterns

### 1. Cost Circuit Breaker

Kill sessions that spend too much:

```yaml
- name: cost-circuit-breaker
  condition: "session.cost > 5.0"
  effect: terminate
  message: "Session terminated: exceeded $5 budget"
```

### 2. Per-Agent Budget

Different budgets for different agents:

```yaml
- name: premium-agent-budget
  condition: 'agent.id == "premium-agent" && session.cost > 50.0'
  effect: terminate
  message: "Premium agent budget exceeded"

- name: default-agent-budget
  condition: "session.cost > 5.0"
  effect: terminate
  message: "Default agent budget exceeded"
```

### 3. Block Expensive Models

Prevent agents from using costly models:

```yaml
- name: no-opus
  condition: 'action.type == "llm.chat" && action.name.contains("opus")'
  effect: deny
  message: "Claude Opus models are restricted. Use Sonnet or Haiku."
```

### 4. Action Count Rate Limit

Cap the total number of actions in a session:

```yaml
- name: max-actions
  condition: "session.action_count > 500"
  effect: terminate
  message: "Session exceeded 500 actions"
```

### 5. Block All Tool Calls

Lock down to LLM-only mode:

```yaml
- name: llm-only
  condition: 'action.type == "tool.call" || action.type == "code.exec"'
  effect: deny
  message: "Only LLM chat is allowed in this environment"
```

### 6. Block Specific MCP Tools

```yaml
- name: no-dangerous-mcp
  condition: >
    action.type == "mcp.tool"
    && action.name in ["file_write", "shell_exec", "browser_navigate"]
  effect: deny
  message: "This MCP tool is not allowed"
```

### 7. Production Database Protection

```yaml
- name: no-prod-writes
  condition: >
    action.type == "db.query"
    && action.target.contains("production")
  effect: deny
  message: "Production database access is blocked"
```

### 8. Throttle After Threshold

Slow down rather than block:

```yaml
- name: slow-down
  condition: "session.action_count > 50"
  effect: throttle
  delay: 3s
  message: "Slowing down: over 50 actions"
```

### 9. Require Approval for File Writes

```yaml
- name: file-write-approval
  condition: 'action.type == "file.write"'
  effect: approve
  message: "File writes require human approval"
  approvers: ["security@example.com"]
  timeout: 5m
  timeout_effect: deny
```

### 10. Agent-Specific Allow List

Allow a trusted agent to bypass a general block:

```yaml
# Allow trusted agent to use shell
- name: trusted-shell-allow
  condition: 'agent.id == "deploy-bot" && action.type == "tool.call" && action.name == "shell_exec"'
  effect: allow
  message: "Deploy bot is allowed shell access"

# Block shell for everyone else
- name: block-shell
  condition: 'action.type == "tool.call" && action.name == "shell_exec"'
  effect: deny
  message: "Shell execution is blocked"
```

Place the `allow` policy before the `deny` policy. Since evaluation short-circuits, the trusted agent matches the first rule and the deny rule is never reached.
