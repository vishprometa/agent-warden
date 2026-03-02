<script setup>
const year = new Date().getFullYear()

const links = [
  { text: 'Quick Start', href: '/quickstart' },
  { text: 'Policies', href: '/policies' },
  { text: 'Architecture', href: '/architecture' },
  { text: 'API Reference', href: '/api-reference' },
  { text: 'OpenClaw', href: '/openclaw' },
  { text: 'GitHub ↗', href: 'https://github.com/vishprometa/agent-warden', external: true },
]
</script>

<template>
  <footer class="custom-footer">
    <!-- CTA Section -->
    <div class="footer-cta">
      <h2 class="footer-cta-title">Ready to govern your agents?</h2>
      <p class="footer-cta-desc">
        Full governance for your agent infra. One binary, zero code changes.
      </p>
      <a href="/quickstart" class="footer-cta-btn">Get Started</a>
    </div>

    <!-- Links Row -->
    <nav class="footer-links">
      <a
        v-for="link in links"
        :key="link.text"
        :href="link.href"
        :target="link.external ? '_blank' : undefined"
        :rel="link.external ? 'noopener noreferrer' : undefined"
        class="footer-link"
      >
        {{ link.text }}
      </a>
    </nav>

    <!-- Bottom line -->
    <div class="footer-bottom">
      <span class="footer-legal">Released under the MIT License.</span>
      <span class="footer-copyright">&copy; {{ year }} AgentWarden</span>
    </div>

    <!-- Big decorative wordmark -->
    <div class="footer-wordmark" aria-hidden="true">
      <span>AgentWarden</span>
    </div>
  </footer>
</template>

<style scoped>
.custom-footer {
  position: relative;
  overflow: hidden;
  border-top: 1px solid rgba(255, 255, 255, 0.04);
  padding: 5rem 1.5rem 0;
  text-align: center;
}

/* CTA */
.footer-cta {
  max-width: 480px;
  margin: 0 auto 3rem;
}

.footer-cta-title {
  font-family: var(--aw-font-serif);
  font-weight: 400;
  font-size: 1.8rem;
  letter-spacing: -0.02em;
  color: var(--vp-c-text-1);
  margin: 0 0 0.75rem;
}

.footer-cta-desc {
  font-family: var(--aw-font-sans);
  font-size: 0.9rem;
  line-height: 1.6;
  color: var(--vp-c-text-3);
  margin: 0 0 1.5rem;
}

.footer-cta-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.55rem 1.6rem;
  border-radius: 9999px;
  background: var(--vp-c-text-1);
  color: var(--vp-c-bg) !important;
  font-family: var(--aw-font-sans);
  font-weight: 500;
  font-size: 0.82rem;
  text-decoration: none !important;
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.footer-cta-btn:hover {
  opacity: 0.88;
  transform: translateY(-1px);
}

/* Links */
.footer-links {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 1.25rem 2rem;
  margin: 0 auto 1.5rem;
  max-width: 640px;
}

.footer-link {
  font-family: var(--aw-font-sans);
  font-size: 0.82rem;
  font-weight: 400;
  color: var(--vp-c-text-2);
  text-decoration: none !important;
  transition: color 0.15s ease;
}

.footer-link:hover {
  color: var(--vp-c-text-1);
}

/* Bottom */
.footer-bottom {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.5rem 1.5rem;
  margin-bottom: 2rem;
}

.footer-legal,
.footer-copyright {
  font-family: var(--aw-font-sans);
  font-size: 0.72rem;
  color: var(--vp-c-text-3);
}

/* Wordmark */
.footer-wordmark {
  position: relative;
  line-height: 0.85;
  margin: 0 -1.5rem;
  overflow: hidden;
  padding-bottom: 0.5rem;
}

.footer-wordmark span {
  display: block;
  font-family: var(--aw-font-serif);
  font-weight: 400;
  font-size: clamp(5rem, 15vw, 12rem);
  letter-spacing: -0.02em;
  background: linear-gradient(
    180deg,
    rgba(255, 255, 255, 0.12) 0%,
    rgba(255, 255, 255, 0.02) 100%
  );
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  user-select: none;
  white-space: nowrap;
}

/* Light mode */
:root:not(.dark) .custom-footer {
  border-top-color: rgba(0, 0, 0, 0.06);
}

:root:not(.dark) .footer-wordmark span {
  background: linear-gradient(
    180deg,
    rgba(0, 0, 0, 0.1) 0%,
    rgba(0, 0, 0, 0.02) 100%
  );
  -webkit-background-clip: text;
  background-clip: text;
}
</style>
