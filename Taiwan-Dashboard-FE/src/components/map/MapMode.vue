<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { ComponentInfo, Dashboard } from '../../api/types'
import { formatFreq } from '../../lib/format'
import { levelsFor } from '../../lib/levels'
import ComponentCard from '../ComponentCard.vue'
import MapCanvas, { type ActiveLayer } from './MapCanvas.vue'

const props = defineProps<{ dashboard: Dashboard; county: string | null }>()
const emit = defineEmits<{ selectCounty: [code: string] }>()

const route = useRoute()
const router = useRouter()

const mapped = computed(() =>
  props.dashboard.components.filter((c) => (c.map_layers?.length ?? 0) > 0),
)

const open = computed<string[]>(() => {
  const q = route.query.layers
  if (typeof q === 'string')
    return q.split(',').filter((s) => props.dashboard.components.some((c) => c.slug === s))
  return mapped.value[0] ? [mapped.value[0].slug] : []
})
function toggle(c: ComponentInfo) {
  const next = open.value.includes(c.slug)
    ? open.value.filter((s) => s !== c.slug)
    : [...open.value, c.slug]
  void router.replace({ query: { ...route.query, layers: next.join(',') } })
}

const active = computed<ActiveLayer[]>(() =>
  props.dashboard.components
    .filter((c) => open.value.includes(c.slug))
    .flatMap((c) => (c.map_layers ?? []).map((layer) => ({ component: c, layer }))),
)

const legends = computed(() =>
  props.dashboard.components
    .filter((c) => open.value.includes(c.slug) && (c.map_layers?.length ?? 0) > 0)
    .map((c) => ({ title: c.name, unit: c.unit, levels: levelsFor(c.slug, c.topic, c.unit) ?? [] }))
    .filter((l) => l.levels.length > 0),
)

const basemap = ref<'emap' | 'none'>('emap')
const canvas = ref<InstanceType<typeof MapCanvas> | null>(null)

function chartSelect(code: string) {
  canvas.value?.flyToCounty(code)
  emit('selectCounty', code)
}

type Sheet = 'collapsed' | 'half' | 'full'
const sheet = ref<Sheet>('half')
const selected = ref<{ title: string; rows: [string, string][] } | null>(null)
watch(selected, (s) => {
  if (s && sheet.value === 'collapsed') sheet.value = 'half'
})
function cycleSheet() {
  sheet.value = sheet.value === 'collapsed' ? 'half' : sheet.value === 'half' ? 'full' : 'collapsed'
}
let dragStart: number | null = null
function onDragStart(e: PointerEvent) {
  dragStart = e.clientY
  ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
}
function onDragEnd(e: PointerEvent) {
  if (dragStart === null) return
  const dy = e.clientY - dragStart
  dragStart = null
  if (Math.abs(dy) < 8) return cycleSheet()
  const order: Sheet[] = ['collapsed', 'half', 'full']
  const i = order.indexOf(sheet.value) + (dy < 0 ? 1 : -1)
  sheet.value = order[Math.max(0, Math.min(2, i))]!
}

const phone = window.matchMedia('(max-width: 47.4375em)')
const insetBottom = computed(() => {
  if (!phone.matches) return 0
  const h = window.innerHeight
  return sheet.value === 'collapsed' ? 56 : sheet.value === 'half' ? h * 0.45 : h * 0.45
})

function freq(c: ComponentInfo) {
  const s = (c.sources ?? []).map((x) => x.update_freq_seconds).filter((x) => x > 0)
  return s.length ? formatFreq(Math.min(...s)) : ''
}
</script>

<template>
  <div class="map-mode">
    <aside :class="['side', `sheet-${sheet}`]" aria-label="地圖圖層">
      <button
        type="button"
        class="grip"
        :aria-label="sheet === 'full' ? '收合清單' : '展開清單'"
        @pointerdown="onDragStart"
        @pointerup="onDragEnd"
      >
        <span></span>
      </button>

      <section v-if="selected" class="selected">
        <header>
          <h2>{{ selected.title }}</h2>
          <button type="button" class="close" aria-label="關閉" @click="selected = null">
            <span class="mi" aria-hidden="true">close</span>
          </button>
        </header>
        <p v-for="[k, v] in selected.rows" :key="k">
          <span>{{ k }}</span
          ><strong>{{ v }}</strong>
        </p>
      </section>

      <div
        v-for="c in dashboard.components"
        :key="c.slug"
        :class="['map-card', { on: open.includes(c.slug) }]"
      >
        <div class="map-card-head">
          <div class="titles">
            <h2>{{ c.name }}</h2>
            <span v-if="freq(c)" class="tag">{{ freq(c) }}</span>
            <span v-if="!c.map_layers?.length" class="tag muted-tag">無圖層</span>
          </div>
          <label class="switch">
            <input
              type="checkbox"
              :checked="open.includes(c.slug)"
              :aria-label="`${open.includes(c.slug) ? '關閉' : '開啟'}${c.name}`"
              @change="toggle(c)"
            />
            <span class="slider"></span>
          </label>
        </div>
        <ComponentCard
          v-if="open.includes(c.slug)"
          :component="c"
          :county="county"
          mode="map"
          @select-county="chartSelect"
        />
      </div>
    </aside>

    <div class="map-area">
      <MapCanvas
        ref="canvas"
        :layers="active"
        :county="county"
        :basemap="basemap"
        :inset-bottom="insetBottom"
        @select="selected = $event"
      />
      <div class="basemap" role="group" aria-label="底圖">
        <button type="button" :class="{ active: basemap === 'emap' }" @click="basemap = 'emap'">
          電子地圖
        </button>
        <button type="button" :class="{ active: basemap === 'none' }" @click="basemap = 'none'">
          無底圖
        </button>
      </div>
      <div v-if="legends.length" class="legend">
        <div v-for="l in legends" :key="l.title">
          <h3>
            {{ l.title }}<template v-if="l.unit">（{{ l.unit }}）</template>
          </h3>
          <ul>
            <li v-for="lv in l.levels" :key="lv.label">
              <i :style="{ background: lv.color }"></i>{{ lv.label }}
            </li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.map-mode {
  display: grid;
  grid-template-columns: 360px minmax(0, 1fr);
  gap: var(--font-m);
  height: calc(100vh - var(--header-h) - 3.2rem - var(--font-m) * 3);
  min-height: 420px;
}
.side {
  display: flex;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
  padding-right: 4px;
  scrollbar-width: thin;
}
.grip {
  display: none;
}
.map-card {
  flex-shrink: 0;
  padding: 12px var(--font-m);
  border-radius: var(--radius);
  background: var(--surface);
}
.map-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}
.titles {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.titles h2 {
  font-size: var(--font-ms);
  font-weight: 700;
}
.tag {
  height: 1rem;
  padding: 0 2px;
  border: 1px solid var(--text-muted);
  border-radius: var(--radius);
  color: var(--text-muted);
  font-size: var(--font-s);
  line-height: 1;
  display: inline-flex;
  align-items: center;
}
/* 不用透明度淡化，文字要維持 4.5:1 對比 */
.muted-tag {
  border: 1px dashed var(--border);
  background: transparent;
}
.map-card.on .map-card-head {
  margin-bottom: 6px;
}

.switch {
  position: relative;
  flex-shrink: 0;
  width: 2.4rem;
  height: 1.3rem;
  margin-top: 2px;
}
.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}
.slider {
  position: absolute;
  inset: 0;
  border-radius: 1rem;
  background: var(--button-muted);
  cursor: pointer;
  transition: background 0.2s;
}
.slider::before {
  content: '';
  position: absolute;
  left: 3px;
  top: 3px;
  width: calc(1.3rem - 6px);
  height: calc(1.3rem - 6px);
  border-radius: 50%;
  background: #ffffff;
  transition: transform 0.2s;
}
.switch input:checked + .slider {
  background: var(--accent);
}
.switch input:checked + .slider::before {
  transform: translateX(1.1rem);
}
.switch input:focus-visible + .slider {
  outline: 2px solid var(--focus);
  outline-offset: 2px;
}

.selected {
  flex-shrink: 0;
  padding: 10px var(--font-m);
  border: 1px solid var(--accent);
  border-radius: var(--radius);
  background: var(--surface);
}
.selected header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.selected h2 {
  font-size: var(--font-ms);
}
.selected p {
  display: flex;
  justify-content: space-between;
  font-size: 0.9rem;
}
.selected span {
  color: var(--text-muted);
}
.close {
  border: 0;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}

.map-area {
  position: relative;
  overflow: hidden;
  border-radius: var(--radius);
  background: var(--surface);
}
.basemap {
  position: absolute;
  top: 10px;
  left: 10px;
  display: flex;
  gap: 2px;
  padding: 2px;
  border-radius: var(--radius);
  background: var(--surface);
  box-shadow: 0 0 4px rgb(0 0 0 / 40%);
}
.basemap button {
  padding: 2px 8px;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-muted);
  font-size: var(--font-s);
  cursor: pointer;
}
.basemap button.active {
  background: var(--text-muted);
  color: #ffffff;
}
.legend {
  position: absolute;
  left: 10px;
  bottom: 24px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 60%;
  overflow-y: auto;
  padding: 8px 10px;
  border-radius: var(--radius);
  background: color-mix(in srgb, var(--surface) 92%, transparent);
  color: var(--text-muted);
  font-size: var(--font-s);
  box-shadow: 0 0 4px rgb(0 0 0 / 40%);
}
.legend h3 {
  margin-bottom: 2px;
  color: var(--text-primary);
  font-size: var(--font-s);
}
.legend ul {
  margin: 0;
  padding: 0;
  list-style: none;
}
.legend li {
  display: flex;
  align-items: center;
  gap: 4px;
}
.legend i {
  width: 0.65rem;
  height: 0.65rem;
  border-radius: 2px;
}

@media (max-width: 47.4375em) {
  .map-mode {
    display: block;
    position: fixed;
    inset: var(--header-h) 0 0 0;
    height: auto;
    z-index: 30;
  }
  .map-area {
    position: absolute;
    inset: 0;
    border-radius: 0;
  }
  .side {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 5;
    padding: 0 12px 12px;
    border-radius: 12px 12px 0 0;
    background: var(--bg);
    box-shadow: 0 -2px 10px rgb(0 0 0 / 40%);
    transition: height 0.25s ease;
  }
  .sheet-collapsed {
    height: 56px;
    overflow: hidden;
  }
  .sheet-half {
    height: 50%;
  }
  .sheet-full {
    height: calc(100% - 24px);
  }
  .grip {
    display: grid;
    place-items: center;
    flex-shrink: 0;
    position: sticky;
    top: 0;
    z-index: 1;
    height: 28px;
    border: 0;
    background: var(--bg);
    cursor: grab;
    touch-action: none;
  }
  .grip span {
    width: 2.5rem;
    height: 4px;
    border-radius: 2px;
    background: var(--border-strong);
  }
  .legend {
    bottom: auto;
    top: 48px;
    max-height: 35%;
  }
}
</style>
