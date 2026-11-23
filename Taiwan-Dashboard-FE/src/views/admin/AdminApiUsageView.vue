<script setup lang="ts">
import type { ApexOptions } from 'apexcharts'
import { computed, onMounted, ref, watch } from 'vue'
import { getApiUsage, revokeUserKey, type AccountStatus, type ApiUsage } from '../../api/admin'
import { ApiError } from '../../api/client'
import ApexChart from '../../components/charts/ApexChart.vue'
import { usePageTitle } from '../../composables/usePageTitle'
import { seriesColor, STATUS_ICONS, STATUS_LABELS, statusColor } from '../../lib/accountStatus'
import { useDatasets } from '../../lib/datasets'
import { formatNumber } from '../../lib/format'

usePageTitle('API 用量')
const { datasetName } = useDatasets()

const RANGES = [7, 30, 90]
const days = ref(30)
const data = ref<ApiUsage | null>(null)
const error = ref('')
const statusFilter = ref<AccountStatus | ''>('')

async function load() {
  try {
    data.value = await getApiUsage(days.value)
    error.value = ''
  } catch {
    error.value = 'API 用量暫時無法取得'
  }
}
onMounted(load)
watch(days, () => {
  data.value = null
  void load()
})

const STATUSES: AccountStatus[] = ['active', 'idle', 'revoked']
const accountTotal = computed(() =>
  data.value ? STATUSES.reduce((n, s) => n + data.value!.accounts[s], 0) : 0,
)
const stackLabel = computed(() =>
  STATUSES.map((s) => `${STATUS_LABELS[s]} ${data.value?.accounts[s] ?? 0}`).join('，'),
)
const share = (n: number, of: number) => (of ? Math.round((n / of) * 100) : 0)

const tiles = computed(() => {
  const t = data.value?.totals
  if (!t) return []
  return [
    { label: '總呼叫次數', value: formatNumber(t.requests), sub: `${days.value} 天` },
    { label: '平均每天', value: formatNumber(t.avg_per_day, 1), sub: '次' },
    {
      label: '平均每個帳號',
      value: formatNumber(t.avg_per_account, 1),
      sub: `${t.accounts} 個帳號有呼叫`,
    },
    { label: '被拒絕', value: formatNumber(t.rejected), sub: '超過每分鐘或每天額度' },
    {
      label: '用完每日額度',
      value: formatNumber(t.quota_hits),
      sub: `帳號×天（上限 ${data.value!.daily_limit} 次）`,
    },
  ]
})

function dayLabel(d: string) {
  return d.slice(5).replace('-', '/')
}

const requestSeries = computed(() => [
  { name: '呼叫次數', data: (data.value?.daily ?? []).map((d) => d.requests) },
])
const requestOptions = computed<ApexOptions>(() => ({
  chart: { type: 'bar' },
  plotOptions: { bar: { borderRadius: 2, columnWidth: '72%' } },
  colors: [seriesColor()],
  dataLabels: { enabled: false },
  xaxis: {
    categories: (data.value?.daily ?? []).map((d) => dayLabel(d.day)),
    labels: { rotate: 0, hideOverlappingLabels: true },
    tickAmount: 8,
  },
  yaxis: { min: 0, forceNiceScale: true, labels: { formatter: (v: number) => formatNumber(v) } },
  tooltip: { y: { formatter: (v: number) => `${formatNumber(v)} 次` } },
}))
const accountSeries = computed(() => [
  { name: '有呼叫的帳號', data: (data.value?.daily ?? []).map((d) => d.accounts) },
])
const accountOptions = computed<ApexOptions>(() => ({
  chart: { type: 'line' },
  stroke: { width: 2, curve: 'straight' },
  markers: { size: 0, hover: { size: 5 } },
  colors: [seriesColor()],
  dataLabels: { enabled: false },
  xaxis: {
    categories: (data.value?.daily ?? []).map((d) => dayLabel(d.day)),
    labels: { rotate: 0, hideOverlappingLabels: true },
    tickAmount: 8,
  },
  yaxis: {
    min: 0,
    forceNiceScale: true,
    labels: { formatter: (v: number) => String(Math.round(v)) },
  },
  tooltip: { y: { formatter: (v: number) => `${v} 個帳號` } },
}))

const datasetMax = computed(() =>
  Math.max(1, ...(data.value?.datasets ?? []).map((d) => d.requests)),
)

const shown = computed(() =>
  (data.value?.top ?? []).filter((a) => !statusFilter.value || a.status === statusFilter.value),
)

const message = ref<{ ok: boolean; text: string } | null>(null)
async function revoke(userId: number, name: string) {
  if (
    !window.confirm(
      `停用「${name}」的所有 API 憑證？使用中的程式會立即失效，對方之後可以重新建立。`,
    )
  )
    return
  try {
    await revokeUserKey(userId)
    message.value = { ok: true, text: `已停用「${name}」的 API 憑證` }
    await load()
  } catch (e) {
    message.value = { ok: false, text: e instanceof ApiError ? e.message : '停用失敗' }
  }
}

function time(iso: string | null) {
  if (!iso) return '從未使用'
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
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">query_stats</span>API 用量
      <span class="tools">
        <span class="range" role="group" aria-label="期間">
          <button
            v-for="r in RANGES"
            :key="r"
            type="button"
            :class="{ on: days === r }"
            :aria-pressed="days === r"
            @click="days = r"
          >
            {{ r }} 天
          </button>
        </span>
      </span>
    </h1>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>
    <p v-if="message" :class="['notice', { bad: !message.ok }]" role="status">{{ message.text }}</p>

    <div class="tiles">
      <template v-if="data">
        <section v-for="t in tiles" :key="t.label" class="card tile">
          <span class="tile-label">{{ t.label }}</span>
          <strong class="tile-value">{{ t.value }}</strong>
          <span class="tile-sub muted">{{ t.sub }}</span>
        </section>
      </template>
      <template v-else>
        <span v-for="i in 5" :key="i" class="card tile"
          ><span class="skeleton tile-skeleton"></span
        ></span>
      </template>
    </div>

    <div class="two">
      <section class="card block">
        <h2>
          帳號狀態<span class="muted small">建立過 API 憑證的 {{ accountTotal }} 個帳號</span>
        </h2>
        <template v-if="data">
          <div class="stack" role="img" :aria-label="stackLabel">
            <i
              v-for="s in STATUSES"
              v-show="data.accounts[s] > 0"
              :key="s"
              :style="{ flexGrow: data.accounts[s], background: statusColor(s) }"
              :title="`${STATUS_LABELS[s]}：${data.accounts[s]}`"
            ></i>
          </div>
          <ul class="legend">
            <li v-for="s in STATUSES" :key="s">
              <i :style="{ background: statusColor(s) }"></i>
              <span class="mi" aria-hidden="true">{{ STATUS_ICONS[s] }}</span>
              <span class="name">{{ STATUS_LABELS[s] }}</span>
              <strong>{{ data.accounts[s] }}</strong>
              <span class="muted">{{ share(data.accounts[s], accountTotal) }}%</span>
            </li>
          </ul>
          <p class="muted small">
            閒置：{{ data.idle_days }} 天沒呼叫。從沒用過：{{ data.accounts.never_used }} 個。
          </p>
        </template>
        <span v-else class="skeleton" style="display: block; height: 7rem"></span>
      </section>

      <section class="card block">
        <h2>各資料集的呼叫次數</h2>
        <ul v-if="data" class="bars">
          <li v-for="d in data.datasets" :key="d.dataset">
            <span class="label">{{ datasetName(d.dataset) }}</span>
            <span class="bar"
              ><i
                :style="{ width: `${(d.requests / datasetMax) * 100}%`, background: seriesColor() }"
              ></i
            ></span>
            <span class="num">{{ formatNumber(d.requests) }}</span>
          </li>
          <li v-if="data.datasets.length === 0" class="muted">這段期間沒有呼叫</li>
        </ul>
        <span v-else class="skeleton" style="display: block; height: 7rem"></span>
      </section>
    </div>

    <div class="two">
      <section class="card block">
        <h2>每天的呼叫次數</h2>
        <div class="chart">
          <ApexChart
            v-if="data"
            type="bar"
            :series="requestSeries"
            :options="requestOptions"
            height="100%"
          />
          <span v-else class="skeleton chart-skeleton"></span>
        </div>
      </section>
      <section class="card block">
        <h2>每天有呼叫的帳號</h2>
        <div class="chart">
          <ApexChart
            v-if="data"
            type="line"
            :series="accountSeries"
            :options="accountOptions"
            height="100%"
          />
          <span v-else class="skeleton chart-skeleton"></span>
        </div>
      </section>
    </div>

    <section class="card block">
      <h2>
        帳號用量
        <select v-model="statusFilter" class="select" aria-label="依狀態篩選">
          <option value="">全部狀態</option>
          <option v-for="s in STATUSES" :key="s" :value="s">{{ STATUS_LABELS[s] }}</option>
        </select>
      </h2>
      <table v-if="data">
        <thead>
          <tr>
            <th>帳號</th>
            <th>狀態</th>
            <th class="num">呼叫</th>
            <th class="num">被拒絕</th>
            <th class="num">使用天數</th>
            <th>最後使用</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="a in shown" :key="a.user_id">
            <td>
              {{ a.display_name
              }}<span v-if="a.prefix" class="sub"
                ><code>{{ a.prefix }}…</code></span
              >
            </td>
            <td>
              <span class="status" :style="{ color: statusColor(a.status) }">
                <span class="mi" aria-hidden="true">{{ STATUS_ICONS[a.status] }}</span>
              </span>
              {{ STATUS_LABELS[a.status] }}
            </td>
            <td class="num">{{ formatNumber(a.requests) }}</td>
            <td class="num" :class="{ warn: a.rejected > 0 }">{{ formatNumber(a.rejected) }}</td>
            <td class="num">{{ a.days_used }}</td>
            <td>{{ time(a.last_used_at) }}</td>
            <td>
              <button
                v-if="a.status !== 'revoked'"
                type="button"
                class="btn"
                @click="revoke(a.user_id, a.display_name)"
              >
                <span class="mi" aria-hidden="true">block</span>停用 key
              </button>
            </td>
          </tr>
          <tr v-if="shown.length === 0">
            <td colspan="7" class="muted">沒有符合的帳號</td>
          </tr>
        </tbody>
      </table>
      <span v-else class="skeleton" style="display: block; height: 8rem"></span>
    </section>
  </div>
</template>

<style scoped>
.range {
  display: inline-flex;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  overflow: hidden;
}
.range button {
  padding: 2px 10px;
  border: 0;
  background: none;
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.875rem;
  cursor: pointer;
}
.range button.on {
  background: var(--accent-soft);
  color: var(--accent);
}
.tiles {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: var(--font-m);
  margin-bottom: var(--font-m);
}
.tile {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-height: 6rem;
  padding: var(--font-m);
}
.tile-label {
  color: var(--text-secondary);
  font-size: 0.875rem;
}
.tile-value {
  font-size: 1.75rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
.tile-sub {
  font-size: var(--font-s);
}
.tile-skeleton {
  display: block;
  height: 100%;
}
.two {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--font-m);
}
.admin h2 .small {
  font-weight: 400;
}
.small {
  font-size: var(--font-s);
}
.stack {
  display: flex;
  gap: 2px;
  height: 0.9rem;
  margin: 0.25rem 0 0.75rem;
  border-radius: 4px;
  overflow: hidden;
  background: var(--surface-muted);
}
.stack i {
  min-width: 4px;
}
.legend {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0 0 0.75rem;
  padding: 0;
  list-style: none;
  font-size: 0.875rem;
}
.legend li {
  display: grid;
  grid-template-columns: 0.75rem 1.2rem 4rem 3rem 3rem;
  align-items: center;
  gap: 6px;
}
.legend i {
  width: 0.75rem;
  height: 0.75rem;
  border-radius: 3px;
}
.legend .mi {
  font-size: 1rem;
  color: var(--text-muted);
}
.legend strong {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.bars {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 0.875rem;
}
.bars li {
  display: grid;
  grid-template-columns: 7rem 1fr 4rem;
  align-items: center;
  gap: 8px;
}
.bar {
  height: 0.7rem;
}
.bar i {
  display: block;
  height: 100%;
  min-width: 2px;
  border-radius: 0 4px 4px 0;
}
.num {
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.chart {
  height: 220px;
}
.chart-skeleton {
  display: block;
  height: 100%;
}
.status .mi {
  font-size: 1rem;
  vertical-align: -3px;
}
code {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: var(--font-s);
}
</style>
