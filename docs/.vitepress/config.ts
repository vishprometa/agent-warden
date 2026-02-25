import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'AgentWarden',
  description: 'Runtime governance for AI agents. Observe. Enforce. Evolve.',

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/logo.svg' }],
    ['meta', { name: 'theme-color', content: '#6366f1' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:title', content: 'AgentWarden' }],
    ['meta', { property: 'og:description', content: 'Runtime governance for AI agents' }],
  ],

  themeConfig: {
    logo: '/logo.svg',

    nav: [
      { text: 'Guide', link: '/quickstart' },
      { text: 'API Reference', link: '/api-reference' },
      { text: 'GitHub', link: 'https://github.com/vishprometa/agent-warden' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Quick Start', link: '/quickstart' },
          { text: 'Configuration', link: '/configuration' },
        ]
      },
      {
        text: 'Core Concepts',
        items: [
          { text: 'Architecture', link: '/architecture' },
          { text: 'Policies', link: '/policies' },
          { text: 'Self-Evolution', link: '/evolution' },
        ]
      },
      {
        text: 'Reference',
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
      copyright: 'Copyright © 2025-present AgentWarden',
    },

    search: {
      provider: 'local',
    },

    editLink: {
      pattern: 'https://github.com/vishprometa/agent-warden/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
  },
})
