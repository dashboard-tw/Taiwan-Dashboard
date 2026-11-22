<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Series } from '../../api/types'
import { imageUrl } from '../../lib/weatherImage'

const props = defineProps<{ series: Series[]; label: string }>()

const allFrames = computed(() => {
  const s = props.series[0]
  if (!s?.name) return []
  return [...s.data]
    .map((p) => p.x)
    .sort()
    .map((x) => ({ time: x, url: imageUrl(s.name!, x) }))
})

const RETRY_MS = 60_000
const failed = ref(new Set<string>())
const frames = computed(() => allFrames.value.filter((f) => !failed.value.has(f.url)))
let retryTimer: number | undefined

function onError(url: string) {
  const atLatest = index.value >= frames.value.length - 1
  failed.value = new Set(failed.value).add(url)
  if (atLatest || !playing.value) index.value = Math.max(0, frames.value.length - 1)
  else index.value = Math.min(index.value, frames.value.length - 1)
  window.clearTimeout(retryTimer)
  retryTimer = window.setTimeout(() => (failed.value = new Set()), RETRY_MS)
}

const index = ref(0)
const playing = ref(false)
let timer: number | undefined

watch(
  () => frames.value.length,
  (n) => {
    if (!playing.value) index.value = Math.max(0, n - 1)
  },
  { immediate: true },
)

const current = computed(() => frames.value[index.value])
const timeLabel = computed(() =>
  current.value
    ? new Date(current.value.time).toLocaleString('zh-TW', {
        timeZone: 'Asia/Taipei',
        month: 'numeric',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })
    : '',
)

function preload() {
  for (const f of frames.value) {
    const img = new Image()
    img.src = f.url
  }
}

function toggle() {
  if (playing.value) {
    stop()
    return
  }
  preload()
  playing.value = true
  if (index.value >= frames.value.length - 1) index.value = 0
  timer = window.setInterval(() => {
    if (index.value >= frames.value.length - 1) {
      index.value = 0
    } else index.value++
  }, 400)
}

function stop() {
  playing.value = false
  window.clearInterval(timer)
}

onBeforeUnmount(() => {
  stop()
  window.clearTimeout(retryTimer)
})
</script>

<template>
  <div class="frames">
    <div class="view" role="img" :aria-label="current ? `${label}（${timeLabel}）` : label">
      <img
        v-if="current"
        :key="current.url"
        :src="current.url"
        alt=""
        decoding="async"
        draggable="false"
        @error="onError(current.url)"
      />
      <p v-else-if="allFrames.length" class="unavailable">影像暫時無法取得，稍後會自動再試</p>
    </div>
    <div v-if="frames.length > 1" class="controls">
      <button
        type="button"
        class="btn-plain"
        :aria-label="playing ? '暫停' : '播放最近的變化'"
        @click="toggle"
      >
        <span class="mi" aria-hidden="true">{{ playing ? 'pause' : 'play_arrow' }}</span>
      </button>
      <input
        v-model.number="index"
        type="range"
        min="0"
        :max="frames.length - 1"
        :aria-label="`${label}的時間`"
        :aria-valuetext="timeLabel"
        @input="stop"
      />
      <span class="time">{{ timeLabel }}</span>
    </div>
  </div>
</template>

<style scoped>
.frames {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 6px;
}
.view {
  position: relative;
  flex: 1;
  min-height: 0;
}
.view img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  user-select: none;
}
.unavailable {
  display: grid;
  place-items: center;
  height: 100%;
  margin: 0;
  color: var(--text-muted);
  font-size: var(--font-s);
}
.controls {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--font-s);
  color: var(--text-secondary);
}
.controls input {
  flex: 1;
}
.time {
  min-width: 7.5em;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>
