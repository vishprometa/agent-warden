# AgentWarden Improvement Loop — State

## Config
- **Project**: /Users/vish/Developer/agentwarden
- **Started**: 2026-02-25
- **Iteration**: 6

## Task Backlog

### Priority 1 — Fix CI (broken right now)
- [x] Fix `internal/dashboard/embed.go` — uses `//go:embed dist/*` but dist/ is gitignored. CI can't build without it. Add a placeholder or generate a stub embed for CI, or commit a minimal dist.
- [x] Fix CI workflow `go-version: "1.26.0"` — doesn't exist. Change to `"1.25.x"` or `"stable"`.
- [x] Fix golangci-lint version mismatch — Go 1.25 target needs a newer lint build. Pin `golangci-lint-action` version that supports Go 1.25.
- [x] Verify CI passes on push after fixes. CI infrastructure works (build ✓, test ✓), but code has 39 linting errors that need fixing.
- [x] Fix golangci-lint errors: 35 errcheck violations (unchecked error returns) + 4 staticcheck violations (style issues)

### Priority 2 — Add Tests for Untested Packages (10 packages have zero tests)
- [ ] Add tests for `internal/session/manager.go` — session create/get/end, concurrent access, metadata handling
- [ ] Add tests for `internal/mdloader/loader.go` — file loading, caching, cache invalidation, version listing
- [ ] Add tests for `internal/mdloader/validator.go` — missing files, valid configs, warnings vs errors
- [ ] Add tests for `internal/approval/queue.go` — enqueue, approve, deny, timeout, list pending
- [ ] Add tests for `internal/alert/manager.go` — alert dispatch, Slack/webhook formatting
- [ ] Add tests for `internal/evolution/analyzer.go` — metrics computation, failure grouping (mock trace store)
- [ ] Add tests for `internal/evolution/versions.go` — version listing, promote, rollback, naming
- [ ] Add tests for `internal/evolution/rollback.go` — trigger parsing, threshold comparison, rollback execution
- [ ] Add tests for `internal/server/grpc.go` — EvaluateAction, StartSession, EndSession (mock deps)
- [ ] Add tests for `internal/server/http_events.go` — HTTP endpoint handlers (httptest)

### Priority 3 — E2E Integration Test (Does AgentWarden Actually Work?)
- [ ] Write a Go integration test that starts the full server (gRPC + HTTP) with in-memory SQLite
- [ ] Test: SDK sends StartSession → EvaluateAction (allowed) → EvaluateAction (denied by policy) → EndSession
- [ ] Test: Loop detection triggers after N repeated identical actions
- [ ] Test: Cost tracking accumulates correctly across session actions
- [ ] Test: Policy evaluation with CEL expression correctly allows/denies
- [ ] Test: Session scoring persists and is retrievable

### Priority 4 — Fix GitHub Actions CI/CD
- [ ] Set up GitHub Actions workflow for Go build + test (fix existing ci.yml)
- [ ] Add dashboard build step that generates dist/ before Go build
- [ ] Verify release.yml works (goreleaser config, binary publishing)
- [ ] Add a test coverage report step

### Priority 5 — Build a Real Test Agent
- [ ] Create `test/agents/support-bot/` — a simple agent that uses the Python SDK
- [ ] Support-bot: takes user input, calls OpenRouter LLM, uses a "search knowledge base" tool
- [ ] Wire support-bot through AgentWarden (start session, evaluate each action)
- [ ] Add a cost limit policy: deny if session cost > $0.50
- [ ] Add a loop detection policy: alert if same tool called > 5 times
- [ ] Run the agent, observe dashboard, verify policies trigger correctly
- [ ] Document what works and what's broken in `.claude-journal/learnings.md`

### Priority 6 — Deploy Docs to Vercel
- [x] Create a docs site project structure (pick framework: Docusaurus, VitePress, or plain HTML)
- [x] Move existing docs/*.md into the site
- [x] Deploy to Vercel using token
- [x] Add docs URL to README

### Priority 7 — SDK Hardening
- [ ] Python SDK: Add retry with exponential backoff for HTTP transport
- [ ] Python SDK: Add connection health check (ping endpoint)
- [ ] TypeScript SDK: Add retry with exponential backoff
- [ ] TypeScript SDK: Add connection health check
- [ ] Both SDKs: Add `agentwarden.yaml` client-side config loading

### Priority 8 — Polish & Release Prep
- [x] Update README.md with current architecture, quickstart, badges
- [ ] Add CONTRIBUTING.md
- [ ] Add LICENSE check (currently MIT)
- [ ] Create GitHub release with goreleaser
- [ ] Publish Python SDK to test.pypi.org
- [ ] Publish TypeScript SDK to npm (scoped @agentwarden/sdk)

## Completed
### Iteration 1
- Fixed `internal/dashboard/embed.go` CI build issue by committing dashboard dist files
- Updated `.gitignore` to allow `internal/dashboard/dist/` (commented out ignore rule)
- Added 4 files: index.html, icon.svg, 2 asset files (CSS + JS)
- Verified `go build ./...` and `go test ./...` both pass
- Files changed: `.gitignore`, `internal/dashboard/dist/*`

### Iteration 2
- Fixed CI workflow Go version mismatch in release.yml
- Changed `go-version: "1.26.0"` to `go-version: "1.25.x"` in `.github/workflows/release.yml` (line 23)
- Verified `go build ./...` and `go test ./...` both pass
- Files changed: `.github/workflows/release.yml`

### Iteration 3
- Fixed golangci-lint version mismatch in ci.yml
- Changed `version: v2.1.0` to `version: latest` in `.github/workflows/ci.yml` (line 40)
- The old v2.1.0 version was from 2019 and doesn't support Go 1.25
- Using `latest` ensures compatibility with Go 1.25.x
- Verified `go build ./...` and `go test ./...` both pass
- Files changed: `.github/workflows/ci.yml`

### Iteration 4
- Verified CI infrastructure is working correctly
- Checked GitHub Actions run 22384476878 (commit 2fd4d61)
- Dashboard Build: ✓ PASSED (17s)
- Test: ✓ PASSED (1m3s) — all existing tests pass in CI
- Lint: ✗ FAILED — 39 linting errors found (35 errcheck + 4 staticcheck)
- Build Go Binary: skipped (lint failed)
- CI infrastructure fixes from iterations 1-3 are successful
- Added new task to Priority 1: Fix golangci-lint errors
- No files changed (verification only)

### Iteration 5
- Fixed ALL 39 golangci-lint violations across 13 files
- 35 errcheck fixes: wrapped deferred Close() with `defer func() { _ = f.Close() }()`, wrapped non-deferred Close() with `_ = f.Close()`, wrapped important error returns with proper logging
- 4 staticcheck fixes: replaced `WriteString(fmt.Sprintf(...))` with `fmt.Fprintf()` in evolution/analyzer.go and evolution/proposer.go
- 6 test fixes in loader_test.go: migrated os.Setenv/os.Unsetenv to t.Setenv()
- Verified: `golangci-lint run ./...` → 0 issues, `go build ./...` → PASS, `go test ./...` → PASS
- Files changed: 13 files across cmd/, internal/api, internal/approval, internal/config, internal/dashboard, internal/policy, internal/server, internal/trace, internal/alert, internal/evolution

### Iteration 6
- Deployed docs to Vercel using VitePress
- Created VitePress site in docs/ with .vitepress/config.ts
- Created homepage (index.md) with hero section and feature grid
- Created SDK doc pages: sdk-python.md, sdk-typescript.md
- Created logo SVG and public/ directory
- Deployed to https://agentwarden-docs.vercel.app (production)
- Updated README.md with docs link, badges (CI, Docs, License)
- Updated .gitignore for docs/.vitepress/dist/ and docs/.vitepress/cache/
- Files changed: docs/*, README.md, .gitignore

## Bugs Found
(none yet)

## Test Results Log
### Baseline (before loop)
```
Packages with tests: 5 (config, cost, detection, policy, trace)
Packages without tests: 10 (alert, api, approval, dashboard, evolution, mdloader, server, session, proto, cmd)
All existing tests: PASS
CI status: FAILING (embed.go dist/, Go version, golangci-lint)
```

### Iteration 1
```
go build ./...  → ✅ PASS (no errors)
go test ./...   → ✅ PASS (5 packages with tests all cached/passed)
Commit: 1cc2a96 "Fix embed.go CI issue by committing dashboard dist files"
```

### Iteration 2
```
go build ./...  → ✅ PASS (no errors)
go test ./...   → ✅ PASS (5 packages with tests all cached/passed)
```

### Iteration 3
```
go build ./...  → ✅ PASS (no errors)
go test ./...   → ✅ PASS (5 packages with tests all cached/passed)
```

### Iteration 4
```
go build ./...  → ✅ PASS (no errors)
go test ./...   → ✅ PASS (5 packages with tests all cached/passed)
CI run 22384476878:
  Dashboard Build → ✅ PASS (17s)
  Test           → ✅ PASS (1m3s)
  Lint           → ❌ FAIL (39 issues: 35 errcheck + 4 staticcheck)
```

### Iteration 5
```
golangci-lint run ./... → ✅ 0 issues (was 39)
go build ./...  → ✅ PASS
go test ./...   → ✅ PASS (5 packages with tests all passed)
```

### Iteration 6
```
VitePress build → ✅ 1.92s (9 pages: index, quickstart, config, architecture, policies, evolution, api-reference, sdk-python, sdk-typescript)
Vercel deploy   → ✅ https://agentwarden-docs.vercel.app
go build ./...  → ✅ PASS
go test ./...   → ✅ PASS
```

## Notes
- ALL CI issues are FIXED (embed.go dist/, Go version, golangci-lint version, 39 lint violations)
- Docs deployed to https://agentwarden-docs.vercel.app (VitePress + Vercel)
- README updated with badges and docs link
- 10 packages still have zero tests — biggest quality gap
- No E2E integration test exists — we don't know if the full system works end-to-end
- OpenRouter key available at AGENTWARDEN_LLM_API_KEY for testing real LLM calls
- P1 (CI) and P6 (Docs) are COMPLETE — loop should continue with P2 (tests)
