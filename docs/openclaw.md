# OpenClaw Integration

Govern autonomous OpenClaw agents with AgentWarden. No OpenClaw code changes required -- AgentWarden acts as a transparent reverse proxy between your agents and the OpenClaw gateway.

## Table of Contents

- [Why Governance for OpenClaw](#why-governance-for-openclaw)
- [Architecture](#architecture)
- [Quick Start (5 minutes)](#quick-start-5-minutes)
- [Kill Switch](#kill-switch)
- [Capability Scoping](#capability-scoping)
- [Spawn Governance](#spawn-governance)
- [Skill Governance](#skill-governance)
- [Message Governance](#message-governance)
- [Safety Invariants](#safety-invariants)
- [Prompt Injection Defense](#prompt-injection-defense)
- [Velocity Detection](#velocity-detection)
- [Pre-Built Policy Packs](#pre-built-policy-packs)
- [API Endpoints](#api-endpoints)

---

## Why Governance for OpenClaw

OpenClaw is a powerful autonomous agent framework, but its power comes with risk:

- **Runaway agents**: Agents can burn through $200+/day in uncontrolled automation loops
- **Context compaction safety loss**: Safety instructions can be dropped when context is compacted, causing agents to ignore STOP commands
- **Excessive permissions**: Agents get shell access, file system access, and network access with no boundaries
- **Malicious skills**: 824+ malicious skills have been distributed through ClawHub
- **Uncontrolled messaging**: Agents can send messages across WhatsApp, Slack, Discord, and email without oversight
- **Agent self-replication**: Agents can spawn child agents and fund them with Bitcoin

AgentWarden addresses every one of these with enforceable, proxy-level controls.

---

## Architecture

AgentWarden sits between OpenClaw agents and the OpenClaw gateway as a transparent WebSocket reverse proxy. Agents connect to AgentWarden instead of the gateway directly -- no code changes needed.

```
BEFORE (no governance):
  Agent --> OpenClaw Gateway --> LLM API

AFTER (with AgentWarden):
  Agent --> AgentWarden (6777) --> OpenClaw Gateway
                 |
                 +-- Kill Switch (hard stop)
                 +-- Capability Engine (scope checks)
                 +-- Policy Engine (CEL rules)
                 +-- Detection Engine (loops, velocity, spirals)
                 +-- Trace Store (audit trail)
                 +-- Dashboard (visibility)
                 |
            AgentWarden (6777) --> LLM API
```

AgentWarden translates OpenClaw's native events into governance actions:

| OpenClaw Event | AgentWarden Action Type |
|---------------|------------------------|
| `tool.shell_exec` | `tool.call` (name: "shell_exec") |
| `tool.file_write` | `file.write` |
| `tool.file_read` | `file.read` |
| `skill.install` | `skill.install` |
| `skill.invoke` | `skill.invoke` |
| `agent.spawn` | `agent.spawn` |
| `agent.fund` | `financial.transfer` |
| `message.send` | `message.send` |
| `config.modify` | `config.change` |

---

## Quick Start (5 minutes)

### 1. Install AgentWarden

```bash
brew install agentwarden
```

### 2. Configure the OpenClaw adapter

Create `agentwarden.yaml`:

```yaml
server:
  port: 6777
  dashboard: true

adapters:
  openclaw:
    enabled: true
    mode: inline
    gateway_url: ws://localhost:4000
    auth_token: ${OPENCLAW_TOKEN}
    intercept:
      - tool_calls
      - skill_installs
      - message_sends
      - agent_spawns
      - financial_transfers
      - config_changes

policies:
  - name: session-budget
    condition: "session.cost > 10.00"
    effect: terminate
    message: "Session killed: exceeded $10 budget"

  - name: no-shell-exec
    condition: 'action.type == "tool.call" && action.name == "shell_exec"'
    effect: deny
    message: "Shell execution blocked by policy"
```

### 3. Start AgentWarden

```bash
agentwarden start
```

### 4. Point OpenClaw to AgentWarden

In your OpenClaw `.env` or config:

```bash
OPENCLAW_GATEWAY_URL=ws://localhost:6777/gateway
OPENAI_API_BASE=http://localhost:6777
```

Restart OpenClaw. Done. All agent actions now flow through AgentWarden's governance pipeline.

---

## Kill Switch

An emergency stop mechanism that operates **outside the LLM's context window**. When triggered, it immediately blocks all agent actions at the proxy level -- it cannot be bypassed by context compaction, prompt injection, or any other mechanism.

### Trigger via CLI

```bash
# Kill everything
agentwarden kill --all

# Kill a specific agent
agentwarden kill agent-id

# Kill a specific session
agentwarden kill --session session-id
```

### Trigger via API

```bash
# Global kill
curl -X POST http://localhost:6777/api/killswitch/trigger \
  -H "Content-Type: application/json" \
  -d '{"scope": "global", "reason": "runaway agent", "source": "api"}'

# Agent-level kill
curl -X POST http://localhost:6777/api/killswitch/trigger \
  -H "Content-Type: application/json" \
  -d '{"scope": "agent", "target_id": "agent-1", "reason": "cost exceeded", "source": "api"}'
```

The `source` field tracks how the kill switch was triggered. Valid values: `api`, `cli`, `dashboard`, `slack`, `file`.

### Trigger via file sentinel

Create a KILL file to trigger a global stop:

```bash
touch ~/.agentwarden/KILL
```

AgentWarden checks for this file every second. Remove it and call the reset API to resume.

### Reset

```bash
curl -X POST http://localhost:6777/api/killswitch/reset
```

---

## Capability Scoping

Define per-agent boundaries that restrict what actions an agent can perform. These are enforced at the proxy level and cannot be exceeded regardless of LLM output.

```yaml
agents:
  support-bot:
    capabilities:
      filesystem:
        allowed_paths: ["/data/support/**"]
        denied_paths: ["/etc/**", "/root/**", "~/.ssh/**"]
        read_only: false
      network:
        allowed_domains: ["api.example.com", "slack.com"]
        blocked_ports: [22, 3306, 5432]
      shell:
        enabled: true
        allowed_commands: ["curl", "jq", "grep"]
        blocked_commands: ["rm", "sudo", "chmod", "dd"]
        blocked_patterns: ["rm -rf", "| sh", "> /dev/"]
      messaging:
        allowed_channels: ["#support"]
        max_messages_per_hour: 50
      financial:
        max_transaction: 0  # disabled
      spawn:
        enabled: false
```

When an agent attempts an action outside its capability scope, the action is immediately denied before it reaches any policy evaluation.

---

## Spawn Governance

Control agent self-replication with depth limits, child counts, budget inheritance, and cascade kill.

```yaml
spawn:
  enabled: true
  max_children_per_agent: 3
  max_depth: 2
  max_global_agents: 20
  inherit_capabilities: true
  require_approval: false
  cascade_kill: true
  child_budget_max: 0.5  # children get max 50% of parent budget
```

| Setting | Description |
|---------|-------------|
| `max_children_per_agent` | Maximum child agents a single parent can spawn |
| `max_depth` | Maximum nesting depth (root=0, child=1, grandchild=2) |
| `max_global_agents` | Hard cap on total concurrent agents |
| `cascade_kill` | Killing a parent automatically kills all descendants |
| `child_budget_max` | Children get at most this fraction of parent's remaining budget |

---

## Skill Governance

Vet ClawHub skills before installation with allowlists, blocklists, and static analysis.

```yaml
skills:
  governance:
    mode: allowlist   # allowlist | blocklist | scan | open
    allowlist: ["official/*", "verified/*"]
    blocklist: ["*crypto-trader*", "*wallet-manager*"]
    require_approval: true
    scan:
      enabled: true
      suspicious_patterns:
        - "eval("
        - "child_process"
        - "fs.readFile"
        - "process.env"
```

| Mode | Behavior |
|------|----------|
| `allowlist` | Only explicitly allowed skills can install |
| `blocklist` | Everything except blocked skills |
| `scan` | All skills are scanned for suspicious patterns |
| `open` | No restrictions (not recommended) |

The scanner checks skill code for credential access patterns, obfuscated code, outbound HTTP to unknown domains, and other indicators of malicious intent.

---

## Message Governance

Control outbound messages across all channels with rate limits, content scanning, and approval gates.

```yaml
messaging:
  require_approval:
    external: true
    mass: true
  rate_limits:
    whatsapp: 10/hour
    slack: 50/hour
    email: 5/hour
  content_scan:
    block_pii: true
    block_credentials: true
```

The content scanner detects API keys (OpenAI, AWS, GitHub, Stripe, Slack, GitLab patterns), private keys, and PII patterns like SSNs in outbound messages.

---

## Safety Invariants

Safety invariants survive context compaction. They are stored outside the LLM context in AgentWarden's own state and enforced at the proxy level on every action.

```yaml
agents:
  email-manager:
    safety_invariants:
      - description: "NEVER delete more than 5 emails per session without approval"
        condition: "session.action_count('email.delete') > 5"
        effect: deny
        enforcement: proxy

      - description: "STOP immediately when user sends STOP"
        enforcement: inject

      - description: "NEVER access emails older than 30 days without approval"
        enforcement: both
```

| Enforcement | How It Works |
|-------------|-------------|
| `proxy` | CEL condition enforced at AgentWarden (cannot be bypassed by context compaction) |
| `inject` | Description re-injected into LLM context on every request |
| `both` | Both proxy enforcement and context injection |

---

## Prompt Injection Defense

Scan LLM inputs for known injection patterns and flag or block suspicious content.

```yaml
sanitize:
  enabled: true
  mode: flag   # flag | warn | deny
```

The scanner detects:
- Role confusion attempts ("ignore previous instructions", "you are now", "system:")
- Authority impersonation ("admin says", "anthropic instructs")
- Hidden text patterns (zero-width characters, base64 encoded instructions)
- Data exfiltration patterns ("send credentials to")
- Action directives in data fields ("execute the following command")

Detection results are recorded in the trace store and trigger alerts.

---

## Velocity Detection

Detects rapid-fire actions that suggest a runaway agent. Unlike loop detection (which catches repeated identical actions), velocity detection catches diverse rapid actions.

```yaml
detection:
  velocity:
    enabled: true
    threshold: 10       # actions per second
    sustained_seconds: 5 # must exceed for this long
    action: pause        # pause | alert | terminate
```

When an agent fires more than `threshold` actions per second for `sustained_seconds` seconds, AgentWarden triggers the configured action.

---

## Pre-Built Policy Packs

AgentWarden ships with ready-to-use policy templates for OpenClaw:

### `safety-basic.yaml` (recommended starter)

Budget limits ($10/session, $100/day), shell/sudo/rm-rf blocks, rate limiting, SSH/env file protection, velocity detection.

### `safety-strict.yaml` (maximum governance)

Tight budgets ($5/$25), human approval for all tool calls and file writes, block spawning and financial transfers, aggressive detection.

### `email-safe.yaml` (inspired by the Summer Yue incident)

Email deletion limits (>5 requires approval, >10 blocked), send rate limits, forwarding approval, attachment download approval.

Apply a policy pack:

```bash
cp policies/openclaw/safety-basic.yaml ./policies/
agentwarden start
```

Or reference it in your config:

```yaml
policy_files:
  - ./policies/openclaw/safety-basic.yaml
  - ./policies/custom-rules.yaml
```

---

## API Endpoints

### Kill Switch

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/killswitch/trigger` | Trigger kill switch (body: `{"scope": "global\|agent\|session", "target_id": "...", "reason": "...", "source": "api\|cli\|dashboard\|slack\|file"}`) |
| `GET` | `/api/killswitch/status` | Get current kill switch state |
| `POST` | `/api/killswitch/reset` | Reset kill switch (body: `{"scope": "global\|agent\|session", "target_id": "..."}`) |

### Kill Switch CLI

```bash
agentwarden kill --all               # Global kill
agentwarden kill <agent-id>          # Kill specific agent
agentwarden kill --session <sess-id> # Kill specific session
```
