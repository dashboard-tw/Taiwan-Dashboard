import { onBeforeUnmount, readonly, ref } from 'vue'

export const REFRESH_MS = 20 * 60_000
export const MANUAL_COOLDOWN_MS = 30_000

let intervalMs = REFRESH_MS
const nextAt = ref(Date.now() + REFRESH_MS)
const now = ref(Date.now())
const refreshing = ref(false)
let lastManual = 0

const reloaders: (() => Promise<unknown>)[] = []
const active = ref(0)
let started = false

async function refresh() {
  if (refreshing.value) return
  refreshing.value = true
  try {
    for (const fn of [...reloaders]) {
      if (!reloaders.includes(fn)) continue
      try {
        await fn()
      } catch {}
    }
  } finally {
    refreshing.value = false
    restartCountdown()
  }
}

function tick() {
  now.value = Date.now()
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  if (now.value >= nextAt.value) void refresh()
}

function start() {
  if (started || typeof window === 'undefined') return
  started = true
  window.setInterval(tick, 1000)
  document.addEventListener('visibilitychange', tick)
  if (import.meta.env.DEV)
    (window as unknown as { __refreshNow: () => Promise<void> }).__refreshNow = refresh
}

export function restartCountdown(ms?: number) {
  if (ms) intervalMs = ms
  now.value = Date.now()
  nextAt.value = now.value + intervalMs
}

export function refreshNow(): boolean {
  const t = Date.now()
  if (refreshing.value || t - lastManual < MANUAL_COOLDOWN_MS) return false
  lastManual = t
  void refresh()
  return true
}

export function onScheduledRefresh(fn: () => Promise<unknown>) {
  start()
  reloaders.push(fn)
  active.value = reloaders.length
  onBeforeUnmount(() => {
    const i = reloaders.indexOf(fn)
    if (i >= 0) reloaders.splice(i, 1)
    active.value = reloaders.length
  })
}

export function useRefreshClock() {
  start()
  return {
    nextAt: readonly(nextAt),
    now: readonly(now),
    refreshing: readonly(refreshing),
    active: readonly(active),
  }
}
