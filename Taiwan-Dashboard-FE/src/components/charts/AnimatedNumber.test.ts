import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AnimatedNumber from './AnimatedNumber.vue'

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) =>
    setTimeout(() => cb(performance.now()), 16),
  )
  vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id))
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const num = (w: ReturnType<typeof mount>) => Number(w.text().replace(/,/g, ''))

describe('AnimatedNumber', () => {
  it('counts up from zero the first time', async () => {
    const w = mount(AnimatedNumber, { props: { value: 1869, decimals: 0 } })
    expect(num(w)).toBe(0)
    vi.advanceTimersByTime(200)
    await w.vm.$nextTick()
    expect(num(w)).toBeGreaterThan(0)
    expect(num(w)).toBeLessThan(1869)
    vi.advanceTimersByTime(800)
    await w.vm.$nextTick()
    expect(w.text()).toBe('1,869')
  })

  it('counts only the difference when the value updates', async () => {
    const w = mount(AnimatedNumber, { props: { value: 1869, decimals: 0 } })
    vi.advanceTimersByTime(1000)
    await w.setProps({ value: 1900 })
    vi.advanceTimersByTime(100)
    await w.vm.$nextTick()
    expect(num(w)).toBeGreaterThanOrEqual(1869)
    expect(num(w)).toBeLessThan(1900)
    vi.advanceTimersByTime(900)
    await w.vm.$nextTick()
    expect(w.text()).toBe('1,900')
  })

  it('keeps whole numbers whole while counting, and shows decimals for fractional values', async () => {
    const w = mount(AnimatedNumber, { props: { value: 1000, decimals: 1 } })
    vi.advanceTimersByTime(150)
    await w.vm.$nextTick()
    expect(w.text()).not.toContain('.')
    await w.setProps({ value: 12.5 })
    vi.advanceTimersByTime(1000)
    await w.vm.$nextTick()
    expect(w.text()).toBe('12.5')
  })

  it('shows a dash for a missing value', () => {
    const w = mount(AnimatedNumber, { props: { value: null, decimals: 0 } })
    expect(w.text()).toBe('—')
  })
})
