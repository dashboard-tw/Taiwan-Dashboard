<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import {
  createComponent,
  getComponentSpec,
  getVersion,
  getVersions,
  previewComponent,
  restoreVersion,
  saveComponent,
  setComponentStatus,
  type ComponentSpec,
  type ComponentVersion,
  type MapLayerSpec,
  type Preview,
  type PreviewCheck,
  type QuerySpec,
} from '../../api/admin'
import { getDatasetHealth, type DatasetHealth } from '../../api/admin'
import { ApiError, getCounties } from '../../api/client'
import type { ComponentInfo, County, DataMeta, DataShape } from '../../api/types'
import ChartView from '../../components/charts/ChartView.vue'
import { usePageTitle } from '../../composables/usePageTitle'
import { TOPIC_LABELS } from '../../lib/topics'

const props = defineProps<{ slug?: string }>()
const route = useRoute()
const router = useRouter()
const isNew = computed(() => !props.slug)
usePageTitle(() => (isNew.value ? '新增元件' : `編輯元件 ${props.slug}`))

const TOPICS = TOPIC_LABELS
const RANGES: Record<string, string> = {
  current: '即時',
  day: '一天',
  week: '一週',
  month: '一個月',
  quarter: '三個月',
  halfyear: '半年',
  year: '一年',
  twoyear: '兩年',
  fiveyear: '五年',
}
const SHAPES: Record<DataShape, string> = {
  by_county: 'by_county：各縣市一個數值（county_code, data）',
  two_d: 'two_d：類別與數值（x_axis, data）',
  three_d: 'three_d：多組類別（x_axis, y_axis, data）',
  percent: 'percent：百分比堆疊（x_axis, y_axis, data）',
  time: 'time：時間序列（x_axis 時間, y_axis, data）',
  map_legend: 'map_legend：地圖圖例（name, type）',
}
const CHART_TYPES: Record<string, string> = {
  CountyMapChart: '縣市地圖',
  BarChart: '橫向長條圖',
  ColumnChart: '直條圖',
  DonutChart: '圓餅圖',
  TreemapChart: '矩形樹狀圖',
  NumberChart: '數字',
  ListChart: '清單',
  TimelineSeparateChart: '折線圖',
  TimelineStackedChart: '堆疊面積圖',
  ImageChart: '影像（雷達、衛星雲圖）',
}
const LAYER_TYPES = ['circle', 'symbol', 'fill', 'line', 'heatmap', 'raster', 'wind'] as const

function blank(): ComponentSpec {
  return {
    slug: '',
    name: '',
    topic: 'aqi',
    short_desc: '',
    long_desc: '',
    use_case: '',
    dataset_ids: [],
    time_range: 'current',
    chart_types: ['BarChart'],
    colors: [],
    unit: '',
    decimals: 0,
    links: [],
    status: 'draft',
    queries: [{ scope: 'national', data_shape: 'two_d', chart_sql: '', history_sql: '' }],
    map_layers: [],
  }
}

interface LayerText {
  style: string
  popup: string
}
const spec = ref<ComponentSpec | null>(null)
const layerText = ref<LayerText[]>([])
const linkText = ref('')
const colorText = ref('')
const mapLinkText = ref('')
const version = ref(0)
const saved = ref('') // 上次載入或儲存時的內容，用來判斷有沒有未儲存的修改

const datasets = ref<DatasetHealth[]>([])
const counties = ref<County[]>([])
const loadError = ref('')
const message = ref<{ ok: boolean; text: string } | null>(null)
const busy = ref<'' | 'preview' | 'save' | 'status' | 'restore'>('')
const note = ref('')

const preview = ref<Preview | null>(null)
const previewCounty = ref('63000')
const previewError = ref('')

const versions = ref<ComponentVersion[] | null>(null)
const showVersions = ref(false)

function pretty(v: unknown, empty: string) {
  return v === undefined || v === null ? empty : JSON.stringify(v, null, 2)
}

function load(s: ComponentSpec) {
  for (const q of s.queries) q.history_sql ??= ''
  for (const l of s.map_layers) l.geojson_sql ??= ''
  spec.value = s
  layerText.value = s.map_layers.map((l) => ({
    style: pretty(l.style, '{}'),
    popup: pretty(l.popup_fields, '[]'),
  }))
  linkText.value = s.links.join('\n')
  colorText.value = s.colors.join(', ')
  mapLinkText.value = pretty(s.chart_map_link, '')
}

function build(): ComponentSpec {
  const s = spec.value!
  const parse = (text: string, label: string): unknown => {
    try {
      return JSON.parse(text)
    } catch {
      throw new Error(`${label} 不是正確的 JSON`)
    }
  }
  return {
    ...s,
    decimals: Number(s.decimals) || 0,
    links: linkText.value
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean),
    colors: colorText.value
      .split(',')
      .map((c) => c.trim())
      .filter(Boolean),
    chart_map_link: mapLinkText.value.trim()
      ? parse(mapLinkText.value, '圖表與地圖連動')
      : undefined,
    queries: s.queries.map((q) => ({
      ...q,
      history_sql: q.history_sql?.trim() ? q.history_sql : undefined,
    })),
    map_layers: s.map_layers.map((l, i) => ({
      ...l,
      geojson_sql: l.source_type === 'query' ? l.geojson_sql : undefined,
      style: parse(layerText.value[i]!.style || '{}', `圖層 ${i + 1} 的樣式`),
      popup_fields: parse(layerText.value[i]!.popup || '[]', `圖層 ${i + 1} 的彈窗欄位`),
    })),
  }
}

function snapshot(): string {
  try {
    return JSON.stringify(build())
  } catch {
    return 'invalid'
  }
}
const dirty = computed(() => spec.value !== null && snapshot() !== saved.value)

async function fetchSpec() {
  loadError.value = ''
  try {
    if (props.slug) {
      const r = await getComponentSpec(props.slug)
      load(r.spec)
      version.value = r.version
    } else if (typeof route.query.from === 'string') {
      const r = await getComponentSpec(route.query.from)
      load({ ...r.spec, slug: '', name: `${r.spec.name}（複本）`, status: 'draft' })
    } else {
      load(blank())
    }
    saved.value = props.slug ? snapshot() : ''
  } catch (e) {
    loadError.value =
      e instanceof ApiError && e.status === 404 ? '找不到這個元件' : '元件設定暫時無法取得'
  }
}

async function fetchVersions() {
  if (!props.slug) return
  versions.value = await getVersions(props.slug).catch(() => [])
}

onMounted(() => {
  void fetchSpec()
  void fetchVersions()
  void getDatasetHealth()
    .then((d) => (datasets.value = d))
    .catch(() => {})
  void getCounties()
    .then((c) => (counties.value = c))
    .catch(() => {})
})
watch(
  () => props.slug,
  () => {
    preview.value = null
    message.value = null
    void fetchSpec()
    void fetchVersions()
  },
)

function beforeUnload(e: BeforeUnloadEvent) {
  if (dirty.value) e.preventDefault()
}
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
onBeforeRouteLeave(() => !dirty.value || window.confirm('有修改還沒儲存，確定要離開嗎？'))

const hasCounty = computed(() => spec.value?.queries.some((q) => q.scope === 'county') ?? false)
function toggleCounty(on: boolean) {
  const s = spec.value!
  if (on) {
    const nat = s.queries.find((q) => q.scope === 'national')
    s.queries.push({
      scope: 'county',
      data_shape: nat?.data_shape ?? 'two_d',
      chart_sql: '',
      history_sql: '',
    })
  } else {
    s.queries = s.queries.filter((q) => q.scope !== 'county')
  }
}
const scopeLabel = (q: QuerySpec) => (q.scope === 'national' ? '全國' : '縣市（用 :county 篩選）')

function addLayer() {
  const l: MapLayerSpec = {
    title: '',
    layer_type: 'circle',
    source_type: 'query',
    geojson_sql: '',
  }
  spec.value!.map_layers.push(l)
  layerText.value.push({ style: '{}', popup: '[]' })
}
function removeLayer(i: number) {
  spec.value!.map_layers.splice(i, 1)
  layerText.value.splice(i, 1)
}

function toggle(list: string[], v: string, on: boolean) {
  const i = list.indexOf(v)
  if (on && i < 0) list.push(v)
  if (!on && i >= 0) list.splice(i, 1)
}

async function runPreview() {
  busy.value = 'preview'
  previewError.value = ''
  try {
    preview.value = await previewComponent(build(), previewCounty.value)
  } catch (e) {
    preview.value = null
    previewError.value = e instanceof Error ? e.message : '試跑失敗'
  } finally {
    busy.value = ''
  }
}

const failed = computed(() => preview.value?.checks.filter((c) => c.error).length ?? 0)

const info = computed<ComponentInfo | null>(() => {
  const s = spec.value
  if (!s) return null
  return {
    slug: s.slug,
    name: s.name,
    topic: s.topic,
    short_desc: s.short_desc,
    long_desc: s.long_desc,
    use_case: s.use_case,
    dataset_ids: s.dataset_ids,
    time_range: s.time_range,
    chart_types: s.chart_types,
    colors: colorText.value
      .split(',')
      .map((c) => c.trim())
      .filter(Boolean),
    unit: s.unit,
    decimals: Number(s.decimals) || 0,
    links: s.links,
    chart_map_link: s.chart_map_link ?? null,
    scopes: s.queries.map((q) => q.scope),
    has_history: s.queries.some((q) => !!q.history_sql?.trim()),
    sources: [],
  }
})

function shapeOf(c: PreviewCheck): DataShape {
  if (c.kind === 'history') return 'time'
  return spec.value!.queries.find((q) => q.scope === c.scope)?.data_shape ?? 'two_d'
}
function chartTypeOf(c: PreviewCheck) {
  if (c.kind === 'history') return 'TimelineSeparateChart'
  const types = spec.value!.chart_types.filter(
    (t) => t !== 'CountyMapChart' || shapeOf(c) === 'by_county',
  )
  return types[0] ?? 'BarChart'
}
function metaOf(c: PreviewCheck): DataMeta {
  const d = c.data as { categories?: string[] } | undefined
  return {
    component: spec.value!.slug,
    scope: c.scope ?? 'national',
    data_shape: shapeOf(c),
    county: c.scope === 'county' ? preview.value?.county : undefined,
    observed_at: null,
    sources: [],
    categories: d?.categories,
  }
}
const dataOf = (c: PreviewCheck) => (c.data as { data?: unknown } | undefined)?.data
const featureCount = (c: PreviewCheck) =>
  (c.data as { features?: unknown[] } | undefined)?.features?.length ?? 0
const openRows = ref<Record<string, boolean>>({})

function cell(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'object') return JSON.stringify(v)
  const s = String(v)
  return s.length > 80 ? `${s.slice(0, 80)}…` : s
}

function fail(e: unknown, fallback: string) {
  message.value = {
    ok: false,
    text: e instanceof ApiError || e instanceof Error ? e.message : fallback,
  }
}

async function save() {
  busy.value = 'save'
  message.value = null
  try {
    const s = build()
    if (isNew.value) {
      await createComponent(s, note.value)
      saved.value = snapshot()
      note.value = ''
      await router.replace({ name: 'admin-component', params: { slug: s.slug } })
      message.value = { ok: true, text: '已建立草稿' }
      return
    }
    version.value = await saveComponent(s, note.value, version.value)
    saved.value = snapshot()
    note.value = ''
    message.value = {
      ok: true,
      text:
        s.status === 'published' ? '已儲存，前台會在幾秒內更新' : '已儲存草稿（前台看不到草稿）',
    }
    void fetchVersions()
  } catch (e) {
    fail(e, '儲存失敗')
  } finally {
    busy.value = ''
  }
}

async function setStatus(status: 'draft' | 'published') {
  if (!spec.value || dirty.value) return
  busy.value = 'status'
  message.value = null
  try {
    version.value = await setComponentStatus(spec.value.slug, status, version.value)
    spec.value.status = status
    saved.value = snapshot()
    message.value = {
      ok: true,
      text: status === 'published' ? '已發布' : '已取消發布，前台不再顯示這個元件',
    }
    void fetchVersions()
  } catch (e) {
    fail(e, '變更狀態失敗')
  } finally {
    busy.value = ''
  }
}

async function loadVersion(v: ComponentVersion) {
  if (!props.slug) return
  if (dirty.value && !window.confirm('有修改還沒儲存，要用這個版本取代嗎？')) return
  try {
    const r = await getVersion(props.slug, v.id)
    load(r.spec)
    message.value = { ok: true, text: `已載入版本 #${v.id}` }
  } catch (e) {
    fail(e, '版本暫時無法取得')
  }
}

async function restore(v: ComponentVersion) {
  if (!props.slug) return
  if (!window.confirm(`還原版本 #${v.id}？`)) return
  busy.value = 'restore'
  message.value = null
  try {
    await restoreVersion(props.slug, v.id)
    await fetchSpec()
    await fetchVersions()
    message.value = { ok: true, text: `已還原版本 #${v.id}` }
  } catch (e) {
    fail(e, '還原失敗')
  } finally {
    busy.value = ''
  }
}

function time(iso: string) {
  return new Date(iso).toLocaleString('zh-TW', {
    timeZone: 'Asia/Taipei',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}
</script>

<template>
  <div>
    <div class="page-title">
      <h1>
        <RouterLink class="back" :to="{ name: 'admin-components' }" aria-label="回到元件清單">
          <span class="mi" aria-hidden="true">arrow_back</span>
        </RouterLink>
        {{ isNew ? '新增元件' : (spec?.name ?? slug) }}
        <span
          v-if="spec && !isNew"
          :class="['badge', spec.status === 'published' ? 'ok' : 'muted']"
        >
          {{ spec.status === 'published' ? '已發布' : '草稿' }}
        </span>
      </h1>
      <span class="tools">
        <button
          v-if="!isNew"
          type="button"
          class="btn"
          :aria-pressed="showVersions"
          @click="showVersions = !showVersions"
        >
          <span class="mi" aria-hidden="true">history</span>版本
        </button>
        <template v-if="spec && !isNew">
          <button
            v-if="spec.status === 'draft'"
            type="button"
            class="btn"
            :disabled="dirty || !!busy"
            :title="dirty ? '先儲存修改再發布' : ''"
            @click="setStatus('published')"
          >
            <span class="mi" aria-hidden="true">publish</span>發布
          </button>
          <button
            v-else
            type="button"
            class="btn"
            :disabled="dirty || !!busy"
            :title="dirty ? '先儲存修改' : ''"
            @click="setStatus('draft')"
          >
            <span class="mi" aria-hidden="true">unpublished</span>取消發布
          </button>
        </template>
      </span>
    </div>
    <p v-if="loadError" class="notice bad" role="alert">{{ loadError }}</p>

    <div v-if="spec" class="editor-layout" :class="{ 'with-versions': showVersions }">
      <form class="form" @submit.prevent="save">
        <section class="card block">
          <h2>基本資料</h2>
          <div class="grid">
            <label class="field">
              <span>代碼</span>
              <input
                v-model="spec.slug"
                class="input"
                :readonly="!isNew"
                pattern="[a-z0-9]+(-[a-z0-9]+)*"
                required
              />
            </label>
            <label class="field">
              <span>名稱</span>
              <input v-model="spec.name" class="input" required />
            </label>
            <label class="field">
              <span>主題</span>
              <select v-model="spec.topic" class="select">
                <option v-for="(label, k) in TOPICS" :key="k" :value="k">{{ label }}</option>
              </select>
            </label>
            <label class="field">
              <span>資料區間</span>
              <select v-model="spec.time_range" class="select">
                <option v-for="(label, k) in RANGES" :key="k" :value="k">{{ label }}</option>
              </select>
            </label>
          </div>
          <label class="field">
            <span>簡短說明</span>
            <input v-model="spec.short_desc" class="input" />
          </label>
          <label class="field">
            <span>詳細說明</span>
            <textarea v-model="spec.long_desc" rows="3"></textarea>
          </label>
          <label class="field">
            <span>使用情境</span>
            <textarea v-model="spec.use_case" rows="2"></textarea>
          </label>
        </section>

        <section class="card block">
          <h2>顯示</h2>
          <div class="field">
            <span>圖表類型</span>
            <div class="checks">
              <label v-for="(label, t) in CHART_TYPES" :key="t" class="check">
                <input
                  type="checkbox"
                  :checked="spec.chart_types.includes(t)"
                  @change="toggle(spec.chart_types, t, ($event.target as HTMLInputElement).checked)"
                />{{ label }}
              </label>
            </div>
          </div>
          <div class="grid three">
            <label class="field">
              <span>單位</span>
              <input v-model="spec.unit" class="input" />
            </label>
            <label class="field">
              <span>小數位數</span>
              <input v-model.number="spec.decimals" class="input" type="number" min="0" max="6" />
            </label>
            <label class="field">
              <span>顏色</span>
              <input v-model="colorText" class="input" placeholder="#5a9cf8, #f5c26b" />
            </label>
          </div>
          <label class="field">
            <span>相關連結</span>
            <textarea v-model="linkText" rows="2"></textarea>
          </label>
        </section>

        <section class="card block">
          <h2>資料來源</h2>
          <div class="checks">
            <label v-for="d in datasets" :key="d.id" class="check">
              <input
                type="checkbox"
                :checked="spec.dataset_ids.includes(d.id)"
                @change="
                  toggle(spec.dataset_ids, d.id, ($event.target as HTMLInputElement).checked)
                "
              />{{ d.name }}<span class="muted">（{{ d.id }}）</span>
            </label>
          </div>
        </section>

        <section class="card block">
          <h2>
            查詢
            <label class="check">
              <input
                type="checkbox"
                :checked="hasCounty"
                @change="toggleCounty(($event.target as HTMLInputElement).checked)"
              />提供縣市版本
            </label>
          </h2>
          <p class="muted small">
            可用的參數：<code>:county</code>（縣市代碼）、<code>:from</code>、<code>:to</code>、<code>:step</code>（hour、day、month）。
            查詢以唯讀帳號執行，只能讀 ref 與 data。
          </p>
          <div v-for="q in spec.queries" :key="q.scope" class="query">
            <h3>{{ scopeLabel(q) }}</h3>
            <label class="field">
              <span>資料格式</span>
              <select v-model="q.data_shape" class="select">
                <option v-for="(label, k) in SHAPES" :key="k" :value="k">{{ label }}</option>
              </select>
            </label>
            <label class="field">
              <span>圖表查詢 chart_sql</span>
              <textarea v-model="q.chart_sql" class="sql" spellcheck="false" required></textarea>
            </label>
            <label class="field">
              <span>歷史查詢 history_sql</span>
              <textarea v-model="q.history_sql" class="sql" spellcheck="false"></textarea>
            </label>
          </div>
        </section>

        <section class="card block">
          <h2>
            地圖圖層
            <button type="button" class="btn" @click="addLayer">
              <span class="mi" aria-hidden="true">add</span>新增圖層
            </button>
          </h2>
          <p v-if="!spec.map_layers.length" class="muted small">還沒有圖層。</p>
          <div v-for="(l, i) in spec.map_layers" :key="i" class="query">
            <h3>
              圖層 {{ i + 1 }}
              <button type="button" class="btn" @click="removeLayer(i)">
                <span class="mi" aria-hidden="true">delete</span>移除
              </button>
            </h3>
            <div class="grid three">
              <label class="field">
                <span>標題</span>
                <input v-model="l.title" class="input" required />
              </label>
              <label class="field">
                <span>圖層類型</span>
                <select v-model="l.layer_type" class="select">
                  <option v-for="t in LAYER_TYPES" :key="t" :value="t">{{ t }}</option>
                </select>
              </label>
              <label class="field">
                <span>資料來源</span>
                <select v-model="l.source_type" class="select">
                  <option value="query">query：geojson_sql</option>
                  <option value="county_boundary">county_boundary：縣市界線＋元件數值</option>
                </select>
              </label>
            </div>
            <label v-if="l.source_type === 'query'" class="field">
              <span>geojson_sql</span>
              <textarea v-model="l.geojson_sql" class="sql" spellcheck="false"></textarea>
            </label>
            <div class="grid">
              <label class="field">
                <span>樣式 style（JSON）</span>
                <textarea
                  v-model="layerText[i]!.style"
                  class="sql short"
                  spellcheck="false"
                ></textarea>
              </label>
              <label class="field">
                <span>彈窗欄位 popup_fields（JSON）</span>
                <textarea
                  v-model="layerText[i]!.popup"
                  class="sql short"
                  spellcheck="false"
                ></textarea>
              </label>
            </div>
          </div>
          <label class="field">
            <span>圖表與地圖連動 chart_map_link</span>
            <textarea v-model="mapLinkText" class="sql short" spellcheck="false"></textarea>
          </label>
        </section>

        <div class="savebar card">
          <span v-if="message" :class="message.ok ? 'ok' : 'bad'" role="status">{{
            message.text
          }}</span>
          <span v-else-if="dirty" class="muted">有未儲存的修改</span>
          <input
            v-model="note"
            class="input note"
            maxlength="200"
            placeholder="修改說明"
            aria-label="修改說明"
          />
          <button type="submit" class="btn-highlight" :disabled="!!busy || (!dirty && !isNew)">
            <span class="mi" aria-hidden="true">save</span>
            {{ busy === 'save' ? '檢查並儲存中' : isNew ? '建立草稿' : '儲存' }}
          </button>
        </div>
      </form>

      <aside class="side">
        <section class="card block preview">
          <h2>
            試跑
            <select v-model="previewCounty" class="select" aria-label="縣市查詢用的縣市">
              <option v-for="c in counties" :key="c.code" :value="c.code">{{ c.name }}</option>
            </select>
            <button type="button" class="btn-highlight" :disabled="!!busy" @click="runPreview">
              <span class="mi" aria-hidden="true">play_arrow</span>
              {{ busy === 'preview' ? '執行中' : '執行' }}
            </button>
          </h2>
          <p v-if="previewError" class="notice bad" role="alert">{{ previewError }}</p>
          <template v-else-if="preview">
            <p :class="failed ? 'bad' : 'ok'" class="small">
              {{ failed ? `${failed} 段查詢有問題` : '每一段查詢都通過' }}
            </p>
            <div v-for="c in preview.checks" :key="c.label" class="check-result">
              <h3>
                <span class="mi" :class="c.error ? 'bad' : 'ok'" aria-hidden="true">{{
                  c.error ? 'error' : 'check_circle'
                }}</span>
                {{ c.label }}
                <span class="muted small">{{ c.row_count }} 列・{{ c.elapsed_ms }} ms</span>
              </h3>
              <pre v-if="c.error" class="error">{{ c.error }}</pre>
              <div v-if="!c.error && c.kind !== 'layer' && info" class="chart">
                <ChartView
                  :type="chartTypeOf(c)"
                  :data="dataOf(c)"
                  :meta="metaOf(c)"
                  :component="info"
                  :counties="counties"
                  :height="240"
                />
              </div>
              <p v-if="!c.error && c.kind === 'layer'" class="small">
                {{ featureCount(c) }} 個地圖物件
              </p>
              <button
                v-if="c.columns.length"
                type="button"
                class="btn rows-toggle"
                :aria-expanded="!!openRows[c.label]"
                @click="openRows[c.label] = !openRows[c.label]"
              >
                {{ openRows[c.label] ? '隱藏' : '查看' }}查詢結果
              </button>
              <div v-if="openRows[c.label]" class="rows">
                <table>
                  <thead>
                    <tr>
                      <th v-for="col in c.columns" :key="col">{{ col }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(r, i) in c.rows" :key="i">
                      <td v-for="(v, j) in r" :key="j">{{ cell(v) }}</td>
                    </tr>
                  </tbody>
                </table>
                <p v-if="c.row_count > c.rows.length" class="muted small">
                  只列出前 {{ c.rows.length }} 列
                </p>
              </div>
            </div>
          </template>
        </section>

        <section v-if="showVersions && !isNew" class="card block versions">
          <h2>版本紀錄</h2>
          <ol v-if="versions">
            <li v-for="v in versions" :key="v.id" :class="{ current: v.id === version }">
              <div>
                <strong>#{{ v.id }}</strong> {{ v.note || '（沒有說明）' }}
                <span class="sub"
                  >{{ time(v.created_at) }}・{{ v.created_by ?? '命令列匯入' }}・{{
                    v.status === 'published' ? '已發布' : '草稿'
                  }}</span
                >
              </div>
              <span v-if="v.id === version" class="muted small">目前</span>
              <template v-else>
                <button type="button" class="btn" @click="loadVersion(v)">載入</button>
                <button type="button" class="btn" :disabled="!!busy" @click="restore(v)">
                  還原
                </button>
              </template>
            </li>
          </ol>
          <p v-else class="skeleton cell"></p>
        </section>
      </aside>
    </div>

    <div v-else-if="!loadError" class="editor-layout" aria-label="載入中">
      <div class="form">
        <section v-for="h in [10, 7, 4, 14]" :key="h" class="card block">
          <span class="skeleton title-sk"></span>
          <span class="skeleton" :style="{ display: 'block', height: `${h}rem` }"></span>
        </section>
      </div>
      <aside class="side">
        <section class="card block">
          <span class="skeleton title-sk"></span>
          <span class="skeleton" style="display: block; height: 3rem"></span>
        </section>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.back {
  display: inline-flex;
  color: var(--text-muted);
}
.back:hover {
  color: var(--text-primary);
}
.editor-layout {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: var(--font-m);
  align-items: start;
}
.form > .block,
.side > .block {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.form h2,
.side h2 {
  margin: 0;
}
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem 1rem;
}
.grid.three {
  grid-template-columns: 1fr 1fr 1fr;
}
.checks {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem 1.25rem;
}
.query {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--border);
}
h3 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  font-weight: 600;
}
textarea.short {
  min-height: 4.5rem;
}
code {
  padding: 0 3px;
  border-radius: 3px;
  background: var(--surface-muted);
  font-size: 0.8125rem;
}
.small {
  font-size: var(--font-s);
}
.savebar {
  position: sticky;
  bottom: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem var(--font-m);
  border-top: 1px solid var(--border);
  font-size: 0.875rem;
}
.note {
  flex: 1;
  max-width: 28rem;
  margin-left: auto;
}
.side {
  position: sticky;
  top: calc(var(--header-h) + var(--font-m));
  display: flex;
  flex-direction: column;
  gap: var(--font-m);
  max-height: calc(100vh - var(--header-h) - 2 * var(--font-m));
  overflow-y: auto;
}
.check-result {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--border);
}
.check-result h3 .mi {
  font-size: 1.1rem;
}
.check-result h3 .small {
  margin-left: auto;
  font-weight: 400;
}
.error {
  margin: 0;
  padding: 8px;
  border-radius: var(--radius);
  background: color-mix(in srgb, var(--danger-text) 12%, transparent);
  font-size: var(--font-s);
  white-space: pre-wrap;
  word-break: break-word;
}
.chart {
  height: 240px;
}
.rows-toggle {
  align-self: flex-start;
}
.rows {
  max-height: 16rem;
  overflow: auto;
}
.rows td {
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: var(--font-s);
}
.versions ol {
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 0.875rem;
}
.versions li {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 6px 0;
  border-bottom: 1px solid var(--border);
}
.versions li > div {
  flex: 1;
  min-width: 0;
}
.versions li.current strong {
  color: var(--accent);
}
.title-sk {
  display: block;
  width: 6rem;
  height: 1rem;
}
</style>
