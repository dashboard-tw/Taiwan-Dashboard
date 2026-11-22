import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getCounties } from '../api/client'
import type { County } from '../api/types'

export const useCatalog = defineStore('catalog', () => {
  const counties = ref<County[]>([])
  const error = ref<string | null>(null)
  let loading: Promise<void> | null = null

  const byCode = computed(() => new Map(counties.value.map((c) => [c.code, c])))

  function load(): Promise<void> {
    if (counties.value.length) return Promise.resolve()
    loading ??= getCounties()
      .then((list) => {
        counties.value = list
        error.value = null
      })
      .catch((e: unknown) => {
        error.value = e instanceof Error ? e.message : String(e)
      })
      .finally(() => (loading = null))
    return loading
  }

  function countyName(code: string | null | undefined): string {
    return (code && byCode.value.get(code)?.name) || ''
  }

  return { counties, error, byCode, load, countyName }
})
