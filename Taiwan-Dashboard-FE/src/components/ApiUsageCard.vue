<script setup lang="ts">
import type { ApexOptions } from 'apexcharts'
import { RouterLink } from 'vue-router'
import ApexChart from './charts/ApexChart.vue'

export interface UsageMeter {
  label: string
  used: string | number
  limit: string | number
  unit: string
  pct: number
}

defineProps<{
  partner: boolean
  meters: UsageMeter[]
  mcpCalls: number
  aiTools: number | null
  series: ApexOptions['series']
  options: ApexOptions
}>()
</script>

<template>
  <section class="card block">
    <h2>
      用量
      <RouterLink v-if="partner" to="/me/showcase" class="badge">合作額度</RouterLink>
    </h2>
    <p class="muted small">所有憑證共用，每天午夜歸零。</p>
    <div class="meters">
      <div v-for="m in meters" :key="m.label" class="meter">
        <div class="meter-head">
          <span>今日{{ m.label }}</span>
          <span
            ><strong>{{ m.used }}</strong> / {{ m.limit }} {{ m.unit }}</span
          >
        </div>
        <div
          class="bar"
          role="progressbar"
          :aria-label="`今日${m.label}`"
          :aria-valuenow="m.pct"
          aria-valuemin="0"
          aria-valuemax="100"
        >
          <span :style="{ width: `${m.pct}%` }" :class="{ high: m.pct >= 80 }"></span>
        </div>
      </div>
    </div>
    <p class="mcp-today">
      <span
        >今日 MCP 呼叫次數<RouterLink v-if="aiTools !== null" to="/me/devices" class="ai-tools"
          >目前有 {{ aiTools }} 個已授權 AI 工具<span class="mi" aria-hidden="true"
            >chevron_right</span
          ></RouterLink
        ></span
      ><strong>{{ mcpCalls }}</strong>
    </p>
    <h3>近 30 天</h3>
    <div class="chart">
      <ApexChart type="bar" :series="series" :options="options" height="100%" />
    </div>
  </section>
</template>

<style scoped>
h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--font-ms);
}
h3 {
  margin: 0.25rem 0 0;
  color: var(--text-secondary);
  font-size: 0.875rem;
  font-weight: normal;
}
.small {
  font-size: var(--font-s);
}
.badge {
  padding: 0 8px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: var(--font-s);
  font-weight: 400;
  text-decoration: none;
}
.meters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
  gap: 0.75rem 1.25rem;
}
.meter-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
  font-size: 0.875rem;
}
.meter-head strong {
  font-size: var(--font-ms);
}
.bar {
  height: 6px;
  overflow: hidden;
  border-radius: 3px;
  background: var(--border);
}
.bar span {
  display: block;
  height: 100%;
  border-radius: 3px;
  background: var(--accent);
  transition: width 0.4s ease;
}
.bar span.high {
  background: var(--danger-text);
}
.mcp-today {
  display: flex;
  justify-content: space-between;
  gap: 0.75rem;
  margin: 0.75rem 0 0;
  padding-top: 0.5rem;
  border-top: 1px solid var(--border);
  font-size: 0.875rem;
}
.ai-tools {
  display: inline-flex;
  align-items: center;
  margin-left: 0.5rem;
  color: var(--accent);
  font-size: var(--font-s);
  text-decoration: none;
}
.ai-tools .mi {
  font-size: 1.1em;
}
.ai-tools:hover {
  text-decoration: underline;
}
.chart {
  height: 220px;
}
</style>
