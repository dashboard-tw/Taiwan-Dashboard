import { computed, readonly, ref } from 'vue'
import { getComponentData } from '../api/client'
import type { Series } from '../api/types'

const CHECK_MS = 10 * 60_000

const warnings = ref<string[]>([])
const loaded = ref(false)
let started = false

async function check() {
  try {
    const r = await getComponentData('typhoon-warning-now', 'data', {}, true)
    warnings.value = ((r.data as Series[])[0]?.data ?? []).map((p) => String(p.x))
  } catch {
  } finally {
    loaded.value = true
  }
}

function start() {
  if (started || typeof window === 'undefined') return
  started = true
  void check()
  window.setInterval(() => {
    if (document.visibilityState === 'visible') void check()
  }, CHECK_MS)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') void check()
  })
}

export function useTyphoonAlert() {
  start()
  const active = computed(() => warnings.value.length > 0)
  const title = computed(() => {
    const names = [...new Set(warnings.value.map((w) => w.split('：')[0]))]
    return names.length === 1 ? `${names[0]}專區` : '颱風專區'
  })
  const detail = computed(() =>
    [...new Set(warnings.value.map((w) => w.split('：')[1] ?? w))].join('、'),
  )
  return { active, title, detail, loaded: readonly(loaded) }
}
