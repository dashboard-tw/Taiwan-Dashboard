<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { pageTyphoon } from '../lib/pageContext'
import { useRouter } from 'vue-router'
import type { ComponentInfo, DataMeta } from '../api/types'
import { useComponentData } from '../composables/useComponentData'
import { useRenderTurn } from '../composables/useRenderTurn'
import { formatFreq, formatObserved } from '../lib/format'
import { useCatalog } from '../stores/catalog'
import { useDialogs } from '../stores/dialog'
import { chartKind } from '../lib/chartKind'
import { freshness } from '../lib/freshness'
import ChartView from './charts/ChartView.vue'
import ChartSkeleton from './skeleton/ChartSkeleton.vue'
import TagTooltip from './TagTooltip.vue'

const props = withDefaults(
  defineProps<{
    component: ComponentInfo
    county?: string | null
    mode?: 'default' | 'large' | 'embed' | 'map'
  }>(),
  { county: null, mode: 'default' },
)

const emit = defineEmits<{ selectCounty: [code: string] }>()

const catalog = useCatalog()
const dialogs = useDialogs()
const router = useRouter()

const effectiveCounty = computed(() =>
  props.county && props.component.scopes.includes('county') ? props.county : null,
)
const query = computed(() => ({
  ...(effectiveCounty.value ? { county: effectiveCounty.value } : {}),
  ...(pageTyphoon.value ? { typhoon: pageTyphoon.value } : {}),
}))
const { result } = useComponentData(() => props.component.slug, 'data', query)
const mapFirst = computed(
  () => props.component.chart_types[0] === 'CountyMapChart' && !effectiveCounty.value,
)
const PENDING_META = { data_shape: 'by_county' } as DataMeta
const turn = useRenderTurn(() => (!!result.value || mapFirst.value) && catalog.counties.length > 0)

const CHART_LABELS: Record<string, string> = {
  CountyMapChart: '縣市地圖',
  BarChart: '橫向長條圖',
  ColumnChart: '直條圖',
  DonutChart: '圓餅圖',
  TreemapChart: '矩形樹狀圖',
  NumberChart: '數字',
  ListChart: '清單',
  TimelineSeparateChart: '折線圖',
  TimelineStackedChart: '堆疊面積圖',
  ImageChart: '影像',
}

const chartTypes = computed(() => {
  const shape = result.value?.meta.data_shape
  const types = props.component.chart_types.filter(
    (t) => t !== 'CountyMapChart' || shape === 'by_county' || (!shape && mapFirst.value),
  )
  return types.length ? types : ['BarChart']
})
const chartType = ref('')
const picked = ref(false)
watch(
  chartTypes,
  (types) => {
    if (!picked.value || !types.includes(chartType.value)) chartType.value = types[0]!
  },
  { immediate: true },
)
function pick(t: string) {
  chartType.value = t
  picked.value = true
}

const sources = computed(() => result.value?.meta.sources ?? props.component.sources ?? [])
const freq = computed(() => {
  const s = sources.value.map((x) => x.update_freq_seconds).filter((x) => x > 0)
  return s.length ? formatFreq(Math.min(...s)) : ''
})
const agencies = computed(() => [...new Set(sources.value.map((s) => s.agency))].join('、'))
const fresh = computed(() => freshness(sources.value))
const paused = computed(() => fresh.value.level === 'paused')
const delayed = computed(
  () => !paused.value && (!!result.value?.meta.stale || fresh.value.level === 'late'),
)
const pausedTitle = computed(
  () =>
    `${fresh.value.agencies.join('、') || '資料來源'}暫停更新，` +
    `目前顯示 ${formatObserved(fresh.value.lastIngestedAt)} 的資料`,
)
const scopeLabel = computed(() =>
  effectiveCounty.value ? catalog.countyName(effectiveCounty.value) : '全國',
)

const detailHref = computed(
  () =>
    router.resolve({
      name: 'component',
      params: { slug: props.component.slug },
      query: effectiveCounty.value ? { county: effectiveCounty.value } : {},
    }).href,
)

const hasFilter = computed(
  () => !!props.component.chart_map_link && !!props.component.has_map_layers,
)
const hasAnyTag = computed(
  () => hasFilter.value || !!props.component.has_map_layers || props.component.has_history,
)
const tip = ref<{ x: number; y: number } | null>(null)
function showTip(e: MouseEvent) {
  if (hasAnyTag.value) tip.value = { x: e.clientX, y: e.clientY }
}
function showTipAt(e: FocusEvent) {
  const r = (e.target as HTMLElement).getBoundingClientRect()
  tip.value = { x: r.left + 40, y: r.top + 4 }
}

function openInfo() {
  dialogs.openInfo(props.component, effectiveCounty.value)
}
</script>

<template>
  <article :class="['component-card', `mode-${mode}`]">
    <header class="head">
      <h2>
        <span class="name">{{ component.name }}</span>
        <span v-if="freq" class="tag">{{ freq }}</span>
      </h2>
      <p class="sub">
        {{ agencies || '—' }} | {{ formatObserved(result?.meta.observed_at) }}
        <span v-if="delayed" class="mi delay" title="資料可能延遲" aria-label="資料延遲"
          >warning</span
        >
      </p>
      <div class="city-tags">
        <span class="tag city">{{ scopeLabel }}</span>
      </div>
    </header>

    <p v-if="paused" class="paused" role="status" :title="pausedTitle">
      <span class="mi" aria-hidden="true">pause_circle</span>
      <span class="paused-text">資料來源暫停更新</span>
      <span v-if="fresh.lastIngestedAt" class="paused-since"
        >最後取得 {{ formatObserved(fresh.lastIngestedAt) }}</span
      >
    </p>

    <div v-if="chartTypes.length > 1" class="control" role="group" aria-label="圖表類型">
      <button
        v-for="t in chartTypes"
        :key="t"
        type="button"
        :class="{ active: t === chartType }"
        :aria-pressed="t === chartType"
        @click="pick(t)"
      >
        {{ CHART_LABELS[t] ?? t }}
      </button>
    </div>

    <div class="chart">
      <ChartView
        v-if="turn && (result || mapFirst)"
        :type="result ? chartType : 'CountyMapChart'"
        :data="result?.data ?? []"
        :meta="result?.meta ?? PENDING_META"
        :pending="!result"
        :paused="paused"
        :component="component"
        :counties="catalog.counties"
        :selected-county="county"
        height="100%"
        @select-county="emit('selectCounty', $event)"
      />
      <ChartSkeleton
        v-else
        :kind="chartKind(component.chart_types[0])"
        role="status"
        aria-label="載入中"
      />
    </div>

    <footer v-if="mode !== 'large'" class="foot">
      <div
        v-if="mode !== 'map'"
        class="tags"
        role="group"
        :tabindex="hasAnyTag ? 0 : undefined"
        aria-label="組件資訊與功能"
        @mouseenter="showTip"
        @mousemove="showTip"
        @mouseleave="tip = null"
        @focus="showTipAt"
        @blur="tip = null"
      >
        <span v-if="hasFilter" class="tag hide-mobile"
          ><span class="mi" aria-hidden="true">tune</span>篩選地圖</span
        >
        <span v-if="component.has_map_layers" class="tag hide-mobile"
          ><span class="mi" aria-hidden="true">map</span>空間資料</span
        >
        <span v-if="component.has_history" class="tag"
          ><span class="mi" aria-hidden="true">insights</span>歷史資料</span
        >
      </div>
      <span v-else></span>
      <TagTooltip
        v-if="tip"
        :x="tip.x"
        :y="tip.y"
        :has-filter="hasFilter"
        :has-map-layer="!!component.has_map_layers"
        :has-history="component.has_history"
      />
      <a v-if="mode === 'embed'" class="info" :href="detailHref" target="_blank" rel="noopener"
        >dashboard.tw<span class="mi" aria-hidden="true">arrow_circle_right</span></a
      >
      <button
        v-else
        type="button"
        class="info"
        :aria-label="`組件資訊：${component.name}`"
        @click="openInfo"
      >
        組件資訊<span class="mi" aria-hidden="true">arrow_circle_right</span>
      </button>
    </footer>
  </article>
</template>

<style scoped>
.component-card {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 330px;
  padding: var(--font-m);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text-primary);
}
@media (min-width: 1050px) {
  .component-card {
    height: 370px;
  }
}
@media (min-width: 1900px) {
  .component-card {
    height: 400px;
  }
}
.mode-large {
  height: 100%;
  min-height: 350px;
  padding: 0;
  background: transparent;
}
.mode-map {
  height: 300px;
  padding: 0;
  background: transparent;
}
.mode-map h2 {
  display: none;
}
.mode-embed {
  height: 100vh;
  border-radius: 0;
}

h2 {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  font-size: var(--font-m);
  font-weight: 700;
  line-height: 1.4;
}
.name {
  min-width: 0;
}
.sub {
  display: flex;
  align-items: center;
  color: var(--text-muted);
  font-size: var(--font-s);
  font-weight: 400;
}
.delay {
  margin-left: 4px;
  color: var(--danger-text);
  font-size: 1rem;
}
.paused {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 6px;
  padding: 4px 8px;
  border-radius: var(--radius);
  background: var(--warning-bg);
  color: var(--warning-text);
  font-size: var(--font-s);
  line-height: 1.4;
}
.paused .mi {
  font-size: 1rem;
}
.paused-text {
  font-weight: 700;
}
.paused-since {
  margin-left: auto;
  white-space: nowrap;
}
.tag {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  gap: 4px;
  height: 1.5rem;
  padding: 0 4px;
  border: 1px solid var(--text-muted);
  border-radius: var(--radius);
  color: var(--text-muted);
  font-size: var(--font-s);
  font-weight: 400;
  white-space: nowrap;
}
h2 .tag {
  height: 1rem;
  margin-top: 4px;
  padding: 0 2px;
}
.city-tags {
  display: flex;
  margin-top: 2px;
}
.tag.city {
  height: 1rem;
  padding: 0 2px;
  border-color: #5a9cf8;
  background: var(--tag-national, #1411ac);
  color: #ffffff;
}

.control {
  display: flex;
  justify-content: center;
  gap: 4px;
  padding: 8px 0;
}
.control button {
  padding: 2px 4px;
  border: 0;
  border-radius: var(--radius);
  background: var(--button-muted);
  color: var(--text-secondary);
  font-size: var(--font-s);
  cursor: pointer;
  transition: color 0.2s;
}
.control button:hover {
  color: var(--text-primary);
}
.control button.active {
  background: var(--text-secondary);
  color: var(--surface);
}

.chart {
  position: relative;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 26px;
  margin-top: 4px;
}
.tags {
  display: flex;
  gap: 4px;
}
@media (max-width: 759px) {
  .hide-mobile {
    display: none;
  }
}
.tags .mi {
  font-size: 1rem;
}
.info {
  display: flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  padding: 0;
  color: var(--accent);
  font-size: var(--font-ms);
  text-decoration: none;
  cursor: pointer;
  transition: opacity 0.2s;
}
.info:hover {
  opacity: 0.8;
}
</style>
