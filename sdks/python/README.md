# AgentWarden Python SDK

Python SDK for [AgentWarden](https://github.com/agentwarden/agentwarden) -- a runtime governance proxy for AI agents.

## Installation

```bash
pip install agentwarden
```

With OpenAI support:

```bash
pip install agentwarden[openai]
```

## Quick Start

### Wrap an OpenAI client

Route all LLM calls through the AgentWarden proxy for governance, tracing, and cost tracking:

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

### Session scoping

Group related actions into a session:

```python
warden = AgentWarden(agent_id="my-agent")
client = warden.wrap_openai(OpenAI())

with warden.session() as w:
    # All calls within this block share the same session ID
    client = w.wrap_openai(client)
    response = client.chat.completions.create(
        model="gpt-4",
        messages=[{"role": "user", "content": "Summarize this document"}]
    )
```

### Manual HTTP integration

For non-OpenAI clients, get the proxy URL and headers directly:

```python
import httpx
from agentwarden import AgentWarden

warden = AgentWarden(agent_id="my-agent")

response = httpx.post(
    f"{warden.get_base_url()}/chat/completions",
    headers={
        "Authorization": "Bearer sk-...",
        **warden.get_headers(),
    },
    json={
        "model": "gpt-4",
        "messages": [{"role": "user", "content": "Hello"}],
    },
)
```

### Management API

Inspect traces, sessions, and manage policies:

```python
from agentwarden import ManagementClient

mgmt = ManagementClient(base_url="http://localhost:6777")

# Health check
print(mgmt.health())

# List recent sessions
sessions = mgmt.list_sessions(limit=10)

# Inspect a session's traces
detail = mgmt.get_session("ses_abc123")

# Search traces
results = mgmt.search_traces("error", limit=20)

# View and reload policies
policies = mgmt.list_policies()
mgmt.reload_policies()

# Handle approvals
pending = mgmt.list_approvals()
if pending.get("approvals"):
    mgmt.approve(pending["approvals"][0]["id"])
```

### Typed models

Parse API responses into Pydantic models:

```python
from agentwarden import ManagementClient, Trace, Session
from agentwarden.models import PaginatedTraces

mgmt = ManagementClient()

data = mgmt.list_traces(agent_id="my-agent", limit=10)
result = PaginatedTraces(**data)

for trace in result.traces:
    print(f"{trace.action_type}: {trace.status} ({trace.latency_ms}ms)")
```

## Configuration

| Parameter    | Default                  | Description                        |
|--------------|--------------------------|------------------------------------|
| `proxy_url`  | `http://localhost:6777`  | AgentWarden proxy base URL         |
| `agent_id`   | `""`                     | Agent identifier for governance    |
| `session_id` | `None`                   | Optional session grouping          |
| `metadata`   | `None`                   | Key-value metadata for requests    |

## Requirements

- Python >= 3.9
- `httpx` >= 0.24
- `pydantic` >= 2.0
- `openai` >= 1.0 (optional, for `wrap_openai`)
