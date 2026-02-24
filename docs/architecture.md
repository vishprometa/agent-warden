# Architecture Deep Dive

A detailed look at AgentWarden's internal architecture, covering the interception pipeline, data model, component design, and performance characteristics.

## Table of Contents

- [System Overview](#system-overview)
- [Architecture Diagram](#architecture-diagram)
- [Component Descriptions](#component-descriptions)
- [Request Flow Walkthrough](#request-flow-walkthrough)
- [Data Model](#data-model)
- [Hash Chain Integrity](#hash-chain-integrity)
- [Concurrency Model](#concurrency-model)
- [Performance Characteristics](#performance-characteristics)
- [Build and Packaging](#build-and-packaging)

---

## System Overview

AgentWarden is a transparent HTTP reverse proxy that sits between AI agents and their upstream LLM providers. It intercepts every request, classifies it, evaluates governance policies, tracks cost, detects anomalies, and records an immutable audit trail -- all before forwarding the request upstream.

The system is a single Go binary (~26 MB) that serves three roles on a single port (default 6777):

1. **Reverse proxy** at `/` -- intercepts and forwards agent-to-LLM traffic
2. **Management API** at `/api/` -- REST endpoints for sessions, traces, agents, policies, approvals, violations, and system stats
3. **Monitoring dashboard** at `/dashboard` -- embedded React SPA for real-time visibility

All state is persisted to a local SQLite database. The dashboard is compiled into the binary via Go's `embed` package, so there are no external runtime dependencies.

---

## Architecture Diagram

```
                          AI Agent (Python / TypeScript / HTTP)
                                        |
                        X-AgentWarden-Agent-Id: my-agent
                        X-AgentWarden-Session-Id: ses_abc
                        Authorization: Bearer sk-...
                                        |
                                        v
                   +--------------------------------------------+
                   |         AgentWarden  (:6777)               |
                   |                                            |
                   |  /dashboard    /api/*        /*            |
                   |     |            |            |            |
                   |  [React SPA]  [API Server]  [Proxy]       |
                   |                  |            |            |
                   |                  |    +-------+-------+   |
                   |                  |    |               |   |
                   |                  | [Classifier]  [Router]  |
                   |                  |    |               |   |
                   |                  |    v               |   |
                   |              [Policy Engine]          |   |
                   |               /    |    \             |   |
                   |           Budget  CEL  Approval       |   |
                   |                  |                    |   |
                   |        +---------+--------+          |   |
                   |        |         |        |          |   |
                   |   [Cost      [Session  [Detection    |   |
                   |    Tracker]   Manager]  Engine]       |   |
                   |        |         |      / | \        |   |
                   |        |         |  Loop Cost Spiral  |   |
                   |        |         |        |          |   |
                   |        +----+----+--------+          |   |
                   |             |                        |   |
                   |        [Trace Store]                  |   |
                   |             |                        |   |
                   |        [SQLite DB]                    |   |
                   |             |                        |   |
                   |      [Alert Manager]   [WebSocket Hub]|   |
                   |        /        \          |         |   |
                   |    Slack      Webhook   Clients      |   |
                   +--------------------------------------------+
                                        |
                                        v
                   +--------------------------------------------+
                   |         Upstream LLM Providers              |
                   |                                            |
                   |   OpenAI    Anthropic    Google Gemini      |
                   |   api.openai.com  api.anthropic.com  ...   |
                   +--------------------------------------------+

              (Optional, separate process)

                   +--------------------------------------------+
                   |         Evolution Sidecar (Python)          |
                   |                                            |
                   |   Scorer -> Analyzer -> Shadow Runner       |
                   |              |                             |
                   |         Uses /api/* management API          |
                   +--------------------------------------------+
```

---

## Component Descriptions

### Proxy (`internal/proxy`)

The central HTTP handler implementing the full interception pipeline. It coordinates all other components through well-defined interfaces.

| Sub-component | File | Role |
|---------------|------|------|
| **Proxy** | `proxy.go` | Main HTTP handler. Implements `http.Handler` so it can be mounted alongside other handlers. Uses functional options (`WithPolicyEngine`, `WithCostTracker`, etc.) for pluggable composition. |
| **Classifier** | `classifier.go` | Inspects URL path suffixes and request body to categorize each request into an `ActionType` (e.g., `llm.chat`, `tool.call`) and extract the model name. Uses a static rule table ordered by specificity. |
| **Router** | `router.go` | Resolves the upstream provider URL based on the model name. Maps model prefixes (`gpt-` -> OpenAI, `claude-` -> Anthropic, `gemini-` -> Google) to configured provider URLs, with a fallback to the default upstream. |
| **SSE Streamer** | `streamer.go` | Handles Server-Sent Events streaming responses. Forwards SSE events to the client in real time while accumulating the full response body for trace storage. |
| **Interceptor** | `interceptor.go` | Captures request bodies (read-and-restore pattern so the body remains readable) and records response bodies for non-streaming requests. |
| **Adapters** | `adapters.go` | Bridges internal package types to proxy-level interfaces (`PolicyEngine`, `DetectionEngine`, `CostTracker`, `AlertManager`). Keeps the proxy decoupled from concrete implementations. |

### Policy Engine (`internal/policy`)

Evaluates governance policies against every intercepted action. The engine holds a compiled, ordered set of policies protected by a `sync.RWMutex` for concurrent read access during evaluation and exclusive write access during hot-reload.

| Sub-component | File | Role |
|---------------|------|------|
| **Engine** | `engine.go` | Orchestrator that runs the evaluation pipeline: budget -> rate limit -> CEL -> AI judge -> approval. Short-circuits on the first `deny` or `terminate`. Accumulates the longest `throttle` delay. Thread-safe via RWMutex. |
| **CEL Evaluator** | `cel.go` | Compiles CEL expressions at startup using `google/cel-go`. Declares typed variables (`action.type`, `session.cost`, `agent.id`, etc.) and evaluates compiled programs with zero allocations in the hot path. |
| **Loader** | `loader.go` | Parses policy configs, classifies them (CEL, AI judge, approval, budget, rate limit), and compiles CEL expressions. Includes an `fsnotify` file watcher for automatic hot-reload when the config file changes. |
| **Budget Checker** | `budget.go` | Stateless evaluator that compares `session.cost` against threshold values extracted from budget policy conditions. |
| **Rate Limiter** | `ratelimit.go` | Sliding-window rate limiter with time-bucketed counters (1-second granularity). Lazy garbage collection. Supports windows up to 24 hours. |

### Detection Engine (`internal/detection`)

Runs three independent anomaly detectors on every intercepted action. Each detector maintains per-session state and can trigger pause, alert, or terminate actions.

| Detector | File | Algorithm |
|----------|------|-----------|
| **Loop** | `loop.go` | Sliding-window counter that tracks (sessionID, signature) pairs. A signature is the concatenation of action type, name, and model. When the same signature repeats more than `threshold` times within `window`, a loop is detected. |
| **Cost Anomaly** | `anomaly.go` | Compares the average cost-per-action in the last 30 seconds against a baseline established from earlier actions in the session. Requires at least 3 data points before establishing a baseline. Fires when the recent rate exceeds the baseline by the configured `multiplier`. |
| **Spiral** | `spiral.go` | Detects non-converging conversation loops by computing word-frequency-based cosine similarity between consecutive LLM outputs. When `window` consecutive outputs exceed `similarity_threshold`, a spiral is detected. |

### Session Manager (`internal/session`)

Manages active agent sessions with thread-safe in-memory state backed by persistent storage.

- Sessions are identified by `ses_` prefixed IDs generated with `crypto/rand`
- In-memory state includes cost accumulator, action count, per-action-type timestamps (for sliding-window rate limiting), and pause flag
- State is persisted to SQLite via the trace store on every cost or action count update
- Agents are auto-registered on first request (upserted by the session manager)

### Cost Tracker (`internal/cost`)

Estimates token counts and computes USD costs for LLM requests.

- Ships with per-model pricing for 20+ models from OpenAI, Anthropic, Google, Meta, Mistral, and DeepSeek
- Token counting uses response body parsing (extracting `usage.prompt_tokens` and `usage.completion_tokens` from provider responses) with fallback to character-based estimation
- Unknown models use a fallback rate of $1.00/M input tokens and $3.00/M output tokens
- Per-session and per-agent cost accumulators

### Trace Store (`internal/trace`)

Defines the `Store` interface for all persistence operations and the data models (Trace, Session, Agent, AgentVersion, Approval, Violation). The SQLite implementation is in `internal/trace/sqlite.go`.

- Six tables: `traces`, `sessions`, `agents`, `agent_versions`, `approvals`, `violations`
- Full-text search via SQLite FTS5
- Configurable retention with automatic pruning of traces older than the configured window (default: 30 days)
- Hash chain verification for audit trail integrity

### Alert Manager (`internal/alert`)

Dispatches alerts for policy violations and anomaly detections with deduplication.

- Alerts are deduplicated with a 5-minute TTL per (type, agent, session) combination
- Two sender implementations: Slack (incoming webhook) and generic HTTP webhook (with HMAC-SHA256 signing)
- Alert payloads include type, severity, message, agent/session context, and detection details

### Approval Queue (`internal/approval`)

Manages human-in-the-loop approval gates for high-risk actions.

- When a policy with `effect: approve` fires, the request is parked in the approval queue
- Approvers can approve or deny via the dashboard or REST API (`POST /api/approvals/:id/approve`)
- Configurable timeout with a fallback effect (`deny` or `allow`) when the timeout expires
- Alert notifications sent to configured channels when a new approval is pending

### API Server (`internal/api`)

REST API and WebSocket server for management and real-time monitoring.

- Uses Go 1.22+ method-based routing (`"GET /api/sessions"`)
- WebSocket hub (`gorilla/websocket`) broadcasts every trace to connected clients
- CORS middleware for development mode
- All endpoints return JSON with standard error format

### Dashboard (`internal/dashboard`)

Embedded React SPA compiled into the Go binary via `go:embed`.

- Built with Vite during the Docker build stage
- Served at `/dashboard` by the Go HTTP server
- Connects to the WebSocket feed for real-time trace updates

### Evolution Sidecar (`evolution/`)

A standalone Python process (separate from the Go binary) that observes agent behavior and proposes configuration improvements.

- Communicates with AgentWarden exclusively through the management API
- Does not intercept traffic or modify proxy state directly
- Uses Pydantic v2 models for type-safe data structures
- Six-stage loop: score -> analyze -> propose -> shadow -> compare -> promote/reject

---

## Request Flow Walkthrough

Here is the step-by-step flow for a single request through the proxy:

### Step 1: Extract Headers

The proxy reads three custom headers from the incoming request:

| Header | Purpose |
|--------|---------|
| `X-AgentWarden-Agent-Id` | Identifies the agent (defaults to `"anonymous"`) |
| `X-AgentWarden-Session-Id` | Groups requests into a session (auto-generated if omitted) |
| `X-AgentWarden-Metadata` | Arbitrary JSON metadata attached to the session |

These headers are stripped before forwarding upstream -- the LLM provider never sees them.

### Step 2: Session Resolution

The session manager looks up or creates a session:

1. If a session ID is provided, check in-memory cache first, then persistent storage
2. If no session ID is provided, generate a new `ses_` prefixed ID
3. Auto-register the agent if this is the first request from that agent ID
4. Check if the session is paused (return 503 if so)

### Step 3: Body Capture

The request body is read and restored (so it remains readable for the reverse proxy). Bodies over 1 MB are truncated for storage but forwarded in full to the upstream.

### Step 4: Classification

The classifier inspects the URL path and request body to determine:

- **ActionType**: `llm.chat`, `llm.embedding`, `tool.call`, `api.request`, `db.query`, `file.write`, `code.exec`, or `mcp.tool`
- **ActionName**: Human-readable name (e.g., `"chat_completion"`, `"embedding"`)
- **Model**: Extracted from the `"model"` field in the JSON body

Classification uses a static rule table matching URL path suffixes:

| Path Suffix | ActionType | ActionName |
|-------------|-----------|------------|
| `/chat/completions` | `llm.chat` | `chat_completion` |
| `/embeddings` | `llm.embedding` | `embedding` |
| `/messages` | `llm.chat` | `messages` (Anthropic) |
| `:generateContent` | `llm.chat` | `generate` (Gemini) |

### Step 5: Policy Evaluation

A ULID-based trace ID is generated. The policy engine evaluates all loaded policies in order:

1. **Budget policies**: Check `session.cost` against thresholds
2. **Rate limit policies**: Check `session.action_count` against limits
3. **CEL policies**: Evaluate compiled CEL expressions
4. **AI judge policies**: (Future) LLM-evaluated semantic checks
5. **Approval policies**: Park the request for human review

If any policy returns `deny` or `terminate`, the request is blocked immediately. The proxy returns a JSON error with the trace ID, policy name, and reason. For `terminate`, the entire session is ended and an alert is dispatched.

### Step 6: Upstream Resolution

The router maps the model name to an upstream provider URL using prefix matching:

| Model Prefix | Provider |
|-------------|----------|
| `gpt-`, `o1-`, `o3-`, `o4-` | OpenAI |
| `claude-` | Anthropic |
| `gemini-`, `gemma-` | Google |

If no match is found, the configured `default` upstream is used.

### Step 7: Forward Request

The request is forwarded to the resolved upstream via Go's `net/http/httputil.ReverseProxy`. The `Authorization` header is passed through unchanged when `passthrough_auth: true` (the default).

For streaming requests (detected by `"stream": true` in the body), the proxy uses a custom HTTP client that streams SSE events through to the caller in real time while accumulating the full response body for trace storage.

### Step 8: Finalize Trace

After the upstream response is received:

1. **Token counting**: Extract `usage.prompt_tokens` and `usage.completion_tokens` from the response, or estimate from body size
2. **Cost calculation**: Look up per-model pricing and compute USD cost
3. **Hash chain**: Fetch the previous trace hash, compute SHA-256 of (ID | SessionID | ActionType | RequestBody | ResponseBody | PrevHash)
4. **Store trace**: Persist to SQLite with all fields
5. **Update session**: Add cost, increment action count
6. **Anomaly detection**: Feed the trace to loop, cost anomaly, and spiral detectors
7. **WebSocket broadcast**: Push the trace to all connected dashboard clients

The trace ID is returned to the caller in the `X-AgentWarden-Trace-Id` response header.

---

## Data Model

AgentWarden persists six entity types in SQLite:

### traces

The core audit log. Every intercepted action produces exactly one trace record.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT (ULID) | Globally unique, time-ordered identifier |
| `session_id` | TEXT | Parent session |
| `agent_id` | TEXT | Agent that performed the action |
| `timestamp` | DATETIME | When the action was intercepted |
| `action_type` | TEXT | Classified action type |
| `action_name` | TEXT | Human-readable action name |
| `request_body` | JSON | Captured request body (truncated at 1 MB) |
| `response_body` | JSON | Captured response body (truncated at 1 MB) |
| `status` | TEXT | Policy evaluation result |
| `policy_name` | TEXT | Name of the policy that triggered (empty if allowed) |
| `policy_reason` | TEXT | Human-readable policy message |
| `latency_ms` | INTEGER | End-to-end latency in milliseconds |
| `tokens_in` | INTEGER | Estimated input token count |
| `tokens_out` | INTEGER | Estimated output token count |
| `cost_usd` | REAL | Computed cost in USD |
| `metadata` | JSON | Arbitrary metadata |
| `prev_hash` | TEXT | SHA-256 hash of the previous trace (chain link) |
| `hash` | TEXT | SHA-256 hash of this trace |
| `model` | TEXT | LLM model name |

### sessions

Groups related traces into a logical unit of work.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Session ID (`ses_` prefix) |
| `agent_id` | TEXT | Owning agent |
| `started_at` | DATETIME | Session start time |
| `ended_at` | DATETIME | Session end time (null if active) |
| `status` | TEXT | `active`, `completed`, `terminated`, `paused` |
| `total_cost` | REAL | Accumulated cost in USD |
| `action_count` | INTEGER | Total actions in this session |
| `metadata` | JSON | Client-provided metadata |
| `score` | JSON | Composite score (set by the evolution engine) |

### agents

Registry of known agents. Auto-populated on first request.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Agent identifier |
| `name` | TEXT | Display name (defaults to ID) |
| `created_at` | DATETIME | First seen timestamp |
| `current_version` | TEXT | Active version ID (managed by evolution) |
| `config` | JSON | Agent configuration snapshot |
| `metadata` | JSON | Arbitrary metadata |

### agent_versions

Version history for agent configurations, managed by the evolution engine.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Version ID |
| `agent_id` | TEXT | Parent agent |
| `version_number` | INTEGER | Sequential version number |
| `created_at` | DATETIME | When the version was created |
| `promoted_at` | DATETIME | When promoted to active |
| `rolled_back_at` | DATETIME | When rolled back (if applicable) |
| `status` | TEXT | `active`, `candidate`, `shadow`, `retired`, `rolled_back` |
| `system_prompt` | TEXT | Full system prompt text |
| `config` | JSON | Configuration snapshot |
| `diff_from_prev` | TEXT | Human-readable diff description |
| `diff_reason` | TEXT | Why this change was proposed |
| `shadow_results` | JSON | Shadow test comparison results |

### approvals

Pending and resolved human approval requests.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Approval request ID |
| `session_id` | TEXT | Associated session |
| `trace_id` | TEXT | Associated trace |
| `policy_name` | TEXT | Policy that triggered the approval gate |
| `action_summary` | JSON | Summary of the action awaiting approval |
| `status` | TEXT | `pending`, `approved`, `denied`, `timed_out` |
| `created_at` | DATETIME | When the approval was requested |
| `resolved_at` | DATETIME | When resolved |
| `resolved_by` | TEXT | Who approved or denied |
| `timeout_at` | DATETIME | When the timeout expires |

### violations

Records of policy violations for audit and analysis.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Violation ID |
| `trace_id` | TEXT | Associated trace |
| `session_id` | TEXT | Associated session |
| `agent_id` | TEXT | Agent that violated the policy |
| `policy_name` | TEXT | Policy that was violated |
| `effect` | TEXT | Effect that was applied (`denied`, `terminated`) |
| `timestamp` | DATETIME | When the violation occurred |
| `action_summary` | JSON | Summary of the blocked action |

---

## Hash Chain Integrity

Every trace is linked to the previous trace in its session via a SHA-256 hash chain, creating a tamper-evident audit trail.

### How It Works

1. **Session seed**: The first trace in a session chains from `SHA-256(sessionID)` as its `prev_hash`
2. **Hash computation**: Each trace hash is computed as:
   ```
   SHA-256( ID | SessionID | ActionType | RequestBody | ResponseBody | PrevHash )
   ```
   The pipe character (`|`) is the literal delimiter in the concatenated string.
3. **Chain linkage**: Each trace's `prev_hash` equals the `hash` of the preceding trace in the same session

### Verification

The `VerifyChain` function walks a list of traces and checks two properties for each:

1. The stored `hash` matches the recomputed hash from the trace's fields
2. The stored `prev_hash` matches the `hash` of the preceding trace

If either check fails, the function returns the index of the first broken link. Verification is available via the CLI (`agentwarden trace verify`) and the management API.

### Guarantees

- **Tamper detection**: Modifying any field in a trace (request body, response body, status, cost) changes its hash and breaks the chain from that point forward
- **Insertion detection**: Inserting a trace between two existing traces breaks the chain because the inserted trace's hash will not match the next trace's `prev_hash`
- **Deletion detection**: Removing a trace breaks the chain because the next trace's `prev_hash` will not match the remaining predecessor's hash

---

## Concurrency Model

AgentWarden is designed for high-throughput concurrent operation:

| Component | Synchronization | Pattern |
|-----------|----------------|---------|
| **Policy Engine** | `sync.RWMutex` | Read lock during evaluation (concurrent reads), write lock during hot-reload |
| **Session Manager** | `sync.RWMutex` | Read lock for lookups and cost queries, write lock for state mutations |
| **Detection Engine** | `sync.RWMutex` | Read lock for configuration access, detectors have internal synchronization |
| **Trace Store** | SQLite WAL mode | Write-ahead logging allows concurrent reads during writes |
| **Alert Manager** | Deduplication map with mutex | 5-minute TTL prevents alert storms |
| **WebSocket Hub** | Channel-based | Non-blocking broadcast to connected clients |

All engines use a "fail open" strategy: if a policy evaluation encounters an error (e.g., CEL compilation failure), the action is allowed rather than blocked. This prevents configuration errors from causing outages.

---

## Performance Characteristics

### Latency Overhead

The proxy adds minimal latency to the request path:

| Stage | Typical Cost |
|-------|-------------|
| Header extraction and parsing | < 1 us |
| Session lookup (in-memory) | < 1 us |
| Request body capture (read + restore) | ~10 us for a 4 KB body |
| Classification (URL pattern match) | < 1 us |
| Policy evaluation (compiled CEL) | ~5-50 us per policy |
| Upstream resolution (prefix match) | < 1 us |
| **Total proxy overhead** | **~50-200 us** |

For context, a typical LLM API call takes 500 ms to 30 seconds, so the proxy overhead is negligible (0.001% to 0.04% of total request time).

### Post-Response Processing

These operations run after the response is sent to the client and do not add to perceived latency:

- Token counting and cost calculation: ~10 us
- Hash chain computation (SHA-256): ~5 us
- SQLite trace insertion: ~100-500 us (WAL mode)
- Detection engine feed: ~10-50 us
- WebSocket broadcast: non-blocking channel send

### Memory Usage

- Base memory: ~15 MB for the Go binary and runtime
- Per active session: ~2 KB (in-memory state, action timestamps)
- SQLite database: grows at approximately 1-5 KB per trace depending on request/response body sizes (truncated at 1 MB each)
- Dashboard: served from embedded filesystem, no runtime memory cost

### Storage

- SQLite with WAL mode for concurrent read/write access
- Automatic retention-based pruning (default: 30 days)
- Typical trace size: 1-5 KB (with truncated bodies)
- At 1,000 traces/day, expect ~1-5 MB/day of storage growth before retention pruning

### Scalability

AgentWarden is designed for single-instance deployment. For most use cases (teams running 1-100 agents), a single instance handles all traffic comfortably. The SQLite storage backend and in-memory session state are the primary scaling constraints:

- **Single writer**: SQLite allows one writer at a time (WAL mode provides concurrent reads during writes)
- **In-memory sessions**: All active session state lives in a single process
- **Horizontal scaling**: Not currently supported (would require replacing SQLite with PostgreSQL and adding distributed session state)

---

## Build and Packaging

### Multi-Stage Docker Build

The Docker build uses three stages:

```
Stage 1: Node.js (node:22-alpine)
  - Installs npm dependencies
  - Builds the React dashboard (Vite -> dist/)

Stage 2: Go (golang:1.26-alpine)
  - Installs gcc + musl-dev (CGO required for SQLite)
  - Downloads Go modules
  - Copies dashboard dist/ into embed directory
  - Builds static binary with version info via ldflags

Stage 3: Runtime (alpine:3.21)
  - Copies binary from Stage 2
  - Adds ca-certificates and tzdata
  - Creates /data directory for SQLite
  - Final image: ~26 MB
```

### Binary Composition

The final binary embeds:

- Go HTTP server (proxy, API, WebSocket)
- Compiled React dashboard (via `go:embed`)
- SQLite driver (via CGO, `mattn/go-sqlite3`)
- CEL expression evaluator (`google/cel-go`)
- Version info injected at build time via `-ldflags`

### CLI

Built with Cobra, the binary provides these commands:

| Command | Description |
|---------|-------------|
| `start` | Start the proxy, API, and dashboard |
| `init` | Generate a starter `agentwarden.yaml` |
| `status` | Show running status and active sessions |
| `version` | Print version, commit, and build date |
| `policy validate` | Validate policy CEL expressions |
| `policy reload` | Hot-reload policies from config file |
| `trace list` | List recent traces |
| `trace show` | Show a single trace by ID |
| `trace search` | Full-text search across traces |
| `trace verify` | Verify hash chain integrity |
| `agent list` | List registered agents |
| `doctor` | Run diagnostic checks |
| `mock` | Start with mock data for testing |
