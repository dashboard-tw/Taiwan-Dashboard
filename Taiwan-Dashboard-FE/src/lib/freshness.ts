import type { Source } from '../api/types'

export const PAUSE_AFTER_MIN_MS = 6 * 60 * 60 * 1000

export type FreshnessLevel = 'ok' | 'late' | 'paused'

export interface Freshness {
  level: FreshnessLevel
  lastIngestedAt: string | null
  agencies: string[]
}

export function sourceFreshness(s: Source, now: number = Date.now()): FreshnessLevel {
  if (s.status === 'pending') return 'ok'
  const staleMs = (s.stale_after_seconds ?? 0) * 1000
  const ingested = s.last_ingested_at ? Date.parse(s.last_ingested_at) : NaN
  if (!staleMs || Number.isNaN(ingested)) {
    return s.status === 'stale' || s.status === 'error' ? 'late' : 'ok'
  }
  const age = now - ingested
  if (age > Math.max(staleMs, PAUSE_AFTER_MIN_MS)) return 'paused'
  if (age > staleMs) return 'late'
  if (s.status === 'stale') return 'late'
  return 'ok'
}

export function freshness(sources: Source[], now: number = Date.now()): Freshness {
  const levels = sources.map((s) => sourceFreshness(s, now))
  const freqs = sources.map((s) => s.update_freq_seconds).filter((f) => f > 0)
  const fastest = freqs.length ? Math.min(...freqs) : 0
  const drivers = sources
    .map((s, i) => ({ s, level: levels[i] }))
    .filter(({ s }) => !fastest || s.update_freq_seconds === fastest)
  const paused = drivers.every((d) => d.level === 'paused') ? drivers.map((d) => d.s) : []
  if (paused.length) {
    const times = paused
      .map((s) => s.last_ingested_at)
      .filter((t): t is string => !!t)
      .sort()
    return {
      level: 'paused',
      lastIngestedAt: times[0] ?? null,
      agencies: [...new Set(paused.map((s) => s.agency))],
    }
  }
  const late = levels.some((l) => l !== 'ok')
  return { level: late ? 'late' : 'ok', lastIngestedAt: null, agencies: [] }
}
