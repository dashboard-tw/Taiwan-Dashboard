<script setup lang="ts">
import type { ApexOptions } from 'apexcharts'
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { getCountiesGeo, type CountyCollection } from '../../api/client'
import type { ByCountyItem, ComponentInfo, County, DataMeta, Series } from '../../api/types'
import { formatAxisTime, formatNumber, formatObserved } from '../../lib/format'
import {
  POWER_COLORS,
  SERIES_COLORS,
  STATUS_LEVELS,
  levelIn,
  levelsFor,
  waterLevel,
  type Level,
} from '../../lib/levels'
import { resolvedTheme } from '../../theme'
import AnimatedNumber from './AnimatedNumber.vue'
import ApexChart from './ApexChart.vue'
import ChartSkeleton from '../skeleton/ChartSkeleton.vue'
import CountyMap from './CountyMap.vue'
import ImageFrames from './ImageFrames.vue'

const props = defineProps<{
  type: string
  data: unknown
  meta: DataMeta
  component: ComponentInfo
  counties: County[]
  height?: number | string
  selectedCounty?: string | null
  pending?: boolean
  paused?: boolean
}>()

const emit = defineEmits<{ selectCounty: [code: string] }>()

const shape = computed(() => props.meta.data_shape)
const decimals = computed(() => props.component.decimals)
const unit = computed(() => props.component.unit)
const countyName = computed(() => new Map(props.counties.map((c) => [c.code, c.name])))

const fmt = (v: number | null) =>
  `${formatNumber(v, decimals.value)}${unit.value ? ` ${unit.value}` : ''}`

const flat = computed<{ labels: string[]; values: (number | null)[] }>(() => {
  if (shape.value === 'by_county') {
    const items = [...(props.data as ByCountyItem[])].sort(
      (a, b) => (b.data ?? -1) - (a.data ?? -1),
    )
    return {
      labels: items.map((i) => countyName.value.get(i.county_code) ?? i.county_code),
      values: items.map((i) => i.data),
    }
  }
  const s = (props.data as Series[])[0]
  return { labels: s?.data.map((p) => p.x) ?? [], values: s?.data.map((p) => p.y) ?? [] }
})

const levels = computed(() =>
  levelsFor(props.component.slug, props.component.topic, props.component.unit),
)
const levelOf = computed<((v: number) => Level) | null>(() => {
  const ls = levels.value
  return ls ? (v: number) => levelIn(ls, v) : null
})

function pointColors(labels: string[], values: (number | null)[], categorical = false): string[] {
  const status = STATUS_LEVELS[props.component.slug]
  if (status) return labels.map((l) => status.find((x) => x.label === l)?.color ?? '#a8a29e')
  const lv = levelOf.value
  if (lv)
    return values.map((v) =>
      v === null
        ? '#a8a29e'
        : lv(v).color.startsWith('var(')
          ? resolveVar(lv(v).color)
          : lv(v).color,
    )
  if (props.component.topic === 'power')
    return labels.map((l, i) => POWER_COLORS[l] ?? SERIES_COLORS[i % SERIES_COLORS.length]!)
  if (props.component.colors.length)
    return labels.map((_, i) => props.component.colors[i % props.component.colors.length]!)
  if (categorical) return labels.map((_, i) => SERIES_COLORS[i % SERIES_COLORS.length]!)
  return labels.map(() => SERIES_COLORS[1]!)
}

function resolveVar(v: string): string {
  const name = v.slice(4, -1)
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || '#3b82f6'
}

const barOptions = computed<ApexOptions>(() => {
  const { labels, values } = flat.value
  const colors = pointColors(labels, values)
  return {
    chart: { type: 'bar' },
    plotOptions: {
      bar: {
        horizontal: true,
        distributed: true,
        borderRadius: 3,
        barHeight: '70%',
        dataLabels: { position: 'top' },
      },
    },
    colors,
    legend: { show: false },
    dataLabels: {
      enabled: labels.length <= 25,
      formatter: (v: number) => formatNumber(v, decimals.value),
      style: { colors: [resolvedTheme.value === 'dark' ? '#f5f5f4' : '#1c1917'], fontWeight: 500 },
      offsetX: 26,
    },
    xaxis: {
      categories: labels,
      ...barRange(values),
      labels: { formatter: (v: string) => axisNumber(Number(v), axisDecimals(values)) },
    },
    yaxis: { labels: { maxWidth: labels.some((l) => l.length > 7) ? 150 : 110 } },
    tooltip: { y: { formatter: (v: number) => fmt(v) } },
    grid: { padding: { right: 12 } },
  }
})

function axisDecimals(values: (number | null)[]): number {
  const span = Math.max(0, ...values.map((v) => Math.abs(v ?? 0)))
  return span > 0 && span < 5 ? Math.min(decimals.value, 1) : 0
}

function axisNumber(v: number, digits: number): string {
  const r = Number(v.toFixed(digits))
  return formatNumber(r === 0 ? 0 : r, Number.isInteger(r) ? 0 : digits)
}

function barRange(values: (number | null)[]): { min?: number; max?: number } {
  const nums = values.filter((v): v is number => v !== null)
  const hi = Math.max(0, ...nums) * 1.15
  const lo = Math.min(0, ...nums) * 1.15
  if (lo === 0) return { min: 0, max: niceMax(hi) }
  const step = 10 ** Math.floor(Math.log10(Math.max(hi, -lo)))
  return { min: Math.floor(lo / step) * step, max: Math.ceil(hi / step) * step }
}

function niceMax(v: number): number | undefined {
  if (v <= 0) return undefined
  const step = 10 ** Math.floor(Math.log10(v))
  return Math.ceil(v / step) * step
}

const barSeries = computed(() => [{ name: props.component.name, data: flat.value.values }])

const columnOptions = computed<ApexOptions>(() => {
  const { labels, values } = flat.value
  return {
    chart: { type: 'bar' },
    plotOptions: {
      bar: {
        horizontal: false,
        distributed: true,
        borderRadius: 3,
        columnWidth: '60%',
        dataLabels: { position: 'top' },
      },
    },
    colors: pointColors(labels, values),
    legend: { show: false },
    dataLabels: {
      enabled: labels.length <= 12,
      formatter: (v: number) => formatNumber(v, decimals.value),
      style: { colors: [resolvedTheme.value === 'dark' ? '#f5f5f4' : '#1c1917'], fontWeight: 500 },
      offsetY: -20,
    },
    xaxis: {
      categories: labels,
      labels: { rotate: -45, hideOverlappingLabels: false, trim: true },
    },
    yaxis: {
      ...barRange(values),
      labels: { formatter: (v: number) => axisNumber(v, axisDecimals(values)) },
    },
    tooltip: { y: { formatter: (v: number) => fmt(v) } },
  }
})

const treemapOptions = computed<ApexOptions>(() => {
  const { labels, values } = flat.value
  return {
    chart: { type: 'treemap' },
    colors: pointColors(labels, values, true),
    legend: { show: false },
    plotOptions: { treemap: { distributed: true, enableShades: false } },
    dataLabels: {
      enabled: true,
      style: { fontSize: '12px', fontWeight: 500 },
      formatter: (text, opts) => [
        String(text),
        formatNumber(Number((opts as { value?: number } | undefined)?.value ?? 0), decimals.value),
      ],
    },
    tooltip: { y: { formatter: (v: number) => fmt(v) } },
  }
})

const treemapSeries = computed(() => [
  {
    data: flat.value.labels
      .map((x, i) => ({ x, y: flat.value.values[i] ?? 0 }))
      .filter((p) => p.y > 0),
  },
])

const listItems = computed(() =>
  flat.value.labels.map((text, i) => {
    const [title = '', ...rest] = text.split('\n')
    return { title, detail: rest.join('\n').trim(), value: flat.value.values[i] ?? null }
  }),
)

const numberTiles = computed(() =>
  flat.value.labels.slice(0, 8).map((label, i) => ({ label, value: flat.value.values[i] ?? null })),
)

const donutOptions = computed<ApexOptions>(() => {
  const { labels, values } = flat.value
  const colors = pointColors(labels, values, true)
  return {
    chart: { type: 'donut' },
    labels,
    colors,
    legend: { show: false },
    stroke: { width: 1 },
    dataLabels: { formatter: (v: number) => `${v.toFixed(1)}%` },
    tooltip: { y: { formatter: (v: number) => fmt(v) } },
    plotOptions: {
      pie: {
        donut: {
          labels: {
            show: true,
            total: {
              show: true,
              label: '總合',
              formatter: () => fmt(values.reduce<number>((a, b) => a + (b ?? 0), 0)),
            },
          },
        },
      },
    },
  }
})

const donutSeries = computed(() => flat.value.values.map((v) => v ?? 0))
const donutLegend = computed(() => {
  const { labels } = flat.value
  const colors = (donutOptions.value.colors ?? []) as string[]
  return labels.map((label, i) => ({ label, color: colors[i] ?? '#a8a29e' }))
})

const timeSeries = computed(() =>
  (props.data as Series[]).map((s) => ({
    name: s.name ?? props.component.name,
    data: s.data.map((p) => ({ x: new Date(p.x).getTime(), y: p.y })),
  })),
)

function monthlyPoints(): boolean {
  const xs = timeSeries.value[0]?.data.map((p) => p.x) ?? []
  if (xs.length < 2) return false
  return xs.slice(1).every((x, i) => x - xs[i]! >= 27 * 24 * 3600 * 1000)
}

const timeOptions = computed<ApexOptions>(() => {
  const names = timeSeries.value.map((s) => s.name)
  const colors =
    props.component.topic === 'power'
      ? names.map((n, i) => POWER_COLORS[n] ?? SERIES_COLORS[i % SERIES_COLORS.length]!)
      : names.map((_, i) => SERIES_COLORS[i % SERIES_COLORS.length]!)
  const step = props.meta.step ?? (monthlyPoints() ? 'month' : 'day')
  const stacked = props.type === 'TimelineStackedChart'
  return {
    chart: { type: stacked ? 'area' : 'line', stacked },
    colors,
    stroke: { width: stacked ? 1 : 2, curve: 'straight' },
    fill: stacked ? { type: 'solid', opacity: 0.8 } : undefined,
    markers: { size: timeSeries.value[0]?.data.length === 1 ? 4 : 0 },
    legend: { show: names.length > 1, position: 'bottom' },
    dataLabels: { enabled: false },
    xaxis: {
      type: 'datetime',
      labels: {
        datetimeUTC: false,
        formatter: (_: string, ts?: number) =>
          ts ? formatAxisTime(new Date(ts).toISOString(), step) : '',
      },
    },
    yaxis: { labels: { formatter: (v: number) => formatNumber(v, decimals.value) } },
    tooltip: {
      x: {
        formatter: (ts: number) =>
          step === 'month'
            ? formatAxisTime(new Date(ts).toISOString(), 'month')
            : new Date(ts).toLocaleString('zh-TW', { timeZone: 'Asia/Taipei', hour12: false }),
      },
      y: { formatter: (v: number) => fmt(v) },
    },
  }
})

const geo = shallowRef<CountyCollection | null>(null)
const geoError = ref(false)
let geoRetry: ReturnType<typeof setTimeout> | undefined
async function loadGeo() {
  clearTimeout(geoRetry)
  if (props.type !== 'CountyMapChart' || geo.value) return
  try {
    geo.value = await getCountiesGeo()
    geoError.value = false
  } catch {
    geoError.value = true
    geoRetry = setTimeout(loadGeo, 15_000)
  }
}
watch(() => props.type, loadGeo, { immediate: true })
function retryGeoSoon() {
  if (geoError.value && document.visibilityState === 'visible') void loadGeo()
}
if (typeof window !== 'undefined') {
  window.addEventListener('online', retryGeoSoon)
  document.addEventListener('visibilitychange', retryGeoSoon)
}
onBeforeUnmount(() => {
  clearTimeout(geoRetry)
  window.removeEventListener('online', retryGeoSoon)
  document.removeEventListener('visibilitychange', retryGeoSoon)
})

const mapValues = computed(() => {
  const m = new Map<string, number>()
  if (shape.value === 'by_county')
    for (const i of props.data as ByCountyItem[]) if (i.data !== null) m.set(i.county_code, i.data)
  return m
})

const outerHeight = computed(() =>
  typeof props.height === 'number' ? `${props.height}px` : (props.height ?? '100%'),
)
const root = ref<HTMLDivElement | null>(null)
const boxHeight = ref(0)
const measured = ref(false)
let resize: ResizeObserver | null = null
onMounted(() => {
  if (!root.value || typeof ResizeObserver === 'undefined') {
    measured.value = true
    return
  }
  resize = new ResizeObserver(([e]) => {
    boxHeight.value = e?.contentRect.height ?? 0
    measured.value = true
  })
  resize.observe(root.value)
})
onBeforeUnmount(() => resize?.disconnect())

const inView = ref(typeof IntersectionObserver === 'undefined')
let viewObserver: IntersectionObserver | null = null
onMounted(() => {
  if (inView.value || !root.value) return
  viewObserver = new IntersectionObserver(
    (entries) => {
      if (!entries.some((e) => e.isIntersecting)) return
      viewObserver?.disconnect()
      viewObserver = null
      const idle = window.requestIdleCallback ?? ((cb: () => void) => setTimeout(cb, 1))
      idle(() => (inView.value = true), { timeout: 600 })
    },
    { rootMargin: '400px 0px' },
  )
  viewObserver.observe(root.value)
})
onBeforeUnmount(() => viewObserver?.disconnect())
const skeletonKind = computed<'bar' | 'donut' | 'line' | 'map' | null>(() => {
  const t = resolved.value
  if (t === 'map' || t === 'donut') return t
  if (t === 'bar' || t === 'column' || t === 'treemap') return 'bar'
  if (t === 'number' || t === 'list' || t === 'image') return null
  return 'line'
})
const barScroll = computed(() => {
  if (resolved.value !== 'bar') return false
  const need = flat.value.labels.length * 16 + 50
  return flat.value.labels.length > 30 || (boxHeight.value > 0 && need > boxHeight.value + 1)
})
const innerHeight = computed(() =>
  barScroll.value ? `${flat.value.labels.length * 16 + 50}px` : '100%',
)

const MAX_SUMMARY = new Set(['quake-max-intensity-by-county'])
const mapSummaryLabel = computed(() => (MAX_SUMMARY.has(props.component.slug) ? '最大' : '平均'))
const mapAverage = computed(() => {
  const vs = [...mapValues.value.values()]
  if (!vs.length) return null
  if (MAX_SUMMARY.has(props.component.slug)) return Math.max(...vs)
  return vs.reduce((a, b) => a + b, 0) / vs.length
})

const resolved = computed(() => {
  if (props.type === 'CountyMapChart' && shape.value === 'by_county') return 'map'
  if (props.type === 'ImageChart' && shape.value === 'time') return 'image'
  if (shape.value === 'time') return 'time'
  if (props.type === 'DonutChart') return 'donut'
  if (props.type === 'TreemapChart') return 'treemap'
  if (props.type === 'ColumnChart') return 'column'
  if (props.type === 'NumberChart') return 'number'
  if (props.type === 'ListChart') return 'list'
  return 'bar'
})

const empty = computed(() => {
  if (props.pending) return false
  if (shape.value === 'time') return timeSeries.value.every((s) => s.data.length === 0)
  return flat.value.labels.length === 0
})

const emptyText = computed(() => {
  const what = props.type === 'ListChart' ? `沒有${props.component.name}` : '沒有資料'
  if (props.meta.as_of) return `${formatObserved(props.meta.as_of)} 時${what}`
  return props.paused ? '暫時無法取得資料' : `目前${what}`
})

const reducedMotion =
  typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
const lineBox = ref<HTMLElement | null>(null)
let lineDrawn = false
let pendingShift = 0

const pointTimes = (series: { data: { x: number }[] }[]) =>
  series.flatMap((s) => s.data.map((p) => p.x))

watch(
  () => (resolved.value === 'time' ? timeSeries.value : null),
  (now, before) => onTimeSeries(now, before),
)

function onTimeSeries(now: typeof timeSeries.value | null, before: typeof timeSeries.value | null) {
  if (!now?.length || !before?.length) return
  const was = pointTimes(before)
  const is = pointTimes(now)
  if (!was.length || !is.length) return
  const oldLast = Math.max(...was)
  const newLast = Math.max(...is)
  const newFirst = Math.min(...is)
  if (newLast > oldLast && newLast > newFirst) {
    pendingShift = Math.min((newLast - oldLast) / (newLast - newFirst), 0.5)
  }
}

function replay(el: HTMLElement, cls: string, ms: number) {
  el.classList.remove(cls)
  void el.offsetWidth // 重新開始 CSS 動畫
  el.classList.add(cls)
  setTimeout(() => el.classList.remove(cls), ms + 100)
}

function wrapSeries(el: HTMLElement) {
  const SVG = 'http://www.w3.org/2000/svg'
  for (const path of el.querySelectorAll<SVGPathElement>('.apexcharts-series > path')) {
    const clip = path.getAttribute('clip-path')
    const outer = document.createElementNS(SVG, 'g')
    if (clip) outer.setAttribute('clip-path', clip)
    const inner = document.createElementNS(SVG, 'g')
    inner.setAttribute('class', 'line-motion')
    path.removeAttribute('clip-path')
    path.replaceWith(outer)
    outer.append(inner)
    inner.append(path)
  }
}

let drawStart = 0
let drawObserver: IntersectionObserver | null = null
const DRAW_FROM = 'inset(-12px 100% -12px -12px) stroke-box'
const DRAW_TO = 'inset(-12px -12px -12px -12px) stroke-box'

function animateDraw(el: HTMLElement) {
  const elapsed = performance.now() - drawStart
  if (elapsed >= LINE_DRAW_MS) return
  for (const g of el.querySelectorAll<SVGGElement>('.apexcharts-plot-series')) {
    if (g.getAnimations().length) continue
    g.animate([{ clipPath: DRAW_FROM }, { clipPath: DRAW_TO }], {
      duration: LINE_DRAW_MS,
      delay: -elapsed,
      easing: 'ease-out',
      fill: 'backwards',
    })
  }
}

function startDrawWhenVisible(el: HTMLElement) {
  if (typeof IntersectionObserver === 'undefined') {
    drawStart = performance.now()
    animateDraw(el)
    return
  }
  drawObserver = new IntersectionObserver(
    (entries) => {
      if (!entries.some((e) => e.isIntersecting)) return
      drawObserver?.disconnect()
      drawObserver = null
      drawStart = performance.now()
      animateDraw(el)
    },
    { threshold: 0.25 },
  )
  drawObserver.observe(el)
}
onBeforeUnmount(() => drawObserver?.disconnect())

watch([resolved, () => props.type], () => {
  lineDrawn = false
  drawStart = 0
  pendingShift = 0
  drawObserver?.disconnect()
  drawObserver = null
})

function onLineRendered() {
  const el = lineBox.value
  if (!el || reducedMotion) return
  if (!lineDrawn) {
    lineDrawn = true
    startDrawWhenVisible(el)
  } else if (drawStart && performance.now() - drawStart < LINE_DRAW_MS) {
    animateDraw(el)
  } else if (pendingShift > 0) {
    el.style.setProperty('--line-shift', `${(pendingShift * 100).toFixed(2)}%`)
    pendingShift = 0
    wrapSeries(el)
    replay(el, 'line-shift', LINE_SHIFT_MS)
  }
}
const LINE_DRAW_MS = 900
const LINE_SHIFT_MS = 700
const timeChartOptions = computed<ApexOptions>(() => ({
  ...timeOptions.value,
  chart: {
    ...timeOptions.value.chart,
    events: { mounted: onLineRendered, updated: onLineRendered },
  },
}))

const apexAnimated = computed(() => ['bar', 'column', 'donut', 'treemap'].includes(resolved.value))

const apex = ref<InstanceType<typeof ApexChart> | null>(null)
const map = ref<InstanceType<typeof CountyMap> | null>(null)

async function pngDataUrl(): Promise<string | null> {
  return (await (map.value ?? apex.value)?.pngDataUrl()) ?? null
}

defineExpose({ pngDataUrl })
</script>

<template>
  <div ref="root" :class="['chart-view', { scroll: barScroll }]" :style="{ height: outerHeight }">
    <p v-if="empty" class="empty muted">{{ emptyText }}</p>
    <ChartSkeleton v-else-if="!inView && skeletonKind" :kind="skeletonKind" />
    <template v-else-if="resolved === 'map'">
      <CountyMap
        v-if="geo"
        ref="map"
        :geo="geo"
        :values="mapValues"
        :level-of="levelOf ?? waterLevel"
        :decimals="decimals"
        :unit="unit"
        :selected="selectedCounty"
        :label="component.name"
        @select="emit('selectCounty', $event)"
      />
      <p v-else-if="geoError" class="empty muted">
        地圖載入失敗
        <button type="button" class="retry" @click="loadGeo">重新載入</button>
      </p>
      <ChartSkeleton v-else kind="map" />
      <div v-if="geo" class="map-overlay">
        <div class="map-total">
          <span>{{ mapSummaryLabel }}</span>
          <strong
            >{{ formatNumber(mapAverage, decimals) }}<small v-if="unit"> {{ unit }}</small></strong
          >
        </div>
        <ul v-if="levels" class="map-legend" :aria-label="`${component.name}的分級`">
          <li v-for="l in levels" :key="l.label">
            <i :style="{ background: l.color }"></i>{{ l.label }}
          </li>
        </ul>
      </div>
    </template>
    <div v-else-if="resolved === 'bar'" class="inner" :style="{ height: innerHeight }">
      <ApexChart
        v-if="measured"
        ref="apex"
        type="bar"
        :series="barSeries"
        :options="barOptions"
        :animated="apexAnimated"
      />
    </div>
    <div v-else-if="resolved === 'donut'" class="donut">
      <div class="donut-chart">
        <ApexChart
          ref="apex"
          type="donut"
          :series="donutSeries"
          :options="donutOptions"
          :animated="apexAnimated"
        />
      </div>
      <ul class="donut-legend">
        <li v-for="l in donutLegend" :key="l.label">
          <i :style="{ background: l.color }"></i>{{ l.label }}
        </li>
      </ul>
    </div>
    <div v-else-if="resolved === 'column'" class="inner">
      <ApexChart
        ref="apex"
        type="bar"
        :series="barSeries"
        :options="columnOptions"
        :animated="apexAnimated"
      />
    </div>
    <ApexChart
      v-else-if="resolved === 'treemap'"
      ref="apex"
      type="treemap"
      :series="treemapSeries"
      :options="treemapOptions"
      :animated="apexAnimated"
    />
    <ul
      v-else-if="resolved === 'number'"
      :class="['numbers', { single: numberTiles.length === 1 }]"
    >
      <li v-for="t in numberTiles" :key="t.label">
        <span class="number-label">{{ t.label }}</span>
        <strong class="number-value"
          ><AnimatedNumber :value="t.value" :decimals="decimals" /><small v-if="unit">
            {{ unit }}</small
          ></strong
        >
      </li>
    </ul>
    <ul v-else-if="resolved === 'list'" class="list">
      <li v-for="(it, i) in listItems" :key="i">
        <div>
          <strong>{{ it.title }}</strong>
          <p v-if="it.detail">{{ it.detail }}</p>
        </div>
        <span v-if="it.value !== null" class="list-value"
          >{{ formatNumber(it.value, decimals) }}<small v-if="unit"> {{ unit }}</small></span
        >
      </li>
    </ul>
    <ImageFrames
      v-else-if="resolved === 'image'"
      :series="data as Series[]"
      :label="component.name"
    />
    <div v-else ref="lineBox" class="line-box">
      <ApexChart
        ref="apex"
        :type="type === 'TimelineStackedChart' ? 'area' : 'line'"
        :series="timeSeries"
        :options="timeChartOptions"
      />
    </div>
  </div>
</template>

<style scoped>
.chart-view {
  position: relative;
  width: 100%;
  min-height: 140px;
  overflow: hidden;
}
.line-box {
  height: 100%;
}
.line-box :deep(.line-motion) {
  transform-box: fill-box;
}
.line-box.line-shift :deep(.line-motion) {
  animation: line-shift 0.7s ease-out;
}
@keyframes line-shift {
  from {
    transform: translateX(var(--line-shift));
  }
}
@media (prefers-reduced-motion: reduce) {
  .line-box :deep(.line-motion) {
    animation: none !important;
  }
}
.chart-view.scroll {
  overflow-y: auto;
  scrollbar-width: thin;
}
.inner {
  position: relative;
}
.donut {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.donut-chart {
  flex: 1;
  min-height: 0;
}
.list {
  height: 100%;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
}
.list li {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 2px;
  border-bottom: 1px solid var(--border);
}
.list li:last-child {
  border-bottom: none;
}
.list strong {
  color: var(--text-primary);
  font-size: var(--font-ms);
  font-weight: 600;
}
.list p {
  margin: 2px 0 0;
  color: var(--text-muted);
  font-size: var(--font-s);
  line-height: 1.5;
  white-space: pre-line;
}
.list-value {
  flex-shrink: 0;
  color: var(--text-primary);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.list-value small {
  color: var(--text-muted);
  font-weight: 400;
}
.numbers {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  align-content: safe center;
  gap: 8px 12px;
  height: 100%;
  margin: 0;
  padding: 4px 0;
  overflow-y: auto;
  scrollbar-width: thin;
  list-style: none;
}
.numbers li {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 12px;
  border-left: 3px solid var(--border-strong);
}
.numbers.single {
  grid-template-columns: 1fr;
  text-align: center;
}
.numbers.single li {
  align-items: center;
  border-left: none;
}
.number-label {
  color: var(--text-muted);
  font-size: var(--font-s);
}
.number-value {
  color: var(--text-primary);
  font-size: 1.75rem;
  font-weight: 600;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.numbers.single .number-value {
  font-size: 2.75rem;
}
.number-value small {
  margin-left: 2px;
  color: var(--text-muted);
  font-size: var(--font-ms);
  font-weight: 400;
}
.donut-legend {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 2px 10px;
  margin: 4px 0 0;
  padding: 0;
  list-style: none;
  color: var(--text-muted);
  font-size: var(--font-s);
}
.donut-legend li,
.map-legend li {
  display: flex;
  align-items: center;
  gap: 4px;
}
.donut-legend i {
  width: 0.6rem;
  height: 0.6rem;
  border-radius: 50%;
}
.map-overlay {
  position: absolute;
  left: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  pointer-events: none;
}
.map-total {
  display: flex;
  flex-direction: column;
  color: var(--text-muted);
  font-size: var(--font-s);
  pointer-events: none;
}
.map-total strong {
  color: var(--text-muted);
  font-size: var(--font-l);
  font-weight: 400;
  line-height: 1.2;
}
.map-total small {
  font-size: var(--font-ms);
}
.map-legend {
  margin: 0;
  padding: 0;
  list-style: none;
  color: var(--text-muted);
  font-size: 0.7rem;
  line-height: 1.5;
  pointer-events: none;
}
.map-legend li {
  display: flex;
  align-items: center;
  gap: 4px;
}
.map-legend i {
  width: 0.65rem;
  height: 0.65rem;
  border-radius: 2px;
}
.empty {
  display: grid;
  place-items: center;
  height: 100%;
  margin: 0;
}
.retry {
  margin-left: 8px;
  padding: 2px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.85em;
  cursor: pointer;
}
.retry:hover {
  color: var(--text-primary);
}
</style>
