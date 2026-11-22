import { onScopeDispose, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { getComponentData, type DataQuery } from '../api/client'
import type { ComponentData } from '../api/types'
import { onScheduledRefresh } from './useRefreshClock'

export const RETRY_DELAYS_MS = [1_000, 2_000, 4_000, 8_000, 15_000, 30_000]

export function useComponentData(
  slug: MaybeRefOrGetter<string>,
  kind: MaybeRefOrGetter<'data' | 'history'> = 'data',
  query: MaybeRefOrGetter<DataQuery> = {},
  enabled: MaybeRefOrGetter<boolean> = true,
) {
  const result = ref<ComponentData | null>(null)
  const loading = ref(false)
  const failures = ref(0)
  let seq = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  function clearRetry() {
    if (timer !== undefined) clearTimeout(timer)
    timer = undefined
  }

  async function load(force = false) {
    if (!toValue(enabled)) return
    clearRetry()
    const mine = ++seq
    loading.value = true
    try {
      const r = await getComponentData(toValue(slug), toValue(kind), toValue(query), force)
      if (mine !== seq) return
      result.value = r
      failures.value = 0
    } catch {
      if (mine !== seq) return
      const delay = RETRY_DELAYS_MS[Math.min(failures.value, RETRY_DELAYS_MS.length - 1)]!
      failures.value++
      timer = setTimeout(() => {
        if (mine === seq) void load(true)
      }, delay)
    } finally {
      if (mine === seq) loading.value = false
    }
  }

  watch(
    () => [toValue(slug), toValue(kind), JSON.stringify(toValue(query)), toValue(enabled)],
    (now, before) => {
      if (before && (now[0] !== before[0] || now[2] !== before[2])) result.value = null
      failures.value = 0
      void load()
    },
    { immediate: true },
  )

  onScheduledRefresh(() => load(true))
  onScopeDispose(() => {
    seq++
    clearRetry()
  })

  return { result, loading, failures, reload: load }
}
