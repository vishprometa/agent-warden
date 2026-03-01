// Build-time data loader: indexes all .md doc files into searchable chunks.
// Each chunk is a section (split on ## headings) with metadata.
// Usage at runtime: import { data } from './doc-index.data'

import { createContentLoader } from 'vitepress'

interface DocChunk {
  title: string      // page title (first H1 or filename)
  path: string       // URL path, e.g. "/policies"
  heading: string    // section heading (## text) or "Introduction"
  content: string    // raw markdown content of the section
}

declare const data: DocChunk[]
export { data }

export default createContentLoader('**/*.md', {
  includeSrc: true,
  transform(rawData): DocChunk[] {
    const chunks: DocChunk[] = []

    for (const page of rawData) {
      const src = page.src || ''
      const url = page.url || ''

      // Extract page title from frontmatter or first H1
      let pageTitle = ''
      if (page.frontmatter?.title) {
        pageTitle = page.frontmatter.title
      } else {
        const h1Match = src.match(/^#\s+(.+)$/m)
        if (h1Match) {
          pageTitle = h1Match[1].trim()
        } else {
          pageTitle = url.replace(/^\//, '').replace(/\/$/, '') || 'index'
        }
      }

      // Skip the home layout page content (hero/features are in frontmatter)
      if (page.frontmatter?.layout === 'home') {
        // Still index the home page but with frontmatter content
        const heroText = page.frontmatter?.hero?.tagline || ''
        const features = page.frontmatter?.features || []
        const featureText = features
          .map((f: any) => `${f.title}: ${f.details}`)
          .join('\n')

        if (heroText || featureText) {
          chunks.push({
            title: pageTitle || 'AgentWarden',
            path: url,
            heading: 'Overview',
            content: [heroText, featureText].filter(Boolean).join('\n\n'),
          })
        }

        // Also index any content below the frontmatter
        const contentAfterFrontmatter = src.replace(/^---[\s\S]*?---/, '').trim()
        if (contentAfterFrontmatter) {
          splitIntoSections(contentAfterFrontmatter, pageTitle, url, chunks)
        }
        continue
      }

      // Remove frontmatter
      const content = src.replace(/^---[\s\S]*?---/, '').trim()
      if (!content) continue

      splitIntoSections(content, pageTitle, url, chunks)
    }

    return chunks
  }
})

function splitIntoSections(
  content: string,
  pageTitle: string,
  url: string,
  chunks: DocChunk[]
) {
  // Split on ## headings
  const sections = content.split(/^(?=## )/m)

  for (const section of sections) {
    const trimmed = section.trim()
    if (!trimmed) continue

    // Extract heading
    const headingMatch = trimmed.match(/^##\s+(.+)$/m)
    const heading = headingMatch ? headingMatch[1].trim() : 'Introduction'

    // Remove the heading line from content
    const body = headingMatch
      ? trimmed.replace(/^##\s+.+$/m, '').trim()
      : trimmed

    if (!body) continue

    // Truncate very long sections to keep index manageable
    const maxLen = 2000
    const truncated = body.length > maxLen
      ? body.slice(0, maxLen) + '...'
      : body

    chunks.push({
      title: pageTitle,
      path: url,
      heading,
      content: truncated,
    })
  }
}
