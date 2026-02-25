---
layout: home

hero:
  name: AgentWarden
  text: Runtime Governance for AI Agents
  tagline: Observe. Enforce. Evolve. — A lightweight sidecar that governs your AI agents in real-time.
  actions:
    - theme: brand
      text: Get Started
      link: /quickstart
    - theme: alt
      text: View on GitHub
      link: https://github.com/vishprometa/agent-warden

features:
  - icon: 🛡️
    title: Policy Engine
    details: CEL-based rules for budget limits, rate limiting, action blocking, and human approval gates. Hot-reload without restarts.
  - icon: 💰
    title: Cost Tracking
    details: Per-session and per-agent token counting with USD cost calculation for 20+ models. Set hard limits per session or globally.
  - icon: 🔄
    title: Loop Detection
    details: Automatic detection of action loops, cost velocity spikes, and conversation spirals before they waste budget.
  - icon: 📊
    title: Real-time Dashboard
    details: React-based monitoring UI with live WebSocket trace feed, embedded directly in the single binary.
  - icon: 🔗
    title: Immutable Audit Trail
    details: Every action traced with SHA-256 hash chain integrity. Session-scoped traces for complete agent lifecycle visibility.
  - icon: 🧬
    title: Self-Evolution
    details: Python sidecar observes agent behavior, identifies failure patterns, and proposes configuration improvements via shadow testing.
---

## How It Works

AgentWarden is an **event-driven governance sidecar** — not a proxy. Your AI agent SDKs send events to AgentWarden, which evaluates policies, tracks costs, detects anomalies, and maintains an immutable audit trail.

```
┌─────────┐     ┌──────────────┐     ┌──────────────┐
│ AI Agent │────▶│  AgentWarden │     │  LLM Provider │
│   SDK    │◀────│    :6777     │     │  (OpenAI etc) │
└─────────┘     └──────────────┘     └──────────────┘
                       │
                 ┌─────┴─────┐
                 │ Dashboard  │
                 │ Traces DB  │
                 │ Alerts     │
                 └───────────┘
```

## Quick Install

```bash
# Go install
go install github.com/agentwarden/agentwarden/cmd/agentwarden@latest

# Or Docker
docker run -p 6777:6777 ghcr.io/agentwarden/agentwarden

# Start with dev mode
agentwarden start --dev
```

## SDKs

AgentWarden provides official SDKs for Python and TypeScript:

```python
# Python
from agentwarden import AgentWarden

warden = AgentWarden("http://localhost:6777")
session = warden.session(agent_id="support-bot")

# Every tool call is governed
result = session.tool("search_knowledge", {"query": "pricing"})
```

```typescript
// TypeScript
import { AgentWarden } from '@agentwarden/sdk'

const warden = new AgentWarden({ baseUrl: 'http://localhost:6777' })
const session = await warden.session({ agentId: 'support-bot' })

const result = await session.tool('search_knowledge', { query: 'pricing' })
```
