# AgentWarden — Product Management Guide

## Push Review Rules

**Not every push needs a version bump.** Use this framework:

### When to bump version
- **Major (1.0.0)**: First public release, breaking API changes
- **Minor (0.x.0)**: New feature that users interact with (new CLI command, new SDK method, new detection type, new dashboard page)
- **Patch (0.0.x)**: Bug fixes, docs, internal refactors, test additions

### When NOT to bump version
- Cleanup commits (removing dead code, fixing comments, renaming)
- CI/CD changes (GitHub Actions, Dockerfile tweaks)
- Journal/docs-only changes
- Refactors that don't change external behavior
- Adding tests for existing functionality

### Push Quality Checklist
Before each push, verify:
- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] No secrets/tokens in committed files
- [ ] Commit messages are descriptive (what + why, not just what)
- [ ] No debug/TODO comments left in production code
- [ ] No version labels ("v2", "new") in code — just build the thing

---

## Product Research: How AgentWarden Is Different

> Full competitive analysis: [competitive-analysis.md](./competitive-analysis.md)

### The Landscape (as of Feb 2026)

The AI agent tooling space has fragmented into 5 categories. AgentWarden sits in a new category — **Agent Control Planes** — that Forrester formalized in 2025-2026.

| Category | What they do | Examples | AgentWarden overlap |
|----------|-------------|----------|-------------------|
| **LLM Proxies** | Route/cache/track LLM API calls | LiteLLM, Portkey, Helicone, Cloudflare AI GW | Cost tracking only |
| **Guardrails** | Validate LLM inputs/outputs | Guardrails AI, NeMo Guardrails, Lakera | Policy enforcement only |
| **Observability** | Trace and evaluate LLM calls | LangSmith, Langfuse, Arize Phoenix, Braintrust | Tracing only |
| **Agent Frameworks** | Build agents with built-in controls | CrewAI, LangGraph, Microsoft Agent Framework | Framework-locked governance |
| **Control Planes** | Govern agent behavior holistically | Fiddler ($100M raised), Swept AI, AControlLayer | **Direct competitors** |

### Why AgentWarden Is Different

**Every existing tool solves a fragment of the problem. AgentWarden is the only product that combines all six:**

#### 1. Sidecar, Not Proxy
| | Proxies (LiteLLM, Portkey) | AgentWarden |
|---|---|---|
| Architecture | Sits in request path | Receives events asynchronously |
| Latency impact | Adds latency to every call | Zero added latency |
| Failure mode | Agent breaks if proxy is down | Agent keeps running (fail-open) |
| Visibility | LLM calls only | Full session (tools, APIs, DB, chat) |

**Analogy**: AgentWarden is to AI agents what Envoy sidecar is to microservices.

#### 2. Full Session Governance (Not Just LLM Calls)
Proxies and observability tools only see LLM API calls. They can't detect:
- An agent calling the same tool 50 times in a loop
- An agent's API calls drifting from its expected behavior
- An agent burning money on DB queries, not LLM calls
- An agent's overall session quality degrading

AgentWarden tracks **everything the agent does** — tool calls, API calls, DB queries, chat messages — in a single session context.

#### 3. Self-Evolution (Nobody Else Does This)
| | Braintrust | Comet Opik | AgentWarden |
|---|---|---|---|
| Trace analysis | AI suggests improvements | 7 optimization algorithms | LLM analyzes failure patterns |
| Proposal | Manual | Developer-triggered | Automatic prompt diffs |
| Testing | Manual A/B | Offline evaluation | Shadow testing on live traffic |
| Deployment | Manual | Manual | Auto-promote if metrics improve |
| Rollback | Manual | Manual | Auto-rollback on degradation |

AgentWarden is the only product with a **closed-loop** evolution pipeline: analyze → propose → shadow test → promote/reject → rollback if needed.

#### 4. Config Files for Cognition (MD-Based Governance)
No competitor uses markdown files for governance config:
- `AGENT.md` — Agent identity, capabilities, constraints
- `EVOLVE.md` — What can change, what must not, optimization goals
- `PROMPT.md` — Versioned system prompts (v1, v2, v3...)
- `POLICY.md` — Rich context for AI-judge policy evaluation

**Why it matters**: Git-managed, PR-reviewable, diffable. DevOps teams already think in config files. No proprietary DSLs (unlike NeMo's Colang), no dashboard-only config.

#### 5. Playbook-Driven Detection
Static rules catch known patterns. AgentWarden's playbooks combine pattern matching with LLM-driven analysis:

| Detection | How it works | What it catches |
|-----------|-------------|-----------------|
| Loop | Signature matching + threshold | Agent repeating the same action |
| Spiral | Cosine similarity on outputs | Agent generating degrading responses |
| Drift | KL-divergence on action distributions | Agent behavior shifting from baseline |
| Cost anomaly | Per-action cost tracking | Runaway spending |

Each detection type has a **playbook** — a markdown file that an LLM uses to make nuanced verdicts (allow/pause/terminate/alert/backoff), not just binary rules.

#### 6. CEL + LLM Judge Hybrid Policy Engine
| | Rule engines (OPA, CEL) | LLM-only judges | AgentWarden |
|---|---|---|---|
| Speed | Fast (microseconds) | Slow (seconds) | Fast for rules, LLM for nuance |
| Nuance | None — binary match | High — context-aware | Both |
| Predictability | 100% deterministic | Variable | Deterministic base + nuanced overlay |
| Config | YAML/Rego/CEL | Prompts | CEL rules + POLICY.md context |

### Positioning Statement

> **AgentWarden is a governance sidecar for AI agents — like Envoy for microservices, but for AI. It tracks full agent sessions (not just LLM calls), enforces policies without adding latency, detects behavioral anomalies with LLM-driven playbooks, and automatically evolves agent prompts through shadow-tested improvements.**

### Target Personas

| Who | Their pain | AgentWarden pitch |
|-----|-----------|-------------------|
| **Platform Engineering** | "Agents are black boxes in prod" | Full session observability + behavioral detection |
| **AI/ML Engineering** | "We manually tune prompts for weeks" | Self-evolution automates improvement |
| **CISO / Security** | "Can't add latency for security checks" | Sidecar = zero latency overhead |
| **VP Engineering** | "Governance is 5 different tools" | One sidecar replaces the stack |
| **DevOps / SRE** | "AI governance isn't in CI/CD" | MD config = git-managed, PR-reviewed |

### Risks & Gaps

| Risk | Severity | Mitigation |
|------|----------|------------|
| "Control Plane" category is new, buyers don't understand it | High | Lead with failure stories, not architecture diagrams |
| Incumbents bundle "good enough" governance | High | Emphasize cross-framework, vendor-neutral value |
| Self-evolution scares compliance teams | Medium | Default to "suggest + human approve", not autonomous |
| SDK integration burden per framework | Medium | Ship top-5 framework SDKs first, one-line init |
| No open source strategy yet | High | Consider OSS core for community adoption |

### Competitive Moats (What's Hard to Copy)

1. **Sidecar architecture** — Proxies can't retrofit into sidecars without rewriting. This is a fundamental architectural difference.
2. **Self-evolution pipeline** — The full loop (analyze → propose → shadow → promote → rollback) is complex and tightly integrated. Adding this to an observability tool is a major undertaking.
3. **MD-based config** — This is a design philosophy, not a feature. Once teams adopt governance-as-code, switching cost is high.
4. **Behavioral detection library** — Each playbook encodes domain knowledge about AI agent failure modes. This library grows over time.

---

## Market Size & Timing

- AI Agent market: **$7.84B (2025) → $52.62B (2030)**, CAGR 46.3%
- Gartner: 40% of enterprise apps will embed AI agents by end of 2026
- Gartner: Organizations with governance see 50% improvement in adoption rates
- Governance gap is #1 barrier to agent adoption (McKinsey 2025)
- Capital flowing: Fiddler ($100M total), Braintrust ($80M Series B at $800M), Dynamo AI ($30M)

**Timing is right**: Enterprises are deploying agents NOW and hitting governance problems. The market is forming. First-mover advantage in the sidecar/control-plane category is available.
