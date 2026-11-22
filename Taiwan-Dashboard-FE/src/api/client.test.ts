import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, getHealth, getJson } from './client'

function mockFetch(status: number, body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('getJson', () => {
  it('returns the envelope on success', async () => {
    mockFetch(200, { data: { a: 1 }, meta: {} })
    await expect(getJson('/x')).resolves.toEqual({ data: { a: 1 }, meta: {} })
  })

  it('always asks the server whether the data changed', async () => {
    mockFetch(200, { data: 1, meta: {} })
    await getJson('/x')
    const init = vi.mocked(fetch).mock.calls[0]![1]
    expect(init?.cache).toBe('no-cache')
  })

  it('throws ApiError with the error code on failure', async () => {
    mockFetch(404, { error: { code: 'not_found', message: '找不到這個路徑' } })
    const err = await getJson('/x').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(404)
    expect((err as ApiError).code).toBe('not_found')
  })
})

describe('getHealth', () => {
  it('returns degraded status instead of throwing when the database is down', async () => {
    mockFetch(503, {
      data: { status: 'degraded', database: 'unavailable', version: 'dev' },
      meta: {},
    })
    const res = await getHealth()
    expect(res.data.database).toBe('unavailable')
  })
})

describe('remembered settings', () => {
  it('keeps dashboard and component settings for the next visit, and data only as a backup', async () => {
    const { clearCache, getComponentData, getDashboard, peekComponent, peekDashboard } =
      await import('./client')
    localStorage.clear()
    clearCache()
    mockFetch(200, { data: { slug: 'aqi', name: '空氣品質', components: [] }, meta: {} })
    await getDashboard('aqi')
    mockFetch(200, { data: [], meta: { data_shape: 'two_d', sources: [] } })
    await getComponentData('aqi-by-county')

    clearCache() // a new page load: only localStorage is left
    expect(peekDashboard('aqi')?.name).toBe('空氣品質')
    expect(peekComponent('aqi-by-county')).toBeNull()
    expect(Object.keys(localStorage).filter((k) => k.includes('/data'))).toEqual([
      'dashboardtw:data:/components/aqi-by-county/data',
    ])
  })

  it('shows the last data from an earlier visit when the server cannot be reached', async () => {
    const { clearCache, getComponentData } = await import('./client')
    localStorage.clear()
    clearCache()
    mockFetch(200, { data: [{ x: 1 }], meta: { data_shape: 'two_d', sources: [] } })
    await getComponentData('aqi-by-county')

    clearCache()
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    const r = await getComponentData('aqi-by-county')
    expect(r.data).toEqual([{ x: 1 }])
    expect(r.meta.stale).toBe(true)
  })

  it('still fails when nothing was ever stored', async () => {
    const { clearCache, getComponentData } = await import('./client')
    localStorage.clear()
    clearCache()
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    await expect(getComponentData('never-seen')).rejects.toThrow()
  })
})

describe('getCountiesGeo', () => {
  const geo = { type: 'FeatureCollection', features: [] }

  async function freshClient() {
    vi.resetModules()
    return import('./client')
  }

  it('retries a failed download before giving up', async () => {
    vi.useFakeTimers()
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('network'))
      .mockResolvedValueOnce(new Response('', { status: 502 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(geo), { status: 200 }))
    vi.stubGlobal('fetch', fetch)
    const { getCountiesGeo } = await freshClient()
    const p = getCountiesGeo()
    await vi.runAllTimersAsync()
    await expect(p).resolves.toEqual(geo)
    expect(fetch).toHaveBeenCalledTimes(3)
    vi.useRealTimers()
  })

  it('starts over on the next call after giving up', async () => {
    vi.useFakeTimers()
    const fetch = vi.fn().mockRejectedValue(new TypeError('network'))
    vi.stubGlobal('fetch', fetch)
    const { getCountiesGeo } = await freshClient()
    const p = getCountiesGeo()
    p.catch(() => {})
    await vi.runAllTimersAsync()
    await expect(p).rejects.toThrow('network')
    expect(fetch).toHaveBeenCalledTimes(3)

    fetch.mockResolvedValue(new Response(JSON.stringify(geo), { status: 200 }))
    await expect(getCountiesGeo()).resolves.toEqual(geo)
    expect(fetch).toHaveBeenCalledTimes(4)
    vi.useRealTimers()
  })
})
