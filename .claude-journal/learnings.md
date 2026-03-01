# AgentWarden — Learnings & Decisions Log

## 2025-02-24: Architecture Shift (Proxy → Event-Driven Sidecar)

**Decision**: Completely removed the proxy architecture. AgentWarden no longer sits between the agent and the LLM.

**Why**:
- Proxy adds latency to every LLM call
- Proxy breaks streaming (or makes it much harder)
- Proxy can't govern non-LLM actions (tool calls, API calls, DB queries)
- Event-driven lets SDKs report actions asynchronously
- Agents can fail-open if AgentWarden is down (configurable)

**Pattern**: SDK creates a session → reports each action → AgentWarden evaluates policies → returns verdict → SDK decides whether to proceed

## 2025-02-24: Go Evolution Engine (replaced Python sidecar)

**Decision**: Rewrote the evolution engine in Go instead of keeping it as a separate Python sidecar.

**Why**:
- Single binary deployment (no Python runtime dependency)
- Direct access to trace store, MD loader, session manager without HTTP round-trips
- Same concurrency model (goroutines) as the rest of the system
- Shadow runner needs tight integration with the request path

**Tradeoff**: Lost the convenience of Python's ML/NLP libraries for analysis. Mitigated by using LLM for analysis (the LLM IS the ML library).

## 2025-02-24: Session-Based SDK Design

**Pattern**: Every governed interaction is wrapped in a session:
```python
async with warden.session(agent_id="bot") as session:
    await session.tool(name="search", params={...}, execute=lambda: ...)
    await session.chat(messages=[...], execute=lambda: ...)
    await session.score(quality=0.95)
```

**Key insight**: The `execute` callback pattern lets AgentWarden evaluate the policy BEFORE the action runs, then execute only if allowed. This is fundamentally different from the proxy which could only observe traffic.

## 2025-02-25: Version Label Cleanup

**Learning**: Never label something as a version in code/commits when it's the primary thing being built. "v2" implies there's a stable "v1" somewhere — confusing for new contributors. Just build the thing.

## Open Questions
- Is SQLite sufficient for production, or do we need PostgreSQL from day 1?
- Should gRPC be the primary transport, or is HTTP-only simpler and good enough?
- How do we handle the cold-start problem for evolution? (No traces = nothing to analyze)
- Should the dashboard be a separate deployable, or always embedded in the binary?
