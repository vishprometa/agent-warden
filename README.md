# AgentWarden

**Runtime governance for AI agents.** Observe. Enforce. Evolve.

[![Docs](https://img.shields.io/badge/docs-agentwarden--docs.vercel.app-6366f1)](https://agentwarden-docs.vercel.app)
[![CI](https://github.com/vishprometa/agent-warden/actions/workflows/ci.yml/badge.svg)](https://github.com/vishprometa/agent-warden/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

AgentWarden is a transparent HTTP proxy that sits between your AI agents and LLM providers, enforcing governance policies, tracking costs, detecting anomalies, and producing an immutable audit trail of every action.

```
┌─────────┐     ┌──────────────┐     ┌──────────────┐
│ AI Agent │────▶│  AgentWarden │────▶│  LLM Provider │
│          │◀────│    :6777     │◀────│  (OpenAI etc) │
└─────────┘     └──────────────┘     └──────────────┘
                       │
                 ┌─────┴─────┐
                 │ Dashboard  │
                 │ Traces DB  │
                 │ Alerts     │
                 └───────────┘
```

## Features

- **Transparent Proxy** — Drop-in replacement for OpenAI/Anthropic/Gemini base URLs. No agent code changes required.
- **Policy Engine** — CEL-based rules for budget limits, rate limiting, action blocking, and human approval gates.
- **Cost Tracking** — Per-session and per-agent token counting and USD cost calculation for 20+ models.
- **Anomaly Detection** — Automatic detection of action loops, cost velocity spikes, and conversation spirals.
- **Immutable Audit Trail** — Every action is traced with SHA-256 hash chain integrity verification.
- **Real-time Dashboard** — React-based monitoring UI with live WebSocket trace feed, embedded in the binary.
- **Alerts** — Slack and generic webhook notifications with deduplication.
- **Human Approval Queue** — Park high-risk actions for human review before execution.
- **Hot-Reload** — Policy changes take effect without restarting the proxy.
- **Single Binary** — Everything (proxy, dashboard, API, SQLite) ships as one ~26MB binary.

## Quick Start

```bash
# Install
go install github.com/agentwarden/agentwarden/cmd/agentwarden@latest

# Generate starter config
agentwarden init

# Start the proxy + dashboard
agentwarden start --dev

# Point your agent at the proxy
export OPENAI_BASE_URL=http://localhost:6777/v1
```

### Docker

```bash
docker run -p 6777:6777 -v ./data:/data ghcr.io/agentwarden/agentwarden
```

## Configuration

Copy `agentwarden.example.yaml` to `agentwarden.yaml` and edit:

```yaml
server:
  port: 6777
  dashboard: true

upstream:
  default: "https://api.openai.com"
  routes:
    - prefix: "gpt-"
      url: "https://api.openai.com"
    - prefix: "claude-"
      url: "https://api.anthropic.com"

policies:
  - name: session-budget
    condition: "session.cost > 5.0"
    effect: terminate
    message: "Session exceeded $5 budget"

  - name: block-shell
    condition: 'action.type == "tool.call" && action.name == "shell_exec"'
    effect: deny
    message: "Shell execution blocked"

detection:
  loop:
    enabled: true
    threshold: 5
    window: 60s
```

Environment variable substitution is supported: `${VAR_NAME:-default}`.

## SDK Usage

### Python

```bash
pip install agentwarden
```

```python
from agentwarden import AgentWarden
from openai import OpenAI

warden = AgentWarden(agent_id="my-agent")
client = warden.wrap_openai(OpenAI())

response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello"}]
)
```

### TypeScript

```bash
npm install @agentwarden/sdk
```

```typescript
import { AgentWarden } from '@agentwarden/sdk';
import OpenAI from 'openai';

const warden = new AgentWarden({ agentId: 'my-agent' });
const client = warden.wrapOpenAI(new OpenAI());

const response = await client.chat.completions.create({
  model: 'gpt-4',
  messages: [{ role: 'user', content: 'Hello' }],
});
```

### Manual (any language)

Point your HTTP client at `http://localhost:6777` and add these headers:

```
X-AgentWarden-Agent-Id: my-agent
X-AgentWarden-Session-Id: ses_abc123  (optional)
X-AgentWarden-Metadata: {"task": "research"}  (optional JSON)
```

## Policy Engine

Policies use [CEL (Common Expression Language)](https://github.com/google/cel-go) conditions:

| Variable | Type | Description |
|----------|------|-------------|
| `action.type` | string | Action type: `llm.chat`, `tool.call`, `api.request`, `db.query`, `file.write`, `code.exec` |
| `action.name` | string | Action name (model, tool name, etc.) |
| `action.params` | map | Action-specific parameters |
| `action.target` | string | Resource being acted upon |
| `session.cost` | double | Accumulated session cost in USD |
| `session.action_count` | int | Total actions in the session |
| `agent.id` | string | Agent identifier |
| `agent.name` | string | Agent display name |

Effects: `allow`, `deny`, `terminate`, `throttle` (with delay), `approve` (human-in-the-loop).

## Management API

REST API at `http://localhost:6777/api/`:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/health` | Health check |
| GET | `/api/stats` | System statistics |
| GET | `/api/sessions` | List sessions |
| GET | `/api/sessions/:id` | Session detail with traces |
| DELETE | `/api/sessions/:id` | Terminate session |
| GET | `/api/traces` | List traces |
| GET | `/api/traces/search?q=` | Full-text trace search |
| GET | `/api/agents` | List agents |
| GET | `/api/agents/:id/stats` | Agent statistics |
| GET | `/api/policies` | List loaded policies |
| POST | `/api/policies/reload` | Hot-reload policies |
| GET | `/api/approvals` | Pending approvals |
| POST | `/api/approvals/:id/approve` | Approve action |
| POST | `/api/approvals/:id/deny` | Deny action |
| WS | `/api/ws/traces` | Live trace WebSocket feed |

## CLI

```bash
agentwarden start [--config FILE] [--port PORT] [--dev]
agentwarden init [PATH]
agentwarden status
agentwarden version
agentwarden policy validate [--config FILE]
agentwarden policy reload
agentwarden trace list [--agent ID]
agentwarden trace show SESSION_ID
agentwarden trace search QUERY
agentwarden trace verify SESSION_ID
agentwarden agent list
agentwarden doctor
agentwarden mock
```

## Architecture

```
cmd/agentwarden/        CLI entrypoint (Cobra)
internal/
  config/               YAML config loader with env var substitution
  proxy/                HTTP reverse proxy with interception pipeline
  policy/               CEL policy engine with budget, rate limit, approval
  detection/            Loop, cost anomaly, and spiral detection
  trace/                SQLite trace store with SHA-256 hash chain
  cost/                 Token counting and pricing for 20+ models
  session/              Session lifecycle management
  alert/                Slack + webhook alert dispatch with dedup
  approval/             Human approval queue with timeout
  api/                  REST management API + WebSocket hub
  dashboard/            Embedded React dashboard (go:embed)
dashboard/              React + Vite + Tailwind dashboard source
sdks/
  python/               Python SDK (agentwarden)
  typescript/           TypeScript SDK (@agentwarden/sdk)
```

## Development

```bash
# Build everything
make build

# Run in dev mode (verbose logs, CORS, hot-reload)
make dev

# Run tests
make test

# Build Docker image
make docker
```

## Documentation

Full documentation is available at **[agentwarden-docs.vercel.app](https://agentwarden-docs.vercel.app)**:

- [Quick Start Guide](https://agentwarden-docs.vercel.app/quickstart)
- [Configuration Reference](https://agentwarden-docs.vercel.app/configuration)
- [Policy Authoring](https://agentwarden-docs.vercel.app/policies)
- [Architecture Deep Dive](https://agentwarden-docs.vercel.app/architecture)
- [Self-Evolution Guide](https://agentwarden-docs.vercel.app/evolution)
- [API Reference](https://agentwarden-docs.vercel.app/api-reference)
- [Python SDK](https://agentwarden-docs.vercel.app/sdk-python)
- [TypeScript SDK](https://agentwarden-docs.vercel.app/sdk-typescript)

## License

MIT
