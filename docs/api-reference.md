# API Reference

AgentWarden exposes a REST management API and a WebSocket endpoint for real-time trace streaming. All endpoints are served on the same port as the proxy (default: 6777) under the `/api/` prefix.

**Base URL:** `http://localhost:6777/api`

## Table of Contents

- [Authentication](#authentication)
- [Response Format](#response-format)
- [Sessions](#sessions)
- [Traces](#traces)
- [Agents](#agents)
- [Policies](#policies)
- [Approvals](#approvals)
- [Violations](#violations)
- [Kill Switch](#kill-switch)
- [System](#system)
- [WebSocket](#websocket)

---

## Authentication

The management API does not require authentication by default. In production deployments, use network-level controls (firewall, VPN, Kubernetes NetworkPolicy) to restrict access to the `/api/` path.

---

## Response Format

All endpoints return JSON. Successful responses have a 200 status code. Error responses use appropriate HTTP status codes with this format:

```json
{
  "error": "human-readable error message"
}
```

---

## Sessions

### List Sessions

```
GET /api/sessions
```

Returns a paginated list of sessions.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `agent_id` | string | | Filter by agent ID |
| `status` | string | | Filter by status: `active`, `completed`, `terminated`, `paused` |
| `since` | string | | Only sessions started after this RFC 3339 timestamp |
| `limit` | int | `50` | Maximum results to return |
| `offset` | int | `0` | Pagination offset |

**Response:**

```json
{
  "sessions": [
    {
      "id": "ses_abc123xyz",
      "agent_id": "research-agent",
      "started_at": "2026-02-25T10:00:00Z",
      "ended_at": null,
      "status": "active",
      "total_cost": 0.0342,
      "action_count": 7,
      "metadata": {"task": "research"},
      "score": null
    }
  ],
  "total": 42
}
```

### Get Session

```
GET /api/sessions/:id
```

Returns session details along with all traces for that session.

**Response:**

```json
{
  "session": {
    "id": "ses_abc123xyz",
    "agent_id": "research-agent",
    "started_at": "2026-02-25T10:00:00Z",
    "ended_at": "2026-02-25T10:05:32Z",
    "status": "completed",
    "total_cost": 0.1247,
    "action_count": 15,
    "metadata": {"task": "research"},
    "score": null
  },
  "traces": [
    {
      "id": "01HQZX7ABC123DEF456",
      "session_id": "ses_abc123xyz",
      "agent_id": "research-agent",
      "timestamp": "2026-02-25T10:00:01Z",
      "action_type": "llm.chat",
      "action_name": "chat_completion",
      "status": "allowed",
      "latency_ms": 1250,
      "tokens_in": 512,
      "tokens_out": 256,
      "cost_usd": 0.0089,
      "model": "gpt-4o"
    }
  ]
}
```

### Terminate Session

```
POST /api/sessions/:id/terminate
```

Forcefully terminates an active session. All subsequent requests for this session will be rejected.

**Note:** The README shows this as `DELETE /api/sessions/:id`, which also works as an alias.

**Response:**

```json
{
  "status": "terminated"
}
```

---

## Traces

### List Traces

```
GET /api/traces
```

Returns a paginated list of traces.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `session_id` | string | | Filter by session ID |
| `agent_id` | string | | Filter by agent ID |
| `action_type` | string | | Filter by action type: `llm.chat`, `llm.embedding`, `tool.call`, `api.request`, `db.query`, `file.write`, `code.exec`, `mcp.tool` |
| `status` | string | | Filter by status: `allowed`, `denied`, `terminated`, `approved`, `pending`, `throttled` |
| `limit` | int | `50` | Maximum results to return |
| `offset` | int | `0` | Pagination offset |

**Response:**

```json
{
  "traces": [
    {
      "id": "01HQZX7ABC123DEF456",
      "session_id": "ses_abc123xyz",
      "agent_id": "research-agent",
      "timestamp": "2026-02-25T10:00:01Z",
      "action_type": "llm.chat",
      "action_name": "chat_completion",
      "request_body": {"model": "gpt-4o", "messages": [...]},
      "response_body": {"choices": [...]},
      "status": "allowed",
      "policy_name": "",
      "policy_reason": "",
      "latency_ms": 1250,
      "tokens_in": 512,
      "tokens_out": 256,
      "cost_usd": 0.0089,
      "metadata": null,
      "prev_hash": "a1b2c3d4...",
      "hash": "e5f6g7h8...",
      "model": "gpt-4o"
    }
  ],
  "total": 150
}
```

### Get Trace

```
GET /api/traces/:id
```

Returns a single trace by its ID.

**Response:**

```json
{
  "id": "01HQZX7ABC123DEF456",
  "session_id": "ses_abc123xyz",
  "agent_id": "research-agent",
  "timestamp": "2026-02-25T10:00:01Z",
  "action_type": "llm.chat",
  "action_name": "chat_completion",
  "request_body": {"model": "gpt-4o", "messages": [...]},
  "response_body": {"choices": [...]},
  "status": "allowed",
  "policy_name": "",
  "policy_reason": "",
  "latency_ms": 1250,
  "tokens_in": 512,
  "tokens_out": 256,
  "cost_usd": 0.0089,
  "metadata": null,
  "prev_hash": "a1b2c3d4e5f6...",
  "hash": "e5f6g7h8i9j0...",
  "model": "gpt-4o"
}
```

### Search Traces

```
GET /api/traces/search
```

Full-text search across traces.

**Query Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `q` | string | Yes | Search query string |
| `limit` | int | No | Maximum results (default: 50) |

**Response:**

```json
{
  "traces": [
    {
      "id": "01HQZX7ABC123DEF456",
      "session_id": "ses_abc123xyz",
      "agent_id": "research-agent",
      "timestamp": "2026-02-25T10:00:01Z",
      "action_type": "tool.call",
      "action_name": "web_search",
      "status": "denied",
      "policy_name": "block-web-search",
      "model": ""
    }
  ]
}
```

---

## Agents

### List Agents

```
GET /api/agents
```

Returns all registered agents. Agents are auto-registered on first request.

**Response:**

```json
{
  "agents": [
    {
      "id": "research-agent",
      "name": "research-agent",
      "created_at": "2026-02-24T14:30:00Z",
      "current_version": "",
      "config": null,
      "metadata": null
    }
  ]
}
```

### Get Agent

```
GET /api/agents/:id
```

Returns a single agent by ID.

**Response:**

```json
{
  "id": "research-agent",
  "name": "research-agent",
  "created_at": "2026-02-24T14:30:00Z",
  "current_version": "v3",
  "config": {"model": "gpt-4o", "temperature": 0.7},
  "metadata": {"team": "platform"}
}
```

### Get Agent Stats

```
GET /api/agents/:id/stats
```

Returns aggregated statistics for an agent.

**Response:**

```json
{
  "agent_id": "research-agent",
  "total_sessions": 142,
  "active_sessions": 3,
  "total_cost": 47.82,
  "total_actions": 2840,
  "total_violations": 12,
  "avg_cost_per_session": 0.3368,
  "completion_rate": 0.92,
  "error_rate": 0.03
}
```

### List Agent Versions

```
GET /api/agents/:id/versions
```

Returns all version snapshots for an agent (used by the evolution engine).

**Response:**

```json
{
  "versions": [
    {
      "id": "ver_001",
      "agent_id": "research-agent",
      "version_number": 1,
      "created_at": "2026-02-20T10:00:00Z",
      "promoted_at": "2026-02-20T10:00:00Z",
      "rolled_back_at": null,
      "status": "retired",
      "system_prompt": "You are a research assistant...",
      "config": {"temperature": 0.7},
      "diff_from_prev": "",
      "diff_reason": "Initial version",
      "shadow_results": null,
      "metadata": null
    },
    {
      "id": "ver_002",
      "agent_id": "research-agent",
      "version_number": 2,
      "created_at": "2026-02-22T08:00:00Z",
      "promoted_at": "2026-02-22T14:00:00Z",
      "status": "active",
      "diff_from_prev": "Changed temperature from 0.7 to 0.5",
      "diff_reason": "Reduce hallucination rate identified in failure pattern FP-003"
    }
  ]
}
```

---

## Policies

### List Policies

```
GET /api/policies
```

Returns the currently loaded policy configuration.

**Response:**

```json
{
  "policies": [
    {
      "name": "session-budget",
      "condition": "session.cost > 10.0",
      "effect": "terminate",
      "message": "Session killed: exceeded $10 budget",
      "type": "",
      "delay": 0,
      "prompt": "",
      "model": "",
      "approvers": null,
      "timeout": 0,
      "timeout_effect": ""
    },
    {
      "name": "block-shell-exec",
      "condition": "action.type == \"tool.call\" && action.name == \"shell_exec\"",
      "effect": "deny",
      "message": "Shell execution is not allowed"
    }
  ]
}
```

### Reload Policies

```
POST /api/policies/reload
```

Hot-reload policies from the config file without restarting the proxy. The config file is re-read from disk and all CEL expressions are recompiled.

**Response:**

```json
{
  "status": "reloaded"
}
```

---

## Approvals

### List Pending Approvals

```
GET /api/approvals
```

Returns all pending approval requests.

**Response:**

```json
{
  "approvals": [
    {
      "id": "apr_xyz789",
      "session_id": "ses_abc123xyz",
      "trace_id": "01HQZX7ABC123DEF456",
      "policy_name": "db-write-approval",
      "action_summary": {
        "type": "db.query",
        "name": "write",
        "target": "users_table"
      },
      "status": "pending",
      "created_at": "2026-02-25T10:30:00Z",
      "resolved_at": null,
      "resolved_by": "",
      "timeout_at": "2026-02-25T10:35:00Z"
    }
  ]
}
```

### Approve Action

```
POST /api/approvals/:id/approve
```

Approve a pending action, allowing it to proceed.

**Response:**

```json
{
  "status": "approved"
}
```

### Deny Action

```
POST /api/approvals/:id/deny
```

Deny a pending action, blocking it.

**Response:**

```json
{
  "status": "denied"
}
```

---

## Violations

### List Violations

```
GET /api/violations
```

Returns policy violation records.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `agent_id` | string | | Filter by agent ID |
| `limit` | int | `50` | Maximum results |

**Response:**

```json
{
  "violations": [
    {
      "id": "vio_001",
      "trace_id": "01HQZX7ABC123DEF456",
      "session_id": "ses_abc123xyz",
      "agent_id": "research-agent",
      "policy_name": "block-shell-exec",
      "effect": "denied",
      "timestamp": "2026-02-25T10:15:00Z",
      "action_summary": {
        "type": "tool.call",
        "name": "shell_exec",
        "params": {"command": "rm -rf /"}
      }
    }
  ]
}
```

---

## Kill Switch

Emergency stop mechanism that operates outside the LLM context window. See [OpenClaw Integration - Kill Switch](openclaw.md#kill-switch) for full details.

### Trigger Kill Switch

```
POST /api/killswitch/trigger
```

Activates the kill switch at the specified scope. All actions matching the scope are immediately blocked.

**Request Body:**

```json
{
  "scope": "global",
  "target_id": "",
  "reason": "runaway agent detected"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `scope` | string | Yes | Kill scope: `global`, `agent`, or `session` |
| `target_id` | string | For agent/session | Agent ID or session ID to kill |
| `reason` | string | Yes | Human-readable reason (recorded in audit trail) |

**Response:**

```json
{
  "status": "triggered",
  "scope": "global"
}
```

### Get Kill Switch Status

```
GET /api/killswitch/status
```

Returns the current state of all kill switches.

**Response:**

```json
{
  "global_triggered": false,
  "agent_kills": {},
  "session_kills": {},
  "history_count": 3
}
```

### Reset Kill Switch

```
POST /api/killswitch/reset
```

Disarms the kill switch at the specified scope.

**Request Body:**

```json
{
  "scope": "global"
}
```

**Response:**

```json
{
  "status": "reset"
}
```

---

## System

### Health Check

```
GET /api/health
```

Simple health check endpoint. Returns 200 if the server is running.

**Response:**

```json
{
  "status": "ok"
}
```

### System Statistics

```
GET /api/stats
```

Returns aggregate system metrics.

**Response:**

```json
{
  "total_traces": 15240,
  "total_sessions": 342,
  "active_sessions": 5,
  "total_agents": 8,
  "total_cost": 284.57,
  "total_violations": 47,
  "pending_approvals": 2
}
```

---

## WebSocket

### Live Trace Feed

```
ws://localhost:6777/api/ws/traces
```

Real-time WebSocket feed of trace events. Connect to receive every trace as it is recorded.

**Connection:**

```javascript
const ws = new WebSocket('ws://localhost:6777/api/ws/traces');

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log(message);
};
```

**Message Format:**

Every message is a JSON object with `type` and `data` fields:

```json
{
  "type": "trace",
  "data": {
    "id": "01HQZX7ABC123DEF456",
    "session_id": "ses_abc123xyz",
    "agent_id": "research-agent",
    "timestamp": "2026-02-25T10:00:01Z",
    "action_type": "llm.chat",
    "action_name": "chat_completion",
    "status": "allowed",
    "latency_ms": 1250,
    "tokens_in": 512,
    "tokens_out": 256,
    "cost_usd": 0.0089,
    "model": "gpt-4o",
    "hash": "e5f6g7h8i9j0..."
  }
}
```

**Notes:**

- The WebSocket connection accepts all origins (no CORS restriction)
- Messages are JSON-encoded and sent as text frames
- The server sends a message for every trace recorded, including denied and terminated actions
- There is no subscription or filtering mechanism; the client receives all traces
- Request and response bodies are included in the trace data (truncated to 1MB)
- To keep the connection alive, the client should handle ping/pong frames (handled automatically by most WebSocket libraries)

**Python Example:**

```python
import asyncio
import json
import websockets

async def watch_traces():
    async with websockets.connect("ws://localhost:6777/api/ws/traces") as ws:
        async for message in ws:
            trace = json.loads(message)
            data = trace["data"]
            print(f"[{data['action_type']}] {data['action_name']} -> {data['status']} (${data['cost_usd']:.4f})")

asyncio.run(watch_traces())
```
