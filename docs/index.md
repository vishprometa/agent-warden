---
layout: home

hero:
  name: AgentWarden
  text: Deploy governance for your AI agents
  tagline: Kill switches, policy enforcement, cost tracking, and audit trails. One binary, zero code changes.
  actions:
    - theme: brand
      text: Get Started
      link: /quickstart
    - theme: alt
      text: GitHub
      link: https://github.com/vishprometa/agent-warden

features:
  - title: Kill Switch
    details: Emergency stop that operates outside the LLM context window. Cannot be bypassed by prompt injection or context compaction.
    link: /openclaw#kill-switch
    linkText: Learn more
  - title: Capability Scoping
    details: Per-agent boundaries for filesystem, shell, network, messaging, and financial actions. Enforced at the proxy level.
    link: /openclaw#capability-scoping
    linkText: Learn more
  - title: Cost Tracking
    details: Per-session and per-agent cost tracking with hard limits. Supports 20+ models from OpenAI, Anthropic, Google, and more.
    link: /policies#budget-policies
    linkText: Learn more
  - title: Spawn Governance
    details: Control agent self-replication with depth limits, child counts, budget inheritance, and cascade kill.
    link: /openclaw#spawn-governance
    linkText: Learn more
  - title: Anomaly Detection
    details: Automatic detection of action loops, cost velocity spikes, conversation spirals, and rapid-fire runaway behavior.
    link: /configuration#detection
    linkText: Learn more
  - title: Policy Engine
    details: CEL-based rules for budget limits, rate limiting, action blocking, and human approval gates. Hot-reload without restarts.
    link: /policies
    linkText: Learn more
---

<div class="home-content">

<div class="terminal-preview">
  <div class="terminal-header">
    <div class="terminal-dots">
      <span class="dot dot-red"></span>
      <span class="dot dot-yellow"></span>
      <span class="dot dot-green"></span>
    </div>
    <span class="terminal-title">terminal</span>
  </div>
  <div class="terminal-body">
    <div class="line"><span class="accent">$</span> brew install agentwarden</div>
    <div class="line dim">Installing agentwarden...</div>
    <div class="line success">✓ Installed agentwarden v0.9.0</div>
    <div class="line mt"><span class="accent">$</span> agentwarden start</div>
    <div class="line dim">Proxy listening on <span class="accent">:6777</span></div>
    <div class="line dim">Dashboard at <span class="accent">http://localhost:6777/dashboard</span></div>
    <div class="line dim">5 detectors active, fail-closed mode</div>
    <div class="line success mt">✓ Ready — point your agent at localhost:6777</div>
  </div>
</div>

<div class="how-it-works">

## How it works

AgentWarden sits between your AI agents and the outside world as a transparent proxy. Every action is evaluated, traced, and governed.

<div class="steps">
  <div class="step">
    <div class="step-num">01</div>
    <h4>Install & Start</h4>
    <p>One binary, zero dependencies. Start the proxy and dashboard with a single command. Sensible defaults out of the box.</p>
  </div>
  <div class="step">
    <div class="step-num">02</div>
    <h4>Point Your Agent</h4>
    <p>Set your LLM client's base URL to the AgentWarden proxy. No SDK required — works with any language or framework.</p>
  </div>
  <div class="step">
    <div class="step-num">03</div>
    <h4>Govern & Observe</h4>
    <p>Every request flows through the policy engine, anomaly detectors, and trace store. Kill runaway agents, enforce budgets, and audit everything.</p>
  </div>
</div>
</div>

<div class="architecture-preview">

```
Agent ──► AgentWarden ──► LLM API / Tools / APIs
               │
               ├── Kill Switch    (hard stop)
               ├── Capabilities   (scope checks)
               ├── Policy Engine  (CEL rules)
               ├── Detection      (loops, velocity, spirals, drift)
               ├── Audit Trail    (hash-chained traces)
               └── Dashboard      (real-time visibility)
```

</div>

<div class="cta-section">

## Ready to govern your agents?

Stop worrying about runaway costs, prompt injection, and uncontrolled agent behavior. AgentWarden gives you the kill switch.

<div class="cta-buttons">
  <a href="/quickstart" class="cta-btn cta-primary">Get Started</a>
  <a href="/architecture" class="cta-btn cta-secondary">Architecture Deep Dive</a>
</div>
</div>

</div>
