# AgentWarden — Future TODOs & Ideas

## Priority 1: Prove It Works (E2E Testing)
- [ ] Build a real agent (support-bot / code-review / data agent) that runs through AgentWarden
- [ ] Wire SDK → gRPC/HTTP → policy evaluation → trace storage → dashboard visibility
- [ ] Test: Does loop detection actually catch loops? Does cost tracking work? Do policies block bad actions?
- [ ] Test: Does the evolution engine actually improve prompts over time?
- [ ] Document what's broken, what's missing, what's useless

## Priority 2: CI/CD & Deployment
- [ ] Set up GitHub Actions workflow for Go build + test on PRs
- [ ] Set up GitHub Actions for release binaries (goreleaser)
- [ ] Deploy docs site to Vercel (or GitHub Pages)
- [ ] Docker image build + push to GHCR on release tags
- [ ] Helm chart publish to GitHub Pages (chart museum)

## Priority 3: SDK Polish
- [ ] Python SDK: Add gRPC transport option (currently HTTP-only)
- [ ] Python SDK: Publish to PyPI (test.pypi.org first)
- [ ] TypeScript SDK: Publish to npm
- [ ] TypeScript SDK: Add gRPC transport via @grpc/grpc-js
- [ ] Both SDKs: Add retry logic with exponential backoff
- [ ] Both SDKs: Add connection health checks

## Priority 4: Missing Features from Plan
- [ ] Approval queue WebSocket push (currently polling)
- [ ] Multi-agent session tracking (parent/child sessions)
- [ ] Trace export (OpenTelemetry format)
- [ ] Dashboard: Real-time WebSocket trace feed (backend exists, frontend needs wiring)
- [ ] Dashboard: Agent comparison view (side-by-side metrics)
- [ ] Rate limiting policy type (currently only CEL/AI-judge/budget)
- [ ] Webhook alerting improvements (retry, dead letter queue)

## Priority 5: Production Readiness
- [ ] PostgreSQL storage driver (currently SQLite only)
- [ ] Redis-backed session store for horizontal scaling
- [ ] mTLS for gRPC connections
- [ ] API key authentication middleware
- [ ] Structured audit logging
- [ ] Prometheus metrics endpoint (/metrics)
- [ ] Graceful degradation when LLM is unavailable (for AI-judge + playbooks)

## Ideas
- **AgentWarden CLI plugin system**: `agentwarden plugin install <name>` for custom detectors
- **Policy marketplace**: Share/import policies from community
- **Agent fingerprinting**: Automatically detect agent framework (LangChain/CrewAI/AutoGen) from traffic patterns
- **Cost prediction**: Before action execution, estimate cost and warn if over budget
- **Prompt diff viewer**: Visual diff tool in dashboard for evolution candidates
- **Regression test suite**: Replay recorded sessions against new prompt versions
- **Multi-tenant mode**: Single AgentWarden instance governing agents across teams with isolation
- **VS Code extension**: See AgentWarden traces inline while developing agents
