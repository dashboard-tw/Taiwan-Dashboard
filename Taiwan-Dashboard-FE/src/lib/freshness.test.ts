import { describe, expect, it } from 'vitest'
import type { Source } from '../api/types'
import { freshness, sourceFreshness } from './freshness'

const NOW = Date.parse('2026-11-20T12:00:00+08:00')
const HOUR = 3600

function src(over: Partial<Source>): Source {
  return {
    dataset: 'x',
    name: 'x',
    agency: '交通部',
    license: '',
    url: '',
    status: 'ok',
    last_observed_at: null,
    update_freq_seconds: 600,
    last_ingested_at: '2026-11-20T11:55:00+08:00',
    stale_after_seconds: HOUR,
    ...over,
  }
}

describe('sourceFreshness', () => {
  it('ignores a single failed run while the last fetch is recent', () => {
    expect(sourceFreshness(src({ status: 'error' }), NOW)).toBe('ok')
  })
  it('is late past stale_after', () => {
    expect(
      sourceFreshness(src({ status: 'error', last_ingested_at: '2026-11-20T10:00:00+08:00' }), NOW),
    ).toBe('late')
  })
  it('is paused only after at least six hours without a fetch', () => {
    expect(
      sourceFreshness(src({ status: 'error', last_ingested_at: '2026-11-20T06:30:00+08:00' }), NOW),
    ).toBe('late')
    expect(
      sourceFreshness(src({ status: 'error', last_ingested_at: '2026-11-03T01:58:00+08:00' }), NOW),
    ).toBe('paused')
  })
  it('waits for a long stale_after on slow datasets', () => {
    const daily = { stale_after_seconds: 72 * HOUR, update_freq_seconds: 24 * HOUR }
    expect(
      sourceFreshness(
        src({ ...daily, status: 'error', last_ingested_at: '2026-11-19T21:42:00+08:00' }),
        NOW,
      ),
    ).toBe('ok')
    expect(
      sourceFreshness(
        src({ ...daily, status: 'error', last_ingested_at: '2026-11-16T00:00:00+08:00' }),
        NOW,
      ),
    ).toBe('paused')
  })
  it('marks old source data as late even when fetching works', () => {
    expect(sourceFreshness(src({ status: 'stale' }), NOW)).toBe('late')
  })
  it('falls back to the status when the API omits the fields', () => {
    expect(
      sourceFreshness(
        src({ status: 'error', last_ingested_at: undefined, stale_after_seconds: undefined }),
        NOW,
      ),
    ).toBe('late')
    expect(sourceFreshness(src({ status: 'pending', last_ingested_at: null }), NOW)).toBe('ok')
  })
})

describe('freshness', () => {
  const stopped = { status: 'error' as const, last_ingested_at: '2026-11-03T01:58:00+08:00' }
  it('pauses the card when its live source stops', () => {
    const f = freshness([src({ ...stopped, update_freq_seconds: 300 })], NOW)
    expect(f).toEqual({
      level: 'paused',
      lastIngestedAt: '2026-11-03T01:58:00+08:00',
      agencies: ['交通部'],
    })
  })
  it('only marks the card late when a slower master file stops', () => {
    const f = freshness(
      [
        src({ update_freq_seconds: 600 }),
        src({
          ...stopped,
          update_freq_seconds: 24 * HOUR,
          stale_after_seconds: 168 * HOUR,
          last_ingested_at: '2026-10-01T00:00:00+08:00',
        }),
      ],
      NOW,
    )
    expect(f.level).toBe('late')
  })
  it('only marks the card late when one of two same-pace sources stops', () => {
    const f = freshness([src({ ...stopped }), src({ update_freq_seconds: 600 })], NOW)
    expect(f.level).toBe('late')
  })
  it('is ok when every source is fresh', () => {
    expect(freshness([src({})], NOW).level).toBe('ok')
  })
})
