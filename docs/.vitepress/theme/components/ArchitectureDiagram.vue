<script setup>
import { ref, onMounted } from 'vue'

const container = ref(null)
const visible = ref(false)

const diagram = `graph LR
    Agent["🤖 AI Agent"] -->|HTTP| Proxy

    subgraph AW["AgentWarden :6777"]
        direction TB

        subgraph Intercept["Interception Layer"]
            Proxy["Reverse Proxy"] --> Classify["Classifier"]
            Classify --> Router["Router"]
        end

        subgraph Governance["Governance Engine"]
            KS["🛑 Kill Switch"]
            Policy["⚙️ CEL Policies"]
            Budget["💰 Budget Gates"]
            Approve["✋ Approval Queue"]
        end

        subgraph Detection["Anomaly Detection"]
            Loop["Loop"]
            Spiral["Spiral"]
            Drift["Drift"]
            Velocity["Velocity"]
            CostSpike["Cost Spike"]
        end

        subgraph Observe["Observability"]
            Trace["🔗 Hash-Chained Traces"]
            Dash["📊 Live Dashboard"]
            Alert["🔔 Alerts"]
        end

        Classify --> KS
        Classify --> Policy
        Classify --> Budget
        Classify --> Approve
        Router --> Trace
        Trace --> Loop
        Trace --> Spiral
        Trace --> Drift
        Trace --> Velocity
        Trace --> CostSpike
        Trace --> Dash
        Loop --> Alert
        Spiral --> Alert
        Drift --> Alert
        Velocity --> Alert
        CostSpike --> Alert
    end

    Router -->|"route"| OpenAI["OpenAI"]
    Router -->|"route"| Anthropic["Anthropic"]
    Router -->|"route"| Gemini["Gemini"]

    Alert -->|"notify"| Ops["👤 Ops Team"]
    Dash -->|"view"| Ops

    style AW fill:#1a1a2e,stroke:#f97316,stroke-width:2px,color:#fff
    style Intercept fill:#0f172a,stroke:#3b82f6,stroke-width:1px,color:#93c5fd
    style Governance fill:#1a0a0a,stroke:#ef4444,stroke-width:1px,color:#fca5a5
    style Detection fill:#0a0a1a,stroke:#6366f1,stroke-width:1px,color:#a5b4fc
    style Observe fill:#0a1a0a,stroke:#22c55e,stroke-width:1px,color:#86efac
    style Agent fill:#1c1917,stroke:#f87171,color:#fca5a5
    style OpenAI fill:#0f172a,stroke:#3b82f6,color:#93c5fd
    style Anthropic fill:#0f172a,stroke:#3b82f6,color:#93c5fd
    style Gemini fill:#0f172a,stroke:#3b82f6,color:#93c5fd
    style Ops fill:#1c1917,stroke:#eab308,color:#fde68a
    style KS fill:#1a0505,stroke:#ef4444,color:#fca5a5
    style Policy fill:#1a0a00,stroke:#f97316,color:#fdba74
    style Budget fill:#1a0a00,stroke:#f97316,color:#fdba74
    style Approve fill:#0f0a1a,stroke:#a855f7,color:#c4b5fd
    style Proxy fill:#0f172a,stroke:#3b82f6,color:#93c5fd
    style Classify fill:#0f172a,stroke:#3b82f6,color:#93c5fd
    style Router fill:#0f172a,stroke:#3b82f6,color:#93c5fd
    style Trace fill:#0a1a0a,stroke:#22c55e,color:#86efac
    style Dash fill:#0a1a0a,stroke:#22c55e,color:#86efac
    style Alert fill:#1a1a00,stroke:#eab308,color:#fde68a
    style Loop fill:#0a0a1a,stroke:#6366f1,color:#a5b4fc
    style Spiral fill:#0a0a1a,stroke:#6366f1,color:#a5b4fc
    style Drift fill:#0a0a1a,stroke:#6366f1,color:#a5b4fc
    style Velocity fill:#0a0a1a,stroke:#6366f1,color:#a5b4fc
    style CostSpike fill:#0a0a1a,stroke:#6366f1,color:#a5b4fc`

onMounted(async () => {
  try {
    const mermaid = (await import('mermaid')).default
    mermaid.initialize({
      startOnLoad: false,
      theme: 'dark',
      themeVariables: {
        darkMode: true,
        background: '#09090b',
        primaryColor: '#1a1a2e',
        primaryTextColor: '#e4e4e7',
        primaryBorderColor: '#f97316',
        lineColor: '#525252',
        secondaryColor: '#0f172a',
        tertiaryColor: '#1a0a0a',
        fontFamily: 'Inter, sans-serif',
        fontSize: '12px',
      },
      flowchart: {
        curve: 'basis',
        padding: 12,
        nodeSpacing: 30,
        rankSpacing: 40,
      },
    })
    const { svg } = await mermaid.render('arch-mermaid', diagram)
    if (container.value) {
      container.value.innerHTML = svg
    }
    setTimeout(() => { visible.value = true }, 50)
  } catch (e) {
    console.warn('Mermaid render failed:', e)
  }
})
</script>

<template>
  <div class="arch-wrapper" :class="{ visible }">
    <div ref="container" class="arch-container"></div>
  </div>
</template>

<style scoped>
.arch-wrapper {
  max-width: 1152px;
  margin: 0 auto;
  padding: 0 24px 2rem;
  opacity: 0;
  transform: translateY(10px);
  transition: opacity 0.5s ease, transform 0.5s ease;
}

.arch-wrapper.visible {
  opacity: 1;
  transform: translateY(0);
}

.arch-container {
  overflow-x: auto;
  scrollbar-width: none;
}

.arch-container::-webkit-scrollbar {
  display: none;
}

.arch-container :deep(svg) {
  width: 100%;
  min-width: 700px;
  height: auto;
}
</style>
