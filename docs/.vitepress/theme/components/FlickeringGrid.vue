<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'

const props = defineProps({
  squareSize: { type: Number, default: 4 },
  gridGap: { type: Number, default: 6 },
  flickerChance: { type: Number, default: 0.3 },
  color: { type: String, default: 'rgb(255, 255, 255)' },
  maxOpacity: { type: Number, default: 0.15 },
})

const containerRef = ref(null)
const canvasRef = ref(null)
let animId = null
let resizeObs = null
let gridState = []

function toRGBA(cssColor) {
  const c = document.createElement('canvas')
  c.width = c.height = 1
  const ctx = c.getContext('2d')
  ctx.fillStyle = cssColor
  ctx.fillRect(0, 0, 1, 1)
  const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data
  return `${r}, ${g}, ${b}`
}

function setupGrid(canvas, container) {
  const dpr = window.devicePixelRatio || 1
  const w = container.clientWidth
  const h = container.clientHeight
  canvas.width = w * dpr
  canvas.height = h * dpr
  canvas.style.width = w + 'px'
  canvas.style.height = h + 'px'

  const ctx = canvas.getContext('2d')
  ctx.scale(dpr, dpr)

  const step = props.squareSize + props.gridGap
  const cols = Math.ceil(w / step) + 1
  const rows = Math.ceil(h / step) + 1

  gridState = []
  for (let i = 0; i < rows; i++) {
    for (let j = 0; j < cols; j++) {
      gridState.push({
        x: j * step,
        y: i * step,
        opacity: Math.random() * props.maxOpacity,
        target: Math.random() * props.maxOpacity,
      })
    }
  }

  return { ctx, w, h }
}

function draw(ctx, w, h) {
  const rgba = toRGBA(props.color)
  ctx.clearRect(0, 0, w, h)
  for (const sq of gridState) {
    // smoothly lerp toward target
    sq.opacity += (sq.target - sq.opacity) * 0.08

    // randomly pick new target
    if (Math.random() < props.flickerChance) {
      sq.target = Math.random() * props.maxOpacity
    }

    ctx.fillStyle = `rgba(${rgba}, ${sq.opacity})`
    ctx.fillRect(sq.x, sq.y, props.squareSize, props.squareSize)
  }
}

onMounted(() => {
  const canvas = canvasRef.value
  const container = containerRef.value
  if (!canvas || !container) return

  let { ctx, w, h } = setupGrid(canvas, container)

  function loop() {
    draw(ctx, w, h)
    animId = requestAnimationFrame(loop)
  }
  loop()

  resizeObs = new ResizeObserver(() => {
    ;({ ctx, w, h } = setupGrid(canvas, container))
  })
  resizeObs.observe(container)
})

onBeforeUnmount(() => {
  if (animId) cancelAnimationFrame(animId)
  if (resizeObs) resizeObs.disconnect()
})
</script>

<template>
  <div ref="containerRef" class="flickering-grid">
    <canvas ref="canvasRef" class="flickering-canvas" />
  </div>
</template>

<style scoped>
.flickering-grid {
  position: fixed;
  inset: 0;
  z-index: 31;
  pointer-events: none;
  overflow: hidden;
  mask-image: radial-gradient(ellipse 80% 60% at 50% 30%, black 0%, transparent 70%);
  -webkit-mask-image: radial-gradient(ellipse 80% 60% at 50% 30%, black 0%, transparent 70%);
}

.flickering-canvas {
  display: block;
  width: 100%;
  height: 100%;
}
</style>
