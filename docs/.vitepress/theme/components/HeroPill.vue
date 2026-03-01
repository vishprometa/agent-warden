<script setup>
</script>

<template>
  <div class="hero-pill-wrapper">
    <a href="/openclaw" class="hero-pill">
      <span class="hero-pill-badge">New</span>
      <span class="hero-pill-text">OpenClaw Integration</span>
      <span class="hero-pill-arrow">→</span>
    </a>
  </div>
</template>

<style scoped>
.hero-pill-wrapper {
  display: flex;
  justify-content: center;
  margin-bottom: 1.5rem;
}

.hero-pill {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 5px 14px 5px 5px;
  border-radius: 9999px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.03);
  text-decoration: none !important;
  font-family: var(--aw-font-sans);
  font-size: 0.8rem;
  color: var(--vp-c-text-2);
  transition: border-color 0.2s ease, background 0.2s ease;
  cursor: pointer;
}

.hero-pill:hover {
  border-color: rgba(255, 255, 255, 0.15);
  background: rgba(255, 255, 255, 0.05);
  color: var(--vp-c-text-1);
}

.hero-pill-badge {
  padding: 2px 10px;
  border-radius: 9999px;
  background: var(--vp-c-brand-1);
  color: #fff;
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}

.hero-pill-text {
  font-weight: 500;
}

.hero-pill-arrow {
  color: var(--vp-c-text-3);
  transition: transform 0.15s ease;
}

.hero-pill:hover .hero-pill-arrow {
  transform: translateX(2px);
}
</style>
