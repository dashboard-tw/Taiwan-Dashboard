<script setup lang="ts">
import type { ApexOptions } from 'apexcharts'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { getAdminOverview, type AdminOverview } from '../../api/admin'
import ApexChart from '../../components/charts/ApexChart.vue'
import { usePageTitle } from '../../composables/usePageTitle'
import { seriesColor } from '../../lib/accountStatus'
import { formatNumber } from '../../lib/format'

usePageTitle('後台總覽')

const data = ref<AdminOverview | null>(null)
const error = ref('')

async function load() {
  try {
    data.value = await getAdminOverview()
    error.value = ''
  } catch {
    error.value = '後台數據暫時無法取得'
  }
}
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void load()
  timer = setInterval(load, 60_000)
})
onBeforeUnmount(() => clearInterval(timer))

function mb(bytes: number) {
  return formatNumber(bytes / 1024 / 1024, 1)
}

const tiles = computed(() => {
  const d = data.value
  if (!d) return []
  const datasetTotal = d.datasets.ok + d.datasets.stale + d.datasets.error + d.datasets.pending
  return [
    {
      label: '帳號',
      value: formatNumber(d.users.total),
      sub: `近 7 天新增 ${d.users.new_7d}，登入 ${d.users.active_7d}`,
      to: '/admin/users',
    },
    {
      label: '登入中的裝置',
      value: formatNumber(d.users.sessions),
      sub: `管理員 ${d.users.admins} 人，停用 ${d.users.disabled} 個帳號`,
      to: '/admin/users',
    },
    {
      label: '今日 API 呼叫',
      value: formatNumber(d.api.requests_today),
      sub: `流量 ${mb(d.api.bytes_today)} MB，被拒絕 ${d.api.rejected_today} 次`,
      to: '/admin/api-usage',
    },
    {
      label: 'API 憑證',
      value: formatNumber(d.api.clients),
      sub: `${d.api.accounts} 個帳號使用中`,
      to: '/admin/api-usage',
    },
    {
      label: '資料來源',
      value: `${d.datasets.ok} / ${datasetTotal}`,
      sub: `正常更新。延遲 ${d.datasets.stale}，錯誤 ${d.datasets.error}`,
      to: '/admin/pipeline',
      warn: d.datasets.error + d.datasets.stale > 0,
    },
    {
      label: '近 24 小時執行',
      value: formatNumber(d.runs_24h.total),
      sub: `失敗 ${formatNumber(d.runs_24h.failed)} 次`,
      to: '/admin/pipeline',
      warn: d.runs_24h.failed > 0,
    },
    {
      label: '元件',
      value: formatNumber(d.components.published),
      sub: `已發布。草稿 ${d.components.draft}，儀表板 ${d.components.dashboards}`,
      to: '/admin/components',
    },
  ]
})

function dayLabel(d: string) {
  return d.slice(5).replace('-', '/')
}
function barOptions(unit: string, color: string): ApexOptions {
  return {
    chart: { type: 'bar' },
    plotOptions: { bar: { borderRadius: 2, columnWidth: '70%' } },
    colors: [color],
    dataLabels: { enabled: false },
    xaxis: {
      categories: (data.value?.days ?? []).map((d) => dayLabel(d.day)),
      labels: { rotate: 0, hideOverlappingLabels: true },
      tickAmount: 7,
    },
    yaxis: {
      min: 0,
      forceNiceScale: true,
      labels: { formatter: (v: number) => formatNumber(v) },
    },
    tooltip: { y: { formatter: (v: number) => `${formatNumber(v)} ${unit}` } },
  }
}
const charts = computed(() => {
  const days = data.value?.days ?? []
  return [
    {
      title: 'API 呼叫次數',
      series: [{ name: '呼叫次數', data: days.map((d) => d.api_requests) }],
      options: barOptions('次', seriesColor()),
    },
    {
      title: '新增帳號',
      series: [{ name: '新增帳號', data: days.map((d) => d.new_users) }],
      options: barOptions('個', '#4fb3a9'),
    },
    {
      title: '資料管線失敗次數',
      series: [{ name: '失敗', data: days.map((d) => d.pipeline_failures) }],
      options: barOptions('次', '#e06a74'),
    },
  ]
})

function time(iso: string | null) {
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
</script>

<template>
  <div>
    <h1 class="page-title"><span class="mi" aria-hidden="true">space_dashboard</span>後台總覽</h1>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>

    <div class="tiles">
      <template v-if="data">
        <RouterLink
          v-for="t in tiles"
          :key="t.label"
          :to="t.to"
          class="card tile"
          :class="{ warn: t.warn }"
        >
          <span class="tile-label">{{ t.label }}</span>
          <strong class="tile-value">{{ t.value }}</strong>
          <span class="tile-sub muted">{{ t.sub }}</span>
        </RouterLink>
      </template>
      <template v-else>
        <span v-for="i in 7" :key="i" class="card tile"
          ><span class="skeleton tile-skeleton"></span
        ></span>
      </template>
    </div>

    <div class="charts">
      <section v-for="c in charts" :key="c.title" class="card block">
        <h2>{{ c.title }}<span class="muted small">近 14 天</span></h2>
        <div class="chart">
          <ApexChart v-if="data" type="bar" :series="c.series" :options="c.options" height="100%" />
          <span v-else class="skeleton chart-skeleton"></span>
        </div>
      </section>
    </div>

    <div class="two">
      <section class="card block">
        <h2>
          需要處理的資料來源
          <RouterLink to="/admin/pipeline" class="small">資料管線</RouterLink>
        </h2>
        <p v-if="data && !data.problems.length" class="ok">所有資料來源都正常更新。</p>
        <table v-else-if="data">
          <thead>
            <tr>
              <th>資料集</th>
              <th>狀態</th>
              <th>最後抓取</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in data.problems" :key="p.id">
              <td>
                {{ p.name }}<span class="sub">{{ p.id }}</span>
                <span v-if="p.last_error" class="error-cell">{{ p.last_error }}</span>
              </td>
              <td :class="p.status === 'error' ? 'bad' : 'warn'">
                {{ p.status === 'error' ? '錯誤' : '延遲' }}
              </td>
              <td class="nowrap">{{ time(p.last_ingested_at) }}</td>
            </tr>
          </tbody>
        </table>
        <span v-else class="skeleton cell"></span>
      </section>

      <section class="card block">
        <h2>最近失敗的執行</h2>
        <p v-if="data && !data.failures.length" class="ok">沒有失敗紀錄。</p>
        <table v-else-if="data">
          <thead>
            <tr>
              <th>時間</th>
              <th>資料集與錯誤</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(f, i) in data.failures" :key="i">
              <td class="nowrap">{{ time(f.started_at) }}</td>
              <td>
                {{ f.dataset }}<span v-if="f.error" class="error-cell">{{ f.error }}</span>
              </td>
            </tr>
          </tbody>
        </table>
        <span v-else class="skeleton cell"></span>
      </section>
    </div>
  </div>
</template>

<style scoped>
.tiles {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
  gap: var(--font-m);
  margin-bottom: var(--font-m);
}
.tile {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-height: 6.5rem;
  padding: var(--font-m);
  color: var(--text-primary);
  text-decoration: none;
  transition: border-color 0.15s;
}
.tile:hover {
  outline: 1px solid var(--accent);
}
.tile.warn .tile-value {
  color: var(--danger-text);
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
.charts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--font-m);
  margin-bottom: var(--font-m);
}
.charts .block {
  margin-bottom: 0;
  overflow: hidden;
}
.chart {
  height: 200px;
}
.chart-skeleton {
  display: block;
  height: 100%;
}
.two {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--font-m);
  align-items: start;
}
.two .block {
  margin-bottom: 0;
}
.small {
  font-size: var(--font-s);
  font-weight: normal;
}
.nowrap {
  white-space: nowrap;
}
.error-cell {
  display: block;
  margin-top: 2px;
}
@media (max-width: 68.75em) {
  .charts,
  .two {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
