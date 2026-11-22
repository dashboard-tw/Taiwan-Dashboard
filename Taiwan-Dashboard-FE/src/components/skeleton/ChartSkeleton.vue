<script setup lang="ts">
import TaiwanLogo from '../layout/TaiwanLogo.vue'

defineProps<{ kind: 'bar' | 'donut' | 'line' | 'map' }>()

const BARS = [92, 78, 70, 64, 55, 47, 40, 31]
</script>

<template>
  <div class="chart-skeleton" aria-hidden="true">
    <div v-if="kind === 'bar'" class="bars">
      <div v-for="(w, i) in BARS" :key="i" class="bar-row">
        <span class="skeleton label"></span>
        <span class="skeleton bar" :style="{ width: `${w}%` }"></span>
      </div>
    </div>

    <div v-else-if="kind === 'donut'" class="donut">
      <div class="skeleton ring"><span class="hole"></span></div>
      <div class="legend">
        <span v-for="i in 5" :key="i" class="skeleton chip"></span>
      </div>
    </div>

    <div v-else-if="kind === 'line'" class="line">
      <svg viewBox="0 0 300 120" preserveAspectRatio="none">
        <line v-for="y in [20, 50, 80, 110]" :key="y" x1="0" x2="300" :y1="y" :y2="y" />
        <path d="M0 90 C40 85 60 40 100 50 S160 95 200 70 S260 30 300 40" />
      </svg>
    </div>

    <div v-else class="map">
      <div class="map-total">
        <span class="skeleton small"></span>
        <span class="skeleton value"></span>
      </div>
      <TaiwanLogo class="island" />
    </div>
  </div>
</template>

<style scoped>
.chart-skeleton {
  height: 100%;
  min-height: 140px;
}
.bars {
  display: flex;
  flex-direction: column;
  justify-content: space-evenly;
  height: 100%;
  padding: 4px 0;
}
.bar-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.label {
  width: 3.5rem;
  height: 0.6rem;
  flex-shrink: 0;
}
.bar {
  height: 0.85rem;
}
.donut {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  height: 100%;
}
.ring {
  width: min(150px, 55%);
  aspect-ratio: 1;
  border-radius: 50%;
  display: grid;
  place-items: center;
}
.hole {
  width: 62%;
  aspect-ratio: 1;
  border-radius: 50%;
  background: var(--surface);
  z-index: 1;
}
.legend {
  display: flex;
  gap: 8px;
}
.chip {
  width: 2.5rem;
  height: 0.6rem;
}
.line {
  height: 100%;
}
.line svg {
  width: 100%;
  height: 100%;
}
.line line {
  stroke: var(--surface-muted);
  stroke-width: 1;
}
.line path {
  fill: none;
  stroke: var(--surface-muted);
  stroke-width: 4;
  stroke-linecap: round;
  animation: pulse 1.4s ease-in-out infinite;
}
.map {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}
.map-total {
  position: absolute;
  top: 0;
  left: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.small {
  width: 2rem;
  height: 0.6rem;
}
.value {
  width: 3.5rem;
  height: 1.2rem;
}
.island {
  height: 85%;
  color: var(--surface-muted);
  animation: pulse 1.4s ease-in-out infinite;
}
@keyframes pulse {
  50% {
    opacity: 0.55;
  }
}
@media (prefers-reduced-motion: reduce) {
  .line path,
  .island {
    animation: none;
  }
}
</style>
