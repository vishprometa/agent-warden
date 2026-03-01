import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'

export default withMermaid(defineConfig({
  title: 'AgentWarden',
  description: 'Runtime governance for AI agents. Observe. Enforce. Evolve.',
  appearance: 'dark',

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/logo.svg' }],
    ['meta', { name: 'theme-color', content: '#09090b' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'AgentWarden — Runtime Governance for AI Agents' }],
    ['meta', { property: 'og:description', content: 'Deploy governance for your AI agents. Kill switches, policy enforcement, cost tracking, and audit trails.' }],
    ['meta', { property: 'og:image', content: '/logo.svg' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    ['link', { href: 'https://fonts.googleapis.com/css2?family=Instrument+Serif&family=Inter:wght@300;400;500;600;700&display=swap', rel: 'stylesheet' }],
  ],

  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'AgentWarden',

    nav: [
      { text: 'Docs', link: '/quickstart' },
      { text: 'Policies', link: '/policies' },
      { text: 'API', link: '/api-reference' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        collapsed: false,
        items: [
          { text: 'Quick Start', link: '/quickstart' },
          { text: 'Configuration', link: '/configuration' },
        ]
      },
      {
        text: 'Governance',
        collapsed: false,
        items: [
          { text: 'OpenClaw Integration', link: '/openclaw' },
          { text: 'Policies', link: '/policies' },
          { text: 'Architecture', link: '/architecture' },
        ]
      },
      {
        text: 'Advanced',
        collapsed: false,
        items: [
          { text: 'Self-Evolution', link: '/evolution' },
        ]
      },
      {
        text: 'Reference',
        collapsed: false,
        items: [
          { text: 'API Reference', link: '/api-reference' },
          { text: 'Python SDK', link: '/sdk-python' },
          { text: 'TypeScript SDK', link: '/sdk-typescript' },
        ]
      }
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/vishprometa/agent-warden' },
    ],

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright \u00A9 2025-present AgentWarden',
    },

    search: {
      provider: 'local',
    },

    editLink: {
      pattern: 'https://github.com/vishprometa/agent-warden/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },

    outline: {
      level: [2, 3],
      label: 'On this page',
    },

    lastUpdated: {
      text: 'Last updated',
    },
  },

  mermaid: {
    theme: 'dark',
  },
}))
