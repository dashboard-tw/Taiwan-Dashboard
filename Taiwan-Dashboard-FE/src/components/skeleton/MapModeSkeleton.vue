<script setup lang="ts">
import type { SlotHint } from '../../lib/layoutHints'
import TaiwanLogo from '../layout/TaiwanLogo.vue'
import ChartSkeleton from './ChartSkeleton.vue'

const props = defineProps<{ slots: SlotHint[] }>()
const openIndex = Math.max(
  0,
  props.slots.findIndex((s) => s.layers),
)
</script>

<template>
  <div class="map-skeleton" role="status" aria-label="載入中">
    <div class="side">
      <div v-for="(s, i) in slots" :key="i" class="item">
        <div class="head">
          <span class="skeleton title"></span>
          <span class="skeleton switch"></span>
        </div>
        <template v-if="i === openIndex">
          <span class="skeleton sub"></span>
          <div class="chart"><ChartSkeleton :kind="s.kind" /></div>
        </template>
      </div>
    </div>
    <div class="map">
      <span class="skeleton basemap"></span>
      <TaiwanLogo class="island" />
    </div>
  </div>
</template>

<style scoped>
.map-skeleton {
  display: grid;
  grid-template-columns: 360px minmax(0, 1fr);
  gap: var(--font-m);
  height: calc(100vh - var(--header-h) - 3.2rem - var(--font-m) * 3);
  min-height: 420px;
}
.side {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px var(--font-m);
  border-radius: var(--radius);
  background: var(--surface);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  width: 55%;
  height: 1rem;
}
.switch {
  width: 2.4rem;
  height: 1.3rem;
  border-radius: 1rem;
}
.sub {
  width: 35%;
  height: 0.65rem;
}
.chart {
  height: 260px;
  padding-top: 6px;
}
.map {
  position: relative;
  display: grid;
  place-items: center;
  border-radius: var(--radius);
  background: var(--surface);
}
.basemap {
  position: absolute;
  top: 10px;
  left: 10px;
  width: 7rem;
  height: 1.4rem;
}
.island {
  height: 80%;
  color: var(--surface-muted);
  animation: pulse 1.4s ease-in-out infinite;
}
@keyframes pulse {
  50% {
    opacity: 0.55;
  }
}
@media (prefers-reduced-motion: reduce) {
  .island {
    animation: none;
  }
}
@media (max-width: 759px) {
  .map-skeleton {
    display: block;
    position: fixed;
    inset: var(--header-h) 0 0 0;
    height: auto;
  }
  .map {
    position: absolute;
    inset: 0;
    border-radius: 0;
  }
  .side {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 1;
    height: 50%;
    overflow: hidden;
    padding: 28px 12px 12px;
    border-radius: 12px 12px 0 0;
    background: var(--bg);
  }
}
</style>
