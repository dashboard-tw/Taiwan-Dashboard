<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import type { CountyCollection } from '../../api/client'
import type { Level } from '../../lib/levels'
import { formatNumber } from '../../lib/format'
import {
  countyShapes,
  HEIGHT,
  INSET,
  INSET_HEADER,
  OUTLYING,
  WIDTH,
  type Shape,
} from '../../lib/countyShapes'

const props = defineProps<{
  geo: CountyCollection
  values: Map<string, number>
  levelOf: (v: number) => Level
  decimals?: number
  unit?: string
  selected?: string | null
  focus?: string | null
  label?: string
}>()

const emit = defineEmits<{ select: [code: string] }>()

const shapes = computed(() => countyShapes(props.geo))

const STEP_MS = 35
const ROW_PX = 50

const order = computed(() =>
  [...shapes.value]
    .sort((a, b) => Math.round(a.cy / ROW_PX) - Math.round(b.cy / ROW_PX) || a.cx - b.cx)
    .map((x) => x.code),
)
const shown = shallowRef(new Map<string, number>())
const pulse = ref<string | null>(null)
let stepTimer: number | undefined
const reduced =
  typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches

watch(
  () => props.values,
  (values) => {
    window.clearTimeout(stepTimer)
    if (reduced) {
      shown.value = new Map(values)
      return
    }
    const codes = order.value
    let i = 0
    const step = () => {
      const code = codes[i++]
      pulse.value = code ?? null
      if (code === undefined) return
      const next = new Map(shown.value)
      const v = values.get(code)
      if (v === undefined) next.delete(code)
      else next.set(code, v)
      shown.value = next
      stepTimer = window.setTimeout(step, STEP_MS)
    }
    step()
  },
  { immediate: true },
)
onBeforeUnmount(() => window.clearTimeout(stepTimer))

const insets = computed(() =>
  OUTLYING.map((code, i) => ({
    code,
    name: props.geo.features.find((f) => f.properties.code === code)?.properties.short_name ?? '',
    y: 4 + i * (INSET.h + INSET.gap),
  })),
)

const hovered = ref<string | null>(null)

function fill(code: string): string {
  const v = shown.value.get(code)
  return v === undefined ? 'var(--map-empty)' : props.levelOf(v).color
}

function ink(code: string): string {
  const v = shown.value.get(code)
  return v === undefined ? 'var(--text-muted)' : props.levelOf(v).ink
}

function describe(s: Shape): string {
  const v = shown.value.get(s.code)
  if (v === undefined) return `${s.name}：沒有資料`
  return `${s.name}：${formatNumber(v, props.decimals)}${props.unit ?? ''}（${props.levelOf(v).label}）`
}

function onKey(e: KeyboardEvent, code: string) {
  if (e.key === 'Enter' || e.key === ' ') {
    e.preventDefault()
    emit('select', code)
  }
}

const tipEl = ref<HTMLElement | null>(null)
const tip = ref<{ text: string; left: number; top: number } | null>(null)
const TIP_GAP = 10
const TIP_MARGIN = 8

watch(hovered, async (code) => {
  const s = shapes.value.find((x) => x.code === code)
  const svg = svgEl.value
  if (!s || !svg) {
    tip.value = null
    return
  }
  const r = svg.getBoundingClientRect()
  const scale = Math.min(r.width / WIDTH, r.height / HEIGHT)
  const x = r.left + (r.width - WIDTH * scale) / 2 + s.cx * scale
  const y = r.top + (r.height - HEIGHT * scale) / 2 + s.cy * scale
  tip.value = { text: describe(s), left: x, top: y }
  await nextTick()
  const el = tipEl.value
  if (!el || !tip.value) return
  const w = el.offsetWidth
  const h = el.offsetHeight
  const left = Math.min(Math.max(x - w / 2, TIP_MARGIN), window.innerWidth - w - TIP_MARGIN)
  const above = y - h - TIP_GAP
  const top = above >= TIP_MARGIN ? above : y + TIP_GAP
  tip.value = { text: tip.value.text, left, top }
})
function clearTip() {
  hovered.value = null
}
if (typeof window !== 'undefined') {
  window.addEventListener('scroll', clearTip, { passive: true, capture: true })
  onBeforeUnmount(() => window.removeEventListener('scroll', clearTip, { capture: true }))
}

async function pngDataUrl(svg: SVGSVGElement | null = svgEl.value): Promise<string | null> {
  if (!svg) return null
  const clone = svg.cloneNode(true) as SVGSVGElement
  const style = getComputedStyle(svg)
  clone.querySelectorAll<SVGElement>('[data-code]').forEach((el, i) => {
    const src = svg.querySelectorAll<SVGElement>('[data-code]')[i]
    if (src) el.setAttribute('fill', getComputedStyle(src).fill)
  })
  clone.setAttribute('style', `background:${style.backgroundColor}`)
  const data = new XMLSerializer().serializeToString(clone)
  const img = new Image()
  img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(data)
  await img.decode()
  const canvas = document.createElement('canvas')
  canvas.width = WIDTH * 2
  canvas.height = HEIGHT * 2
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  ctx.fillStyle = getComputedStyle(document.body).backgroundColor
  ctx.fillRect(0, 0, canvas.width, canvas.height)
  ctx.drawImage(img, 0, 0, canvas.width, canvas.height)
  return canvas.toDataURL('image/png')
}

const svgEl = ref<SVGSVGElement | null>(null)

const scale = ref(0.4)
let resizer: ResizeObserver | null = null
watch(
  svgEl,
  (el) => {
    resizer?.disconnect()
    if (!el || typeof ResizeObserver === 'undefined') return
    resizer = new ResizeObserver(() => {
      const r = el.getBoundingClientRect()
      if (r.width && r.height) scale.value = Math.min(r.width / WIDTH, r.height / HEIGHT)
    })
    resizer.observe(el)
  },
  { immediate: true },
)
onBeforeUnmount(() => resizer?.disconnect())
const insetFont = computed(() => Math.min(Math.max(13 / scale.value, 22), 36))
defineExpose({ pngDataUrl: () => pngDataUrl() })
</script>

<template>
  <div class="county-map">
    <svg
      ref="svgEl"
      :viewBox="`0 0 ${WIDTH} ${HEIGHT}`"
      role="group"
      :aria-label="label ?? '臺灣縣市地圖'"
    >
      <g
        v-for="inset in insets"
        :key="inset.code"
        class="inset"
        :class="{ hover: inset.code === hovered, selected: inset.code === selected }"
        aria-hidden="true"
        @click="emit('select', inset.code)"
        @mouseenter="hovered = inset.code"
        @mouseleave="hovered = null"
      >
        <rect :x="4" :y="inset.y" :width="INSET.w" :height="INSET.h" rx="6" />
        <text
          class="inset-name"
          :x="12"
          :y="inset.y + INSET_HEADER / 2 + 2"
          :style="{ fontSize: `${insetFont}px` }"
        >
          {{ inset.name }}
          <tspan v-if="shown.has(inset.code)" class="inset-value" dx="8">
            {{ formatNumber(shown.get(inset.code), decimals) + (unit ?? '') }}
          </tspan>
        </text>
      </g>
      <path
        v-for="s in shapes"
        :key="s.code"
        :d="s.d"
        :data-code="s.code"
        :style="{ fill: fill(s.code) }"
        :class="{
          selected: s.code === selected,
          hover: s.code === hovered,
          pulse: s.code === pulse,
          faded: focus && s.code !== focus,
        }"
        tabindex="0"
        role="button"
        :aria-label="describe(s)"
        :aria-pressed="s.code === selected"
        @click="emit('select', s.code)"
        @keydown="onKey($event, s.code)"
        @mouseenter="hovered = s.code"
        @mouseleave="hovered = null"
        @focus="hovered = s.code"
        @blur="hovered = null"
      />
      <circle
        v-for="s in shapes.filter((x) => x.small)"
        :key="`hit-${s.code}`"
        class="hit"
        :cx="s.cx"
        :cy="s.cy"
        r="13"
        aria-hidden="true"
        @click="emit('select', s.code)"
        @mouseenter="hovered = s.code"
        @mouseleave="hovered = null"
      />
      <template v-for="s in shapes" :key="`label-${s.code}`">
        <text
          v-if="shown.has(s.code)"
          class="value"
          :x="s.cx"
          :y="s.cy"
          :style="{ fill: ink(s.code) }"
          aria-hidden="true"
        >
          {{ levelOf(shown.get(s.code)!).short ?? formatNumber(shown.get(s.code), 0) }}
        </text>
      </template>
    </svg>
    <Teleport to="body">
      <div
        v-if="tip"
        ref="tipEl"
        class="county-map-tooltip"
        role="status"
        :style="{ left: `${tip.left}px`, top: `${tip.top}px` }"
      >
        {{ tip.text }}
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.county-map {
  position: relative;
  width: 100%;
  height: 100%;
}
svg {
  display: block;
  width: 100%;
  height: 100%;
}
path {
  stroke: var(--map-stroke);
  stroke-width: 0.8;
  cursor: pointer;
  transition:
    opacity 0.15s,
    fill 0.3s;
}
path:hover,
path.hover,
path:focus-visible {
  stroke: var(--text-primary);
  stroke-width: 1.6;
  outline: none;
}
path.selected {
  stroke: var(--text-primary);
  stroke-width: 2.4;
}
path.pulse {
  animation: pulse 0.35s ease-out;
}
@keyframes pulse {
  from {
    opacity: 0.45;
  }
}
path.faded {
  opacity: 0.35;
}
.inset {
  cursor: pointer;
}
.inset rect {
  fill: transparent;
  stroke: var(--border-strong);
  stroke-dasharray: 3 3;
}
.inset text {
  font-weight: 600;
  fill: var(--text-secondary);
  dominant-baseline: central;
}
.inset .inset-value {
  fill: var(--text-primary);
}
.inset.hover rect,
.inset.selected rect {
  stroke: var(--text-primary);
  stroke-dasharray: none;
  stroke-width: 1.6;
}
.value {
  font-size: 11px;
  font-weight: 600;
  text-anchor: middle;
  dominant-baseline: central;
  pointer-events: none;
}
.hit {
  fill: transparent;
  cursor: pointer;
}
</style>

<style>
.county-map-tooltip {
  position: fixed;
  z-index: 1000;
  padding: 0.25rem 0.55rem;
  border-radius: 6px;
  background: var(--text-primary);
  color: var(--bg);
  font-size: 0.85rem;
  white-space: nowrap;
  pointer-events: none;
}
</style>
