<script setup lang="ts">
import ChartSkeleton from './ChartSkeleton.vue'

withDefaults(defineProps<{ kind?: 'bar' | 'donut' | 'line' | 'map' }>(), { kind: 'bar' })
</script>

<template>
  <div class="info-skeleton" role="status" aria-label="載入中">
    <div class="left">
      <span class="skeleton title"></span>
      <span class="skeleton sub"></span>
      <span class="skeleton city"></span>
      <div class="chart"><ChartSkeleton :kind="kind" /></div>
    </div>
    <div class="right">
      <span class="skeleton heading"></span>
      <span
        v-for="w in [100, 96, 88, 60]"
        :key="w"
        class="skeleton line"
        :style="{ width: `${w}%` }"
      ></span>
      <span class="skeleton heading"></span>
      <span
        v-for="w in [100, 70]"
        :key="`b${w}`"
        class="skeleton line"
        :style="{ width: `${w}%` }"
      ></span>
      <span class="skeleton heading"></span>
      <div class="history"><ChartSkeleton kind="line" /></div>
      <div class="buttons">
        <span class="skeleton button"></span>
        <span class="skeleton button"></span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.info-skeleton {
  display: grid;
  gap: var(--font-ms);
  width: min(820px, 100%);
  padding: var(--font-m);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
}
@media (min-width: 820px) {
  .info-skeleton {
    grid-template-columns: 3fr 2fr;
    height: 440px;
  }
  .right {
    padding-left: var(--font-ms);
    border-left: 1px solid var(--border);
  }
}
.left,
.right {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-height: 0;
}
.title {
  width: 50%;
  height: 1.1rem;
}
.sub {
  width: 30%;
  height: 0.65rem;
}
.city {
  width: 2.2rem;
  height: 0.9rem;
}
.chart {
  flex: 1;
  min-height: 260px;
  padding-top: 8px;
}
.heading {
  width: 40%;
  height: 0.9rem;
  margin-top: 6px;
}
.line {
  height: 0.6rem;
}
.history {
  height: 120px;
}
.buttons {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: auto;
}
.button {
  width: 3.5rem;
  height: 1.4rem;
}
</style>
