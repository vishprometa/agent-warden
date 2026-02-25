# Quick Start Guide

Get AgentWarden running in under five minutes. This guide covers installation, zero-config startup, adding your first policy, and integrating with Python and TypeScript agents.

## Table of Contents

- [Installation](#installation)
- [Zero-Config Quickstart](#zero-config-quickstart)
- [Your First Policy](#your-first-policy)
- [Using with Python](#using-with-python)
- [Using with TypeScript](#using-with-typescript)
- [Manual Integration (Any Language)](#manual-integration-any-language)
- [Next Steps](#next-steps)

---

## Installation

### Docker (recommended)

```bash
docker run -p 6777:6777 -v ./data:/data ghcr.io/agentwarden/agentwarden
```

### Docker Compose

```bash
curl -O https://raw.githubusercontent.com/agentwarden/agentwarden/main/docker-compose.yml
docker compose up -d
```

### Homebrew (macOS / Linux)

```bash
brew tap agentwarden/tap
brew install agentwarden
```

### Pre-built Binary

Download the latest release for your platform from the [GitHub Releases](https://github.com/agentwarden/agentwarden/releases) page. Extract the binary and place it in your PATH:

```bash
# macOS (Apple Silicon)
curl -L https://github.com/agentwarden/agentwarden/releases/latest/download/agentwarden_darwin_arm64.tar.gz | tar xz
sudo mv agentwarden /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/agentwarden/agentwarden/releases/latest/download/agentwarden_linux_amd64.tar.gz | tar xz
sudo mv agentwarden /usr/local/bin/
```

### Go Install

Requires Go 1.22+ and a C compiler (CGO is needed for SQLite):

```bash
go install github.com/agentwarden/agentwarden/cmd/agentwarden@latest
```

---

## Zero-Config Quickstart

AgentWarden ships with sensible defaults. You can start it with no configuration at all:

```bash
# Start the proxy and dashboard
agentwarden start
```

That is it. AgentWarden is now:

- Listening on **port 6777** as an HTTP reverse proxy
- Serving the **monitoring dashboard** at `http://localhost:6777/dashboard`
- Serving the **management API** at `http://localhost:6777/api`
- Storing traces in a local **SQLite database** (`./agentwarden.db`)
- Running **loop detection**, **cost anomaly detection**, and **spiral detection** with default thresholds

### Point Your Agent at the Proxy

Set the base URL for your LLM client to the AgentWarden proxy:

```bash
export OPENAI_BASE_URL=http://localhost:6777/v1
```

Now run your agent normally. Every LLM API call flows through AgentWarden, which classifies it, evaluates policies, tracks cost, checks for anomalies, and records an immutable trace.

### Open the Dashboard

Open `http://localhost:6777/dashboard` in your browser to see live traces, session summaries, cost breakdowns, and agent activity as your agent runs.

### Generate a Starter Config

For customization, generate a starter configuration file:

```bash
agentwarden init
```

This creates `agentwarden.yaml` in the current directory with commented defaults. Edit it, then start with:

```bash
agentwarden start --config agentwarden.yaml
```

---

## Your First Policy

Add a cost circuit breaker to kill runaway sessions. Create or edit `agentwarden.yaml`:

```yaml
server:
  port: 6777
  dashboard: true

upstream:
  default: https://api.openai.com/v1

policies:
  - name: cost-circuit-breaker
    condition: "session.cost > 2.00"
    effect: terminate
    message: "Session killed: exceeded $2.00 budget"
```

Start AgentWarden:

```bash
agentwarden start
```

Now if any agent session accumulates more than $2.00 in LLM costs, the session is terminated and subsequent requests return an error. The violation is logged, visible in the dashboard, and triggers alerts if configured.

### Validate Your Config

Before starting, check that your policy expressions are valid:

```bash
agentwarden policy validate
```

### Hot-Reload

Edit `agentwarden.yaml` while the proxy is running. Changes take effect automatically (file-watch based hot-reload). You can also trigger a manual reload:

```bash
agentwarden policy reload
```

---

## Using with Python

### Install the SDK

```bash
pip install agentwarden
```

### Wrap OpenAI

The SDK wraps your OpenAI client, automatically routing requests through the proxy and attaching agent/session headers:

```python
from agentwarden import AgentWarden
from openai import OpenAI

# Initialize the warden with your agent identity
warden = AgentWarden(
    agent_id="research-agent",
    warden_url="http://localhost:6777",  # default
)

# Wrap the OpenAI client -- all requests now flow through AgentWarden
client = warden.wrap_openai(OpenAI())

# Use the client as normal
response = client.chat.completions.create(
    model="gpt-4o",
    messages=[{"role": "user", "content": "Summarize the latest AI safety research"}],
)

print(response.choices[0].message.content)
```

### Session Management

```python
# Start a named session for grouped tracing
with warden.session(metadata={"task": "research"}) as session:
    client = session.wrap_openai(OpenAI())

    # All requests within this block share the same session ID
    response = client.chat.completions.create(
        model="gpt-4o",
        messages=[{"role": "user", "content": "Hello"}],
    )
    # Session automatically ends when the block exits
```

---

## Using with TypeScript

### Install the SDK

```bash
npm install @agentwarden/sdk
```

### Wrap OpenAI

```typescript
import { AgentWarden } from '@agentwarden/sdk';
import OpenAI from 'openai';

// Initialize the warden
const warden = new AgentWarden({
  agentId: 'research-agent',
  wardenUrl: 'http://localhost:6777', // default
});

// Wrap the OpenAI client
const client = warden.wrapOpenAI(new OpenAI());

// Use the client as normal
const response = await client.chat.completions.create({
  model: 'gpt-4o',
  messages: [{ role: 'user', content: 'Summarize the latest AI safety research' }],
});

console.log(response.choices[0].message.content);
```

### Session Management

```typescript
// Start a tracked session
const session = warden.startSession({ task: 'research' });
const client = session.wrapOpenAI(new OpenAI());

try {
  const response = await client.chat.completions.create({
    model: 'gpt-4o',
    messages: [{ role: 'user', content: 'Hello' }],
  });
} finally {
  await session.end();
}
```

---

## Manual Integration (Any Language)

If you do not want to use an SDK, point your HTTP client at `http://localhost:6777` and add these headers to every request:

| Header | Required | Description |
|--------|----------|-------------|
| `X-AgentWarden-Agent-Id` | Recommended | Identifies your agent (defaults to `anonymous`) |
| `X-AgentWarden-Session-Id` | Optional | Groups related requests into a session. If omitted, a new session is created per request. |
| `X-AgentWarden-Metadata` | Optional | JSON object with arbitrary metadata attached to the session |

Example with curl:

```bash
curl http://localhost:6777/v1/chat/completions \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "X-AgentWarden-Agent-Id: my-agent" \
  -H "X-AgentWarden-Session-Id: ses_abc123" \
  -H 'X-AgentWarden-Metadata: {"task": "research"}' \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

AgentWarden strips the `X-AgentWarden-*` headers before forwarding the request upstream, so the LLM provider never sees them.

The response includes a trace ID header:

```
X-AgentWarden-Trace-Id: 01HQZX7ABC123DEF456
```

---

## Using with OpenClaw

AgentWarden can govern autonomous OpenClaw agents via a transparent WebSocket reverse proxy. No changes to OpenClaw are needed -- just point your agents at AgentWarden.

```bash
# 1. Configure the adapter in agentwarden.yaml
# 2. Start AgentWarden
agentwarden start

# 3. Point OpenClaw to AgentWarden (just change 2 env vars)
OPENCLAW_GATEWAY_URL=ws://localhost:6777/gateway
OPENAI_API_BASE=http://localhost:6777
```

AgentWarden adds kill switches, capability scoping, spawn governance, skill vetting, message governance, safety invariants, and prompt injection defense on top of the standard policy engine.

For the full setup guide, see [OpenClaw Integration](openclaw.md).

---

## Next Steps

- [Configuration Reference](configuration.md) -- Full reference for `agentwarden.yaml`
- [Policy Authoring Guide](policies.md) -- Write CEL-based governance policies
- [OpenClaw Integration](openclaw.md) -- Govern autonomous OpenClaw agents
- [Architecture Deep Dive](architecture.md) -- Understand the interception pipeline
- [API Reference](api-reference.md) -- Management API endpoints
- [Self-Evolution Guide](evolution.md) -- Auto-tuning agent behavior
