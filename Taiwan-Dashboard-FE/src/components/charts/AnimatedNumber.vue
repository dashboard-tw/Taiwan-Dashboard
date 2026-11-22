<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { formatNumber } from '../../lib/format'

const props = defineProps<{
  value: number | null
  decimals: number
}>()

const DURATION_MS = 700

const reducedMotion =
  typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

const shown = ref<number | null>(null)
let frame = 0

function countTo(target: number | null) {
  cancelAnimationFrame(frame)
  if (target === null || reducedMotion || typeof requestAnimationFrame === 'undefined') {
    shown.value = target
    return
  }
  const start = shown.value ?? 0
  shown.value = start // 第一次從 0 開始，不先閃一下「—」
  if (start === target) return
  const t0 = performance.now()
  const step = (t: number) => {
    const p = Math.min((t - t0) / DURATION_MS, 1)
    const eased = 1 - (1 - p) ** 3 // 先快後慢
    shown.value = p < 1 ? start + (target - start) * eased : target
    if (p < 1) frame = requestAnimationFrame(step)
  }
  frame = requestAnimationFrame(step)
}

watch(() => props.value, countTo, { immediate: true })
onBeforeUnmount(() => cancelAnimationFrame(frame))

function display(v: number | null): string {
  if (v === null || props.value === null) return formatNumber(null)
  const decimals = Number.isInteger(props.value) ? 0 : props.decimals
  return formatNumber(decimals === 0 ? Math.round(v) : v, decimals)
}
</script>

<template>
  <span :aria-label="formatNumber(value, Number.isInteger(value) ? 0 : decimals)">{{
    display(shown)
  }}</span>
</template>
