<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getComponent } from '../../api/client'
import type { ComponentInfo } from '../../api/types'
import { useComponentData } from '../../composables/useComponentData'
import { formatObserved } from '../../lib/format'
import { useCatalog } from '../../stores/catalog'
import { useDialogs } from '../../stores/dialog'
import ComponentCard from '../ComponentCard.vue'
import ChartView from '../charts/ChartView.vue'
import ChartSkeleton from '../skeleton/ChartSkeleton.vue'

const props = defineProps<{ component: ComponentInfo; county: string | null }>()

const dialogs = useDialogs()
const catalog = useCatalog()

const full = ref<ComponentInfo | null>(null)
watch(
  () => props.component.slug,
  async (slug) => {
    full.value = null
    try {
      const c = await getComponent(slug)
      if (props.component.slug === slug) full.value = c
    } catch {}
  },
  { immediate: true },
)
const component = computed<ComponentInfo>(() => ({ ...props.component, ...(full.value ?? {}) }))
const DAY = 86_400_000
const ranges = computed(() => {
  const freqs = (component.value?.sources ?? []).map((s) => s.update_freq_seconds)
  const fast = freqs.length === 0 || Math.min(...freqs) <= 3600
  return fast
    ? [
        { label: '一天前', days: 1 },
        { label: '一週前', days: 7 },
        { label: '一個月前', days: 30 },
      ]
    : [
        { label: '一季前', days: 91 },
        { label: '半年前', days: 182 },
        { label: '一年前', days: 365 },
      ]
})
const rangeIndex = ref(0)
watch(
  () => props.component.slug,
  () => (rangeIndex.value = 0),
)

const span = computed(() => {
  const days = ranges.value[rangeIndex.value]?.days ?? 1
  const to = new Date()
  to.setSeconds(0, 0) // whole minutes, matching the API's cache key
  return { from: new Date(to.getTime() - days * DAY).toISOString(), to: to.toISOString() }
})
const historyQuery = computed(() => ({
  ...(props.county ? { county: props.county } : {}),
  ...span.value,
}))
const history = useComponentData(
  () => component.value.slug,
  'history',
  historyQuery,
  () => component.value.has_history,
)
const historyEmpty = computed(() => {
  const d = history.result.value?.data as { data: unknown[] }[] | undefined
  return !d || d.every((s) => s.data.length === 0)
})

function hostname(link: string): string {
  try {
    return new URL(link).hostname
  } catch {
    return link
  }
}
</script>

<template>
  <div class="info-body">
    <ComponentCard :component="component" :county="county" mode="large" />

    <div class="info">
      <div class="info-data">
        <h2>組件說明（{{ component.slug }}｜{{ component.dataset_ids.join('、') }}）</h2>
        <p>{{ component.long_desc || component.short_desc }}</p>

        <template v-if="component.use_case">
          <h2>範例情境</h2>
          <p>{{ component.use_case }}</p>
        </template>

        <div v-if="component.has_history" class="history">
          <h2>歷史軸</h2>
          <div class="ranges" role="group" aria-label="歷史資料的時間範圍">
            <button
              v-for="(r, i) in ranges"
              :key="r.label"
              type="button"
              :class="{ active: i === rangeIndex }"
              :aria-pressed="i === rangeIndex"
              @click="rangeIndex = i"
            >
              {{ r.label }}
            </button>
          </div>
          <div class="history-chart">
            <ChartView
              v-if="history.result.value && !historyEmpty && catalog.counties.length"
              type="TimelineSeparateChart"
              :data="history.result.value.data"
              :meta="history.result.value.meta"
              :component="component"
              :counties="catalog.counties"
              height="100%"
            />
            <ChartSkeleton v-else-if="!history.result.value" kind="line" />
            <div v-else class="state">
              <p>這段期間沒有資料</p>
            </div>
          </div>
        </div>

        <h2>相關資料</h2>
        <div class="notice">資料可能因更新時間不同，與原始來源略有差異。</div>
        <ul class="links">
          <li v-for="s in component.sources" :key="s.dataset">
            <a :href="s.url" target="_blank" rel="noreferrer">{{ s.name }}（{{ s.agency }}）</a>
            <span>{{ s.license }}・資料時間 {{ formatObserved(s.last_observed_at) }}</span>
          </li>
          <li v-for="l in component.links" :key="l">
            <a :href="l" target="_blank" rel="noreferrer">{{ hostname(l) }}</a>
          </li>
        </ul>
      </div>

      <div class="info-control">
        <button
          type="button"
          class="btn-highlight"
          @click="dialogs.openDownload(component, county)"
        >
          <span class="mi" aria-hidden="true">download</span>下載
        </button>
        <button type="button" class="btn-highlight" @click="dialogs.openEmbed(component, county)">
          <span class="mi" aria-hidden="true">code</span>內嵌
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.info-body {
  display: grid;
  width: min(400px, calc(100vw - 4rem));
}
@media (min-width: 51.25em) {
  .info-body {
    width: 720px;
    height: 410px;
    grid-template-columns: 3fr 2fr;
  }
}
@media (min-width: 75em) {
  .info-body {
    width: 820px;
    height: 440px;
  }
}
.info {
  display: flex;
  flex-direction: column;
  min-height: 0;
  padding: var(--font-ms);
  border-top: 1px solid var(--border);
}
@media (min-width: 51.25em) {
  .info {
    margin-left: var(--font-ms);
    border-top: 0;
    border-left: 1px solid var(--border);
  }
}
.info-data {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: 8px;
}
.info-data::-webkit-scrollbar {
  width: 4px;
}
.info-data::-webkit-scrollbar-thumb {
  border-radius: 4px;
  background: rgb(136 135 135 / 50%);
}
h2 {
  font-size: var(--font-ms);
  font-weight: 700;
}
.info-data p {
  margin-bottom: 0.75rem;
  color: var(--text-muted);
  text-align: justify;
  font-size: 0.9rem;
}
.ranges {
  display: flex;
  gap: 4px;
  margin: 4px 0;
}
.ranges button {
  padding: 2px 4px;
  border: 0;
  border-radius: var(--radius);
  background: var(--button-muted);
  opacity: 0.6;
  color: var(--text-muted);
  font-size: var(--font-s);
  cursor: pointer;
}
.ranges button.active {
  opacity: 1;
  background: var(--text-muted);
  color: #ffffff;
}
.history-chart {
  position: relative;
  height: 170px;
  margin-bottom: 0.5rem;
}
.state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
  color: var(--text-muted);
  font-size: var(--font-s);
}
.state .mi {
  font-size: 1.6rem;
  margin-bottom: 0.3rem;
}
.notice {
  margin: 4px 0 8px;
  padding: 8px 12px;
  border: 1px solid #878787;
  border-radius: 8px;
  color: var(--text-muted);
  font-size: var(--font-s);
  line-height: 1.5;
}
.links {
  margin: 0 0 var(--font-s);
  padding: 0;
  list-style: none;
  display: grid;
  gap: 4px;
}
.links li {
  display: flex;
  flex-direction: column;
  font-size: var(--font-s);
}
.links a {
  color: var(--text-muted);
  transition: color 0.2s;
}
.links a:hover {
  color: var(--accent);
}
.links span {
  color: var(--text-muted);
  opacity: 0.8;
}
.info-control {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding-top: 8px;
}
.info-control .mi {
  font-size: var(--font-m);
}
</style>
