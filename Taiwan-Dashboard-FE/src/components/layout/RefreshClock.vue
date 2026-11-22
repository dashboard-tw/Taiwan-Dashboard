<script setup lang="ts">
import { computed, ref } from 'vue'
import { MANUAL_COOLDOWN_MS, refreshNow, useRefreshClock } from '../../composables/useRefreshClock'

defineProps<{ raised?: boolean }>()

const { nextAt, now, refreshing, active } = useRefreshClock()

const remaining = computed(() => {
  const s = Math.max(0, Math.ceil((nextAt.value - now.value) / 1000))
  return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
})

const coolUntil = ref(0)
const cooling = computed(() => now.value < coolUntil.value)

function click() {
  if (refreshNow()) coolUntil.value = Date.now() + MANUAL_COOLDOWN_MS
}
</script>

<template>
  <button
    v-if="active > 0"
    type="button"
    :class="['refresh-clock', { raised, cooling }]"
    :aria-disabled="refreshing || cooling"
    :title="cooling ? '剛更新過，請稍候再試' : '按一下馬上更新資料'"
    @click="click"
  >
    <span aria-live="polite">{{ refreshing ? '更新中…' : `下次更新：${remaining}` }}</span>
  </button>
</template>

<style scoped>
.refresh-clock {
  position: fixed;
  right: 14px;
  bottom: 10px;
  z-index: 40;
  margin: 0;
  padding: 2px 0;
  border: 0;
  background: none;
  color: var(--text-muted);
  font: inherit;
  font-size: var(--font-s);
  font-variant-numeric: tabular-nums;
  cursor: pointer;
  transition: color 0.15s;
}
.refresh-clock:hover,
.refresh-clock:focus-visible {
  color: var(--text-primary);
}
.refresh-clock.cooling {
  cursor: default;
}
.refresh-clock.cooling:hover {
  color: var(--text-muted);
}
.refresh-clock.raised {
  bottom: 80px;
}
</style>
