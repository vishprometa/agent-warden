---
layout: home

hero:
  name: AgentWarden
  text: Runtime Governance for AI Agents
  tagline: The lightweight proxy that keeps your AI agents safe, auditable, and under control.
  image:
    src: /logo.svg
    alt: AgentWarden
  actions:
    - theme: brand
      text: Get Started
      link: /quickstart
    - theme: alt
      text: OpenClaw Integration
      link: /openclaw
    - theme: alt
      text: GitHub
      link: https://github.com/vishprometa/agent-warden

features:
  - title: Kill Switch
    details: Emergency stop that operates outside the LLM context window. Cannot be bypassed by prompt injection or context compaction.
    link: /openclaw#kill-switch
    linkText: Learn more
  - title: Capability Scoping
    details: Per-agent boundaries for filesystem, shell, network, messaging, and financial actions. Enforced at the proxy level.
    link: /openclaw#capability-scoping
    linkText: Learn more
  - title: Cost Tracking & Budgets
    details: Per-session and per-agent cost tracking with hard limits. Supports 20+ models from OpenAI, Anthropic, Google, and more.
    link: /policies#budget-policies
    linkText: Learn more
  - title: Spawn Governance
    details: Control agent self-replication with depth limits, child counts, budget inheritance, and cascade kill.
    link: /openclaw#spawn-governance
    linkText: Learn more
  - title: Anomaly Detection
    details: Automatic detection of action loops, cost velocity spikes, conversation spirals, and rapid-fire runaway behavior.
    link: /configuration#detection
    linkText: Learn more
  - title: Policy Engine
    details: CEL-based rules for budget limits, rate limiting, action blocking, and human approval gates. Hot-reload without restarts.
    link: /policies
    linkText: Learn more
  - title: Prompt Injection Defense
    details: Detects role confusion, authority impersonation, hidden instructions, and data exfiltration patterns in LLM inputs.
    link: /openclaw#prompt-injection-defense
    linkText: Learn more
  - title: Immutable Audit Trail
    details: Every action traced with SHA-256 hash chain integrity. Session-scoped traces for complete agent lifecycle visibility.
    link: /api-reference#traces
    linkText: Learn more
---

<div class="home-content">

## How It Works

AgentWarden sits between your AI agents and the outside world as a transparent proxy. Every action is evaluated, traced, and governed.

```
Agent ──> AgentWarden ──> LLM API / Tools / APIs
              │
              ├── Kill Switch    (hard stop)
              ├── Capabilities   (scope checks)
              ├── Policy Engine  (CEL rules)
              ├── Detection      (loops, velocity, spirals)
              ├── Audit Trail    (hash-chained traces)
              └── Dashboard      (real-time visibility)
```

### Zero-modification integration

For **OpenClaw** agents, change two environment variables:

```bash
OPENCLAW_GATEWAY_URL=ws://localhost:6777/gateway
OPENAI_API_BASE=http://localhost:6777
```

For **any other agent**, point your LLM client at the proxy:

```bash
export OPENAI_BASE_URL=http://localhost:6777/v1
```

## Quick Install

```bash
# Homebrew
brew tap agentwarden/tap && brew install agentwarden

# Or Docker
docker run -p 6777:6777 ghcr.io/agentwarden/agentwarden

# Start with dev mode
agentwarden start --dev
```

Open the dashboard at `http://localhost:6777/dashboard` to see live traces, sessions, costs, and agent activity.

</div>
