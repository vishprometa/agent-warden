<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { data as docIndex } from '../doc-index.data'

// ── State ──────────────────────────────────────────────────

const isOpen = ref(false)
const apiKey = ref('')
const hasKey = ref(false)
const messages = ref<Array<{ role: string; content: string; type?: string }>>([])
const input = ref('')
const isLoading = ref(false)
const error = ref('')
const messagesEl = ref<HTMLElement | null>(null)
const inputEl = ref<HTMLElement | null>(null)

// Check localStorage on mount
onMounted(() => {
  const stored = localStorage.getItem('aw-ai-api-key')
  if (stored) {
    apiKey.value = stored
    hasKey.value = true
  }
})

// Keyboard shortcut
function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) {
    isOpen.value = false
  }
}

onMounted(() => document.addEventListener('keydown', handleKeydown))
onUnmounted(() => document.removeEventListener('keydown', handleKeydown))

// ── API Key ────────────────────────────────────────────────

function saveKey() {
  if (!apiKey.value.trim()) return
  localStorage.setItem('aw-ai-api-key', apiKey.value.trim())
  hasKey.value = true
  error.value = ''
}

function clearKey() {
  localStorage.removeItem('aw-ai-api-key')
  apiKey.value = ''
  hasKey.value = false
  messages.value = []
}

// ── Tools ──────────────────────────────────────────────────

// OpenRouter uses OpenAI-compatible tool format
const tools = [
  {
    type: 'function' as const,
    function: {
      name: 'search_docs',
      description: 'Search through AgentWarden documentation. Returns matching sections with their page path and content.',
      parameters: {
        type: 'object',
        properties: {
          query: {
            type: 'string',
            description: 'Search query — keywords or a question about AgentWarden'
          }
        },
        required: ['query']
      }
    }
  },
  {
    type: 'function' as const,
    function: {
      name: 'get_page_content',
      description: 'Get the full content of a specific documentation page by its URL path.',
      parameters: {
        type: 'object',
        properties: {
          path: {
            type: 'string',
            description: 'URL path of the doc page, e.g. "/policies", "/api-reference", "/quickstart"'
          }
        },
        required: ['path']
      }
    }
  },
  {
    type: 'function' as const,
    function: {
      name: 'list_pages',
      description: 'List all available documentation pages with their titles and paths.',
      parameters: {
        type: 'object',
        properties: {},
        required: []
      }
    }
  }
]

function executeTool(name: string, input: any): string {
  switch (name) {
    case 'search_docs': {
      const query = (input.query || '').toLowerCase()
      const terms = query.split(/\s+/).filter(Boolean)
      if (!terms.length) return JSON.stringify([])

      // Simple BM25-ish scoring
      const scored = docIndex.map(chunk => {
        const text = `${chunk.title} ${chunk.heading} ${chunk.content}`.toLowerCase()
        let score = 0
        for (const term of terms) {
          const idx = text.indexOf(term)
          if (idx !== -1) {
            score += 1
            // Boost title/heading matches
            if (chunk.title.toLowerCase().includes(term)) score += 3
            if (chunk.heading.toLowerCase().includes(term)) score += 2
          }
        }
        return { ...chunk, score }
      }).filter(r => r.score > 0)
        .sort((a, b) => b.score - a.score)
        .slice(0, 5)

      return JSON.stringify(scored.map(r => ({
        title: r.title,
        path: r.path,
        heading: r.heading,
        content: r.content.slice(0, 800),
        score: r.score
      })))
    }

    case 'get_page_content': {
      const path = (input.path || '').replace(/\/$/, '')
      const normalizedPath = path.endsWith('.html') ? path : path
      const sections = docIndex.filter(chunk => {
        const chunkPath = chunk.path.replace(/\/$/, '').replace('.html', '')
        return chunkPath === path || chunkPath === normalizedPath
      })
      if (!sections.length) {
        return JSON.stringify({ error: `No page found at path: ${path}` })
      }
      return JSON.stringify(sections.map(s => ({
        heading: s.heading,
        content: s.content
      })))
    }

    case 'list_pages': {
      const pages = new Map<string, string>()
      for (const chunk of docIndex) {
        if (!pages.has(chunk.path)) {
          pages.set(chunk.path, chunk.title)
        }
      }
      return JSON.stringify(
        Array.from(pages.entries()).map(([path, title]) => ({ path, title }))
      )
    }

    default:
      return JSON.stringify({ error: `Unknown tool: ${name}` })
  }
}

// ── Chat ───────────────────────────────────────────────────

const SYSTEM_PROMPT = `You are the AgentWarden documentation assistant. AgentWarden is a runtime governance sidecar for AI agents — it provides kill switches, policy enforcement, cost tracking, capability scoping, anomaly detection, and audit trails.

Use your tools to search documentation before answering questions. Always cite specific doc pages using markdown links like [Page Title](/path). Be concise and direct. Use code examples when helpful.

If you cannot find an answer in the docs, say so honestly.`

async function send() {
  const text = input.value.trim()
  if (!text || isLoading.value) return

  input.value = ''
  messages.value.push({ role: 'user', content: text })
  error.value = ''
  isLoading.value = true
  scrollToBottom()

  // Build messages for the API (only user and assistant messages)
  const apiMessages: any[] = []
  for (const msg of messages.value) {
    if (msg.role === 'user') {
      apiMessages.push({ role: 'user', content: msg.content })
    } else if (msg.role === 'assistant') {
      apiMessages.push({ role: 'assistant', content: msg.content })
    }
  }

  try {
    await runAgentLoop(apiMessages)
  } catch (e: any) {
    error.value = e.message || 'Something went wrong'
    if (e.message?.includes('401') || e.message?.includes('invalid')) {
      error.value = 'Invalid API key. Please update your key.'
    }
  } finally {
    isLoading.value = false
    scrollToBottom()
  }
}

async function runAgentLoop(apiMessages: any[]) {
  // Agentic tool-use loop via OpenRouter (OpenAI-compatible format)
  let maxIterations = 10
  let iteration = 0

  while (iteration < maxIterations) {
    iteration++

    const response = await fetch('https://openrouter.ai/api/v1/chat/completions', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${apiKey.value}`,
        'HTTP-Referer': window.location.origin,
        'X-Title': 'AgentWarden Docs',
      },
      body: JSON.stringify({
        model: 'anthropic/claude-sonnet-4',
        max_tokens: 1024,
        messages: [
          { role: 'system', content: SYSTEM_PROMPT },
          ...apiMessages,
        ],
        tools,
      }),
    })

    if (!response.ok) {
      const errText = await response.text()
      throw new Error(`API error ${response.status}: ${errText}`)
    }

    const data = await response.json()
    const choice = data.choices?.[0]
    if (!choice) throw new Error('No response from model')

    const msg = choice.message
    const textContent = msg.content || ''
    const toolCalls = msg.tool_calls || []

    // If there are tool calls, execute them and continue the loop
    if (toolCalls.length > 0) {
      // Add the assistant's message (with tool_calls) to conversation
      apiMessages.push({
        role: 'assistant',
        content: textContent || null,
        tool_calls: toolCalls,
      })

      // Execute each tool and add results
      for (const tc of toolCalls) {
        const fnName = tc.function.name
        const fnArgs = JSON.parse(tc.function.arguments || '{}')

        // Show tool usage in UI
        messages.value.push({
          role: 'tool',
          content: `Searching: ${fnName}(${JSON.stringify(fnArgs)})`,
          type: 'tool',
        })
        scrollToBottom()

        const result = executeTool(fnName, fnArgs)

        // Add tool result in OpenAI format
        apiMessages.push({
          role: 'tool',
          tool_call_id: tc.id,
          content: result,
        })
      }
      continue
    }

    // No tool calls — we have a final text response
    if (textContent) {
      messages.value.push({ role: 'assistant', content: textContent })
    }
    break
  }
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesEl.value) {
      messagesEl.value.scrollTop = messagesEl.value.scrollHeight
    }
  })
}

function toggle() {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    nextTick(() => inputEl.value?.focus())
  }
}

// Simple markdown-ish rendering (bold, code, links, paragraphs)
function renderMarkdown(text: string): string {
  return text
    // Code blocks
    .replace(/```(\w*)\n([\s\S]*?)```/g, '<pre><code>$2</code></pre>')
    // Inline code
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    // Bold
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    // Links
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2">$1</a>')
    // Paragraphs
    .split('\n\n')
    .map(p => p.trim())
    .filter(Boolean)
    .map(p => p.startsWith('<pre>') ? p : `<p>${p}</p>`)
    .join('')
    // Line breaks within paragraphs
    .replace(/\n/g, '<br>')
}

const canSend = computed(() => input.value.trim().length > 0 && !isLoading.value)
</script>

<template>
  <!-- Floating trigger button -->
  <button
    v-if="!isOpen"
    class="ai-agent-trigger"
    @click="toggle"
    aria-label="Open AI assistant"
  >
    <span class="ai-icon"></span>
    Ask AI
  </button>

  <!-- Chat panel -->
  <div v-if="isOpen" class="ai-agent-panel">
    <!-- Header -->
    <div class="ai-agent-header">
      <div class="ai-agent-header-left">
        <span class="ai-agent-header-dot"></span>
        <span class="ai-agent-header-title">AgentWarden AI</span>
      </div>
      <div style="display: flex; gap: 4px;">
        <button
          v-if="hasKey"
          class="ai-agent-header-btn"
          @click="clearKey"
          title="Change API key"
        >&#9881;</button>
        <button
          class="ai-agent-header-btn"
          @click="isOpen = false"
          aria-label="Close"
        >&times;</button>
      </div>
    </div>

    <!-- API Key Setup -->
    <div v-if="!hasKey" class="ai-agent-setup">
      <div class="ai-agent-setup-title">Connect via OpenRouter</div>
      <div class="ai-agent-setup-desc">
        Enter your OpenRouter API key to enable the AI assistant.
        Your key is stored locally and never sent to our servers.
      </div>
      <input
        v-model="apiKey"
        class="ai-agent-setup-input"
        type="password"
        placeholder="sk-or-v1-..."
        @keyup.enter="saveKey"
      />
      <button
        class="ai-agent-setup-btn"
        :disabled="!apiKey.trim()"
        @click="saveKey"
      >
        Connect
      </button>
    </div>

    <!-- Chat Messages -->
    <template v-else>
      <div ref="messagesEl" class="ai-agent-messages">
        <!-- Welcome message -->
        <div v-if="messages.length === 0" class="ai-msg ai-msg-assistant">
          <p>Hi! I can help you with AgentWarden — policies, configuration, APIs, SDKs, and more.</p>
          <p>Try asking something like:</p>
          <p>
            <em>"How do I set up a cost budget policy?"</em><br>
            <em>"What API endpoint lists sessions?"</em><br>
            <em>"How does the kill switch work?"</em>
          </p>
        </div>

        <template v-for="(msg, i) in messages" :key="i">
          <div v-if="msg.type === 'tool'" class="ai-msg ai-msg-tool">
            {{ msg.content }}
          </div>
          <div
            v-else-if="msg.role === 'user'"
            class="ai-msg ai-msg-user"
          >
            {{ msg.content }}
          </div>
          <div
            v-else-if="msg.role === 'assistant'"
            class="ai-msg ai-msg-assistant"
            v-html="renderMarkdown(msg.content)"
          />
        </template>

        <!-- Loading indicator -->
        <div v-if="isLoading" class="ai-msg-thinking">
          <span></span><span></span><span></span>
        </div>

        <!-- Error -->
        <div v-if="error" class="ai-agent-error">{{ error }}</div>
      </div>

      <!-- Input -->
      <div class="ai-agent-input-area">
        <div class="ai-agent-input-wrap">
          <input
            ref="inputEl"
            v-model="input"
            class="ai-agent-input"
            placeholder="Ask about policies, config, APIs..."
            @keyup.enter="send"
            :disabled="isLoading"
          />
          <button
            class="ai-agent-send"
            :class="{ active: canSend }"
            @click="send"
            :disabled="!canSend"
            aria-label="Send"
          >&#8593;</button>
        </div>
      </div>
    </template>
  </div>
</template>
