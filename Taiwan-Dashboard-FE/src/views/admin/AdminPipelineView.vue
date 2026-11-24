<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  getDatasetHealth,
  getPipelineRuns,
  getRunAll,
  runDataset,
  startRunAll,
  type DatasetHealth,
  type PipelineRun,
  type RunAllState,
} from '../../api/admin'
import { ApiError } from '../../api/client'
import { usePageTitle } from '../../composables/usePageTitle'
import { formatFreq } from '../../lib/format'
import { rememberCount, rememberedCount } from '../../lib/rememberCount'

usePageTitle('資料管線')

const DATASET_COLUMNS = [
  '資料集',
  '狀態',
  '更新頻率',
  '資料時間',
  '最後抓取',
  '今日執行',
  '錯誤訊息',
  '',
]
const RUN_COLUMNS = ['開始', '資料集', '結果', '抓到', '寫入', '耗時', '警告與錯誤']
const datasetRows = rememberedCount('admin-datasets', 6)

const datasets = ref<DatasetHealth[] | null>(null)
const runs = ref<PipelineRun[] | null>(null)
const filter = ref('')
const error = ref('')
const loadedAt = ref<Date | null>(null)

async function load() {
  try {
    const [d, r] = await Promise.all([
      getDatasetHealth(),
      getPipelineRuns(filter.value || undefined),
    ])
    datasets.value = d
    runs.value = r
    rememberCount('admin-datasets', d.length)
    loadedAt.value = new Date()
    error.value = ''
  } catch {
    error.value = '後台資料暫時無法取得'
  }
}

let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void load()
  timer = setInterval(() => void load(), 60_000)
  void pollAll()
})
onBeforeUnmount(() => {
  clearInterval(timer)
  clearTimeout(allTimer)
})

const all = ref<RunAllState | null>(null)
const allError = ref('')
const allStarting = ref(false)
let allTimer: ReturnType<typeof setTimeout> | undefined

async function pollAll() {
  clearTimeout(allTimer)
  try {
    const was = all.value?.running
    all.value = await getRunAll()
    if (all.value.running) allTimer = setTimeout(() => void pollAll(), 2000)
    else if (was) void load()
  } catch {
    if (all.value?.running) allTimer = setTimeout(() => void pollAll(), 5000)
  }
}

async function runAll() {
  allStarting.value = true
  allError.value = ''
  try {
    all.value = await startRunAll()
    allTimer = setTimeout(() => void pollAll(), 1000)
  } catch (e) {
    allError.value = e instanceof ApiError ? e.message : '無法開始全部更新，請稍後再試'
  } finally {
    allStarting.value = false
  }
}

const allPercent = computed(() =>
  all.value && all.value.total ? Math.round((all.value.done / all.value.total) * 100) : 0,
)
const allCurrent = computed(() => {
  const a = all.value
  return a?.current ? (a.steps.find((s) => s.id === a.current)?.name ?? a.current) : ''
})
const allCounts = computed(() => {
  const c = { ok: 0, warning: 0, failed: 0, skipped: 0 }
  for (const s of all.value?.steps ?? []) if (s.status in c) c[s.status as keyof typeof c]++
  return c
})
const allProblems = computed(() =>
  (all.value?.steps ?? []).filter(
    (s) => s.status === 'failed' || s.status === 'skipped' || s.status === 'warning',
  ),
)
const allElapsed = computed(() => {
  const a = all.value
  if (!a?.started_at) return ''
  const end = a.finished_at ? new Date(a.finished_at).getTime() : Date.now()
  const s = Math.max(0, Math.round((end - new Date(a.started_at).getTime()) / 1000))
  return s < 60 ? `${s} 秒` : `${Math.floor(s / 60)} 分 ${s % 60} 秒`
})
const STEP_LABEL: Record<string, string> = {
  ok: '完成',
  warning: '有警告',
  failed: '失敗',
  skipped: '略過',
}

async function changeFilter(v: string) {
  filter.value = v
  runs.value = null
  runs.value = await getPipelineRuns(v || undefined).catch(() => [])
}

const STATUS: Record<string, { label: string; cls: string }> = {
  ok: { label: '正常', cls: 'ok' },
  stale: { label: '延遲', cls: 'bad' },
  error: { label: '錯誤', cls: 'bad' },
  pending: { label: '尚未取得', cls: 'muted' },
  success: { label: '成功', cls: 'ok' },
  partial: { label: '有警告', cls: 'warn' },
  failed: { label: '失敗', cls: 'bad' },
  running: { label: '執行中', cls: 'muted' },
}

const running = ref<Record<string, boolean>>({})
const results = ref<Record<string, { ok: boolean; text: string }>>({})

async function run(id: string) {
  running.value[id] = true
  delete results.value[id]
  try {
    const r = await runDataset(id)
    if (r.background) {
      results.value[id] = { ok: true, text: '抓取時間較長，已改在背景執行，完成後會更新抓取紀錄' }
      return
    }
    const warn = r.warnings.length ? `。${r.warnings.join('。')}` : ''
    results.value[id] = {
      ok: true,
      text: `完成：抓到 ${r.rows_fetched} 筆，寫入 ${r.rows_inserted} 筆${warn}`,
    }
  } catch (e) {
    results.value[id] = {
      ok: false,
      text: e instanceof ApiError ? e.message : '執行失敗，請稍後再試',
    }
  } finally {
    running.value[id] = false
    void load()
  }
}

const problems = computed(() => (datasets.value ?? []).filter((d) => d.status !== 'ok').length)

function time(iso: string | null): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('zh-TW', {
    timeZone: 'Asia/Taipei',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}
function duration(r: PipelineRun): string {
  if (!r.finished_at) return '—'
  const ms = new Date(r.finished_at).getTime() - new Date(r.started_at).getTime()
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`
}
</script>

<template>
  <div>
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">monitor_heart</span>資料管線
      <span class="tools">
        <span v-if="datasets" :class="problems ? 'bad' : 'ok'">
          {{ problems ? `${problems} 個資料集需要注意` : '所有資料集都正常' }}
        </span>
      </span>
    </h1>
    <p v-if="error" class="bad" role="alert">{{ error }}</p>

    <section class="card block run-all">
      <h2>
        全部更新一次
        <button
          type="button"
          class="btn btn-highlight"
          :disabled="allStarting || all?.running"
          @click="runAll"
        >
          <span class="mi" aria-hidden="true">{{ all?.running ? 'hourglass_top' : 'sync' }}</span>
          {{ all?.running ? '更新中…' : '全部更新一次' }}
        </button>
      </h2>
      <p class="muted">大約要幾分鐘。</p>
      <p v-if="allError" class="bad" role="alert">{{ allError }}</p>
      <template v-if="all && all.total">
        <div
          class="progress"
          role="progressbar"
          :aria-valuenow="all.done"
          aria-valuemin="0"
          :aria-valuemax="all.total"
          :aria-label="`全部更新進度 ${all.done} / ${all.total}`"
        >
          <span :style="{ width: `${allPercent}%` }"></span>
        </div>
        <p class="progress-text" role="status">
          <strong>{{ all.done }} / {{ all.total }}</strong
          >（{{ allPercent }}%）・{{ allElapsed }}
          <template v-if="all.running && allCurrent">・正在抓：{{ allCurrent }}</template>
          <template v-else-if="!all.running">・{{ time(all.finished_at) }} 完成</template>
          <span class="counts">
            <span class="ok">完成 {{ allCounts.ok }}</span>
            <span v-if="allCounts.warning" class="warn">有警告 {{ allCounts.warning }}</span>
            <span v-if="allCounts.failed" class="bad">失敗 {{ allCounts.failed }}</span>
            <span v-if="allCounts.skipped" class="muted">略過 {{ allCounts.skipped }}</span>
          </span>
        </p>
        <ul v-if="allProblems.length" class="problems">
          <li v-for="s in allProblems" :key="s.id">
            <span
              :class="[
                'badge',
                s.status === 'failed' ? 'bad' : s.status === 'warning' ? 'warn' : 'muted',
              ]"
              >{{ STEP_LABEL[s.status] }}</span
            >
            <strong>{{ s.name }}</strong>
            <span class="muted">{{ s.message }}</span>
          </li>
        </ul>
      </template>
    </section>

    <section class="card block">
      <h2>
        資料集健康狀態
        <span v-if="loadedAt" class="loaded-at"
          >畫面更新於 {{ loadedAt.toLocaleTimeString('zh-TW', { hour12: false }) }}</span
        >
      </h2>
      <table>
        <thead>
          <tr>
            <th v-for="c in DATASET_COLUMNS" :key="c">{{ c }}</th>
          </tr>
        </thead>
        <tbody v-if="datasets">
          <template v-for="d in datasets" :key="d.id">
            <tr>
              <td>
                <strong>{{ d.name }}</strong>
                <span class="sub"
                  >{{ d.id }}・<a :href="d.url" target="_blank" rel="noopener">{{
                    d.agency
                  }}</a></span
                >
              </td>
              <td>
                <span :class="['badge', STATUS[d.status]?.cls]">{{
                  STATUS[d.status]?.label ?? d.status
                }}</span>
              </td>
              <td>{{ formatFreq(d.update_freq_seconds) }}</td>
              <td>{{ time(d.last_observed_at) }}</td>
              <td>{{ time(d.last_ingested_at) }}</td>
              <td>
                {{ d.runs_today }} 次<span v-if="d.failures_today" class="bad"
                  >（失敗 {{ d.failures_today }}）</span
                >
              </td>
              <td class="error-cell">{{ d.last_error ?? '' }}</td>
              <td>
                <button
                  type="button"
                  class="btn"
                  :disabled="running[d.id]"
                  :aria-label="`立即抓取${d.name}`"
                  @click="run(d.id)"
                >
                  <span class="mi" aria-hidden="true">{{
                    running[d.id] ? 'hourglass_top' : 'play_arrow'
                  }}</span>
                  {{ running[d.id] ? '抓取中' : '立即抓取' }}
                </button>
              </td>
            </tr>
            <tr v-if="results[d.id]" class="result-row">
              <td
                :colspan="DATASET_COLUMNS.length"
                :class="results[d.id]!.ok ? 'ok' : 'bad'"
                role="status"
              >
                {{ results[d.id]!.text }}
              </td>
            </tr>
          </template>
        </tbody>
        <tbody v-else>
          <tr v-for="i in datasetRows" :key="i">
            <td v-for="c in DATASET_COLUMNS" :key="c"><span class="skeleton cell"></span></td>
          </tr>
        </tbody>
      </table>
    </section>

    <section class="card block">
      <h2>
        最近的抓取紀錄
        <select
          class="select"
          :value="filter"
          aria-label="篩選資料集"
          @change="changeFilter(($event.target as HTMLSelectElement).value)"
        >
          <option value="">全部資料集</option>
          <option v-for="d in datasets ?? []" :key="d.id" :value="d.id">{{ d.name }}</option>
        </select>
      </h2>
      <table>
        <thead>
          <tr>
            <th v-for="c in RUN_COLUMNS" :key="c">{{ c }}</th>
          </tr>
        </thead>
        <tbody v-if="runs">
          <tr v-for="r in runs" :key="r.id">
            <td>{{ time(r.started_at) }}</td>
            <td>{{ r.dataset }}</td>
            <td>
              <span :class="['badge', STATUS[r.status]?.cls]">{{
                STATUS[r.status]?.label ?? r.status
              }}</span>
            </td>
            <td>{{ r.rows_fetched }}</td>
            <td>{{ r.rows_inserted }}</td>
            <td>{{ duration(r) }}</td>
            <td class="error-cell">
              <template v-if="r.error">{{ r.error }}</template>
              <template v-else>{{ r.warnings.join('。') }}</template>
            </td>
          </tr>
          <tr v-if="runs.length === 0">
            <td :colspan="RUN_COLUMNS.length" class="muted">沒有紀錄</td>
          </tr>
        </tbody>
        <tbody v-else>
          <tr v-for="i in 8" :key="i">
            <td v-for="c in RUN_COLUMNS" :key="c"><span class="skeleton cell"></span></td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<style scoped>
.loaded-at {
  color: var(--text-muted);
  font-size: var(--font-s);
  font-weight: 400;
}
.run-all h2 {
  display: flex;
  align-items: center;
  gap: 12px;
}
.run-all h2 .btn {
  margin-left: auto;
}
.progress {
  height: 10px;
  overflow: hidden;
  border-radius: 999px;
  background: var(--border);
}
.progress span {
  display: block;
  height: 100%;
  border-radius: 999px;
  background: var(--accent, #3b82f6);
  transition: width 0.4s ease;
}
.progress-text {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 8px;
  margin: 8px 0 0;
}
.counts {
  display: inline-flex;
  gap: 12px;
  margin-left: auto;
  font-size: var(--font-s);
}
.problems {
  display: grid;
  gap: 4px;
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  font-size: var(--font-s);
}
.problems li {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px;
}
.result-row td {
  border-top: 0;
  font-size: var(--font-s);
}
</style>
