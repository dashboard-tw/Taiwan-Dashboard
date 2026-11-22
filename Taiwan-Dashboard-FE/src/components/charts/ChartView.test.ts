import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { ComponentInfo, DataMeta } from '../../api/types'
import ChartView from './ChartView.vue'

vi.mock('../../api/client', () => ({
  getCountiesGeo: () => new Promise(() => {}), // 地圖的縣市界線不用真的載入
}))

const component = {
  slug: 'x',
  name: '測試',
  topic: 'aqi',
  chart_types: [],
  decimals: 0,
  unit: '',
  colors: [],
  scopes: ['national'],
} as unknown as ComponentInfo

const byCounty = (v: number) => [
  { county_code: '63000', data: v },
  { county_code: '64000', data: v + 1 },
]
const twoD = (v: number) => [
  {
    data: [
      { x: '甲', y: v },
      { x: '乙', y: v + 1 },
    ],
  },
]
const time = (days: number) => [
  {
    name: '數值',
    data: Array.from({ length: days }, (_, i) => ({
      x: new Date(Date.UTC(2026, 10, 1 + i)).toISOString(),
      y: i,
    })),
  },
]

const cases: [string, string, (n: number) => unknown][] = [
  ['CountyMapChart', 'by_county', byCounty],
  ['BarChart', 'by_county', byCounty],
  ['BarChart', 'two_d', twoD],
  ['ColumnChart', 'two_d', twoD],
  ['DonutChart', 'two_d', twoD],
  ['TreemapChart', 'two_d', twoD],
  ['NumberChart', 'two_d', twoD],
  ['ListChart', 'two_d', twoD],
  ['TimelineSeparateChart', 'time', time],
  ['TimelineStackedChart', 'time', time],
]

describe('ChartView', () => {
  it.each(cases)('draws and updates %s with %s data without errors', async (type, shape, make) => {
    const errors: unknown[] = []
    const w = mount(ChartView, {
      props: {
        type,
        data: make(3),
        meta: { data_shape: shape, sources: [], observed_at: null } as unknown as DataMeta,
        component,
        counties: [],
      },
      global: {
        stubs: { ApexChart: true, CountyMap: true },
        config: {
          errorHandler: (e) => errors.push(e),
          warnHandler: (msg) => errors.push(msg),
        },
      },
    })
    await w.setProps({ data: make(4) })
    await w.setProps({ type: type === 'BarChart' ? 'ColumnChart' : 'BarChart' })
    expect(errors).toEqual([])
  })
})
