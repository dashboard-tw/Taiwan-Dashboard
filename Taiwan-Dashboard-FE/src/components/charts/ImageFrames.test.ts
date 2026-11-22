import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import ImageFrames from './ImageFrames.vue'

const series = [
  {
    name: 'radar',
    data: [
      '2026-11-20T19:00:00+08:00',
      '2026-11-20T19:10:00+08:00',
      '2026-11-20T19:20:00+08:00',
    ].map((x) => ({ x, y: 1200 })),
  },
]

describe('ImageFrames', () => {
  it('shows the newest frame', () => {
    const w = mount(ImageFrames, { props: { series, label: '雷達回波' } })
    expect(w.get('img').attributes('src')).toContain('20261120T1120Z.jpg')
  })

  it('falls back to the newest frame that loads, never a broken image', async () => {
    const w = mount(ImageFrames, { props: { series, label: '雷達回波' } })
    await w.get('img').trigger('error')
    expect(w.get('img').attributes('src')).toContain('20261120T1110Z.jpg')
    expect(w.get('img').attributes('alt')).toBe('')
  })

  it('explains instead of showing broken images when no frame loads', async () => {
    const w = mount(ImageFrames, { props: { series, label: '雷達回波' } })
    for (let i = 0; i < 3; i++) await w.get('img').trigger('error')
    expect(w.find('img').exists()).toBe(false)
    expect(w.text()).toContain('影像暫時無法取得')
  })
})
