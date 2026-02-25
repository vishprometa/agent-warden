# TypeScript SDK

TypeScript SDK for AgentWarden — runtime governance for AI agents.

- **Zero dependencies** — uses native `fetch`
- **OpenAI SDK integration** — one-line wrapper to route through the proxy
- **Management API client** — sessions, traces, policies, approvals
- **Full TypeScript** — strict types for all models and API responses

## Installation

```bash
npm install @agentwarden/sdk
```

## Quick Start

### Wrap an OpenAI client

```typescript
import { AgentWarden } from '@agentwarden/sdk'
import OpenAI from 'openai'

const warden = new AgentWarden({ agentId: 'my-agent' })
const client = warden.wrapOpenAI(new OpenAI())

// All requests now flow through the AgentWarden proxy.
// Policies are evaluated, costs are tracked, traces are recorded.
const response = await client.chat.completions.create({
  model: 'gpt-4',
  messages: [{ role: 'user', content: 'Hello' }],
})
```

### Manual Header Injection

For HTTP clients that don't follow the OpenAI SDK pattern:

```typescript
const warden = new AgentWarden({
  agentId: 'my-agent',
  sessionId: 'session-123',
  metadata: { environment: 'production' },
})

const response = await fetch(`${warden.getBaseUrl()}/chat/completions`, {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer sk-...',
    ...warden.getHeaders(),
  },
  body: JSON.stringify({
    model: 'gpt-4',
    messages: [{ role: 'user', content: 'Hello' }],
  }),
})
```

### Session Scoping

```typescript
const warden = new AgentWarden({ agentId: 'my-agent' })

// Create a session-scoped instance
const scoped = warden.withSession('task-abc-123')
const client = scoped.wrapOpenAI(new OpenAI())

// All requests share the same session
await client.chat.completions.create({ /* ... */ })
await client.chat.completions.create({ /* ... */ })
```

### Management API

```typescript
import { ManagementClient } from '@agentwarden/sdk/management'

const mgmt = new ManagementClient('http://localhost:6777')

// System health
const health = await mgmt.health()

// List active sessions
const { sessions, total } = await mgmt.listSessions({ status: 'active' })

// Inspect a session and its traces
const { session, traces } = await mgmt.getSession('session-id')

// Search traces
const results = await mgmt.searchTraces('gpt-4')

// Policy management
const { policies } = await mgmt.listPolicies()
await mgmt.reloadPolicies()

// Human-in-the-loop approvals
const { approvals } = await mgmt.listApprovals()
await mgmt.approve('approval-id')
await mgmt.deny('approval-id')
```

## Configuration

```typescript
const warden = new AgentWarden({
  // AgentWarden proxy URL (default: http://localhost:6777)
  proxyUrl: 'http://localhost:6777',

  // Required: identifies this agent in traces and policies
  agentId: 'my-agent',

  // Optional: group requests into a session
  sessionId: 'session-123',

  // Optional: arbitrary metadata attached to every request
  metadata: {
    environment: 'staging',
    version: '1.2.0',
  },
})
```

## Headers

The SDK injects these headers into proxied requests:

| Header | Description |
|--------|-------------|
| `X-AgentWarden-Agent-Id` | Agent identifier |
| `X-AgentWarden-Session-Id` | Session identifier (optional) |
| `X-AgentWarden-Metadata` | JSON-encoded metadata (optional) |

The proxy returns:

| Header | Description |
|--------|-------------|
| `X-AgentWarden-Trace-Id` | Unique trace ID for the intercepted request |

## Error Handling

Management API errors throw `AgentWardenError`:

```typescript
import { AgentWardenError } from '@agentwarden/sdk'

try {
  await mgmt.getSession('nonexistent')
} catch (err) {
  if (err instanceof AgentWardenError) {
    console.error(err.status) // 404
    console.error(err.body)   // { error: "session not found" }
  }
}
```

## License

MIT
