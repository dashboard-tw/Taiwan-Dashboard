<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { getQuakes, type Quake } from '../api/client'
import { intensityLevel, QUAKE_KINDS, quakePlace, quakeTime } from '../lib/quake'
import { formatNumber } from '../lib/format'

const props = defineProps<{ scroll?: boolean }>()

const quakes = ref<Quake[] | null>(null)
const failed = ref(false)
const showAll = ref(false)
const FIRST = 10

onMounted(async () => {
  try {
    quakes.value = await getQuakes()
  } catch {
    failed.value = true
  }
})
</script>

<template>
  <section
    class="card reports"
    :class="{ scroll: props.scroll }"
    aria-labelledby="quake-reports-title"
  >
    <h2 id="quake-reports-title">最近的地震報告</h2>
    <p class="muted small">資料來源：交通部中央氣象署。點一份報告可以看各縣市震度與報告圖。</p>

    <p v-if="failed" class="muted">地震報告暫時無法取得。</p>
    <ul v-else-if="!quakes" class="list" aria-label="載入中">
      <li v-for="i in 5" :key="i"><span class="skeleton row-skeleton"></span></li>
    </ul>
    <p v-else-if="!quakes.length" class="muted">目前沒有地震報告。</p>
    <ul v-else class="list">
      <li v-for="q in showAll || props.scroll ? quakes : quakes.slice(0, FIRST)" :key="q.id">
        <RouterLink :to="`/earthquake/report/${q.id}`" class="row">
          <span class="when">{{ quakeTime(q.origin_time) }}</span>
          <span class="where">
            <strong>{{ quakePlace(q.location) }}</strong>
            <span class="kind">{{ QUAKE_KINDS[q.kind] ?? q.kind }}</span>
          </span>
          <span class="mag">
            規模 <strong>{{ q.magnitude === null ? '—' : formatNumber(q.magnitude, 1) }}</strong>
          </span>
          <span
            class="intensity"
            :style="
              intensityLevel(q.max_intensity)
                ? {
                    background: intensityLevel(q.max_intensity)!.color,
                    color: intensityLevel(q.max_intensity)!.ink,
                  }
                : undefined
            "
            >最大 {{ q.max_intensity ?? '—' }}</span
          >
          <span class="mi chevron" aria-hidden="true">chevron_right</span>
        </RouterLink>
      </li>
    </ul>
    <button
      v-if="!props.scroll && quakes && quakes.length > FIRST"
      type="button"
      class="more"
      @click="showAll = !showAll"
    >
      {{ showAll ? '收合' : `顯示全部 ${quakes.length} 份` }}
    </button>
  </section>
</template>

<style scoped>
.reports {
  margin-top: var(--font-m);
  padding: var(--font-m);
}
/* 固定高度，清單在框內捲動 */
.reports.scroll {
  display: flex;
  flex-direction: column;
  min-height: 0;
  margin-top: 0;
}
.reports.scroll .list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
h2 {
  font-size: var(--font-ms);
}
.small {
  margin-top: 4px;
  font-size: var(--font-s);
}
.list {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
}
.list li + li {
  border-top: 1px solid var(--border);
}
.row {
  display: grid;
  grid-template-columns: 9.5rem minmax(0, 1fr) auto auto 20px;
  gap: 12px;
  align-items: center;
  padding: 10px 6px;
  border-radius: var(--radius);
  color: var(--text-primary);
  text-decoration: none;
}
.row:hover {
  background: var(--surface-muted);
}
.when {
  color: var(--text-muted);
  font-size: 0.875rem;
  font-variant-numeric: tabular-nums;
}
.where {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 10px;
  min-width: 0;
}
.kind {
  color: var(--text-muted);
  font-size: var(--font-s);
}
.mag {
  color: var(--text-secondary);
  font-size: 0.875rem;
  white-space: nowrap;
}
.mag strong {
  color: var(--text-primary);
  font-size: 1rem;
}
.intensity {
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--surface-muted);
  font-size: 0.8rem;
  white-space: nowrap;
}
.chevron {
  color: var(--text-muted);
}
.row-skeleton {
  display: block;
  height: 1.5rem;
  margin: 10px 0;
}
.more {
  margin-top: 8px;
  padding: 4px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.875rem;
  cursor: pointer;
}
.more:hover {
  color: var(--text-primary);
}
@media (max-width: 40em) {
  .row {
    grid-template-columns: minmax(0, 1fr) auto 20px;
  }
  .when {
    grid-column: 1 / -1;
  }
  .mag {
    display: none;
  }
}
</style>
