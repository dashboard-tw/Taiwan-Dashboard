<script setup lang="ts">
import type { ApexOptions } from 'apexcharts'
import { computed, ref } from 'vue'
import 'apexcharts/bar'
import 'apexcharts/line'
import 'apexcharts/donut'
import 'apexcharts/area'
import 'apexcharts/treemap'
import 'apexcharts/features/legend'
import 'apexcharts/features/exports'
import VueApexCharts from 'vue3-apexcharts/core'
import { resolvedTheme } from '../../theme'

const props = defineProps<{
  type: 'bar' | 'donut' | 'line' | 'area' | 'treemap'
  series: ApexOptions['series']
  options: ApexOptions
  height?: number | string
  animated?: boolean
}>()

const reducedMotion =
  typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

const chart = ref<{
  chart?: { dataURI: () => Promise<{ imgURI: string } | { blob: Blob }> }
} | null>(null)

const themed = computed<ApexOptions>(() => {
  const dark = resolvedTheme.value === 'dark'
  return {
    ...props.options,
    chart: {
      fontFamily: 'inherit',
      background: 'transparent',
      foreColor: dark ? '#c3c3c3' : '#4a4d52',
      toolbar: { show: false },
      animations:
        props.animated && !reducedMotion
          ? {
              enabled: true,
              speed: 700,
              animateGradually: { enabled: false },
              dynamicAnimation: { enabled: true, speed: 600 },
            }
          : { enabled: false },
      zoom: { enabled: false },
      ...props.options.chart,
    },
    theme: { mode: dark ? 'dark' : 'light' },
    grid: { borderColor: dark ? '#3a3c3f' : '#e1e3e7', ...props.options.grid },
    tooltip: { theme: dark ? 'dark' : 'light', ...props.options.tooltip },
  }
})

async function pngDataUrl(): Promise<string | null> {
  const res = await chart.value?.chart?.dataURI()
  return res && 'imgURI' in res ? res.imgURI : null
}

defineExpose({ pngDataUrl })
</script>

<template>
  <VueApexCharts
    ref="chart"
    :type="type"
    :series="series"
    :options="themed"
    :height="height ?? '100%'"
    width="100%"
  />
</template>
