<script setup lang="ts">
defineProps<{
  x: number
  y: number
  hasFilter: boolean
  hasMapLayer: boolean
  hasHistory: boolean
}>()
</script>

<template>
  <Teleport to="body">
    <div
      class="tag-tooltip"
      aria-hidden="true"
      :style="{ left: `${x - 40}px`, top: `${y - 110}px` }"
    >
      <p class="tip-title">組件資訊與功能</p>
      <p :class="{ off: !hasFilter }">
        <span class="mi" aria-hidden="true">{{ hasFilter ? 'check' : 'clear' }}</span
        >可用圖表篩選地圖
      </p>
      <p :class="{ off: !hasMapLayer }">
        <span class="mi" aria-hidden="true">{{ hasMapLayer ? 'check' : 'clear' }}</span
        >具備空間資料
      </p>
      <p :class="{ off: !hasHistory }">
        <span class="mi" aria-hidden="true">{{ hasHistory ? 'check' : 'clear' }}</span
        >具備歷史資料
      </p>
    </div>
  </Teleport>
</template>

<style scoped>
.tag-tooltip {
  position: fixed;
  z-index: 200;
  padding: 5px 10px 5px 5px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  box-shadow: 0 0 5px rgb(0 0 0 / 60%);
  pointer-events: none;
}
.tip-title {
  margin: 0;
  color: var(--text-muted);
  font-size: var(--font-s);
  font-weight: 400;
}
p {
  display: flex;
  align-items: center;
  margin: 0;
  color: var(--text-primary);
  font-size: var(--font-ms);
}
p .mi {
  color: var(--accent);
  font-size: var(--font-m);
}
</style>
