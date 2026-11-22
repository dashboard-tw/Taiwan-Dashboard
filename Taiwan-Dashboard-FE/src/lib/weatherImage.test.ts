import { describe, expect, it } from 'vitest'
import { imageFileName, imageUrl } from './weatherImage'

describe('weather image names', () => {
  it('match the worker, in UTC', () => {
    expect(imageFileName('2026-10-22T17:30:00+08:00')).toBe('20261022T0930Z.jpg')
    expect(imageFileName('2026-10-23T00:10:00+08:00')).toBe('20261022T1610Z.jpg')
  })
  it('build the served URL', () => {
    expect(imageUrl('radar', '2026-10-22T17:30:00+08:00')).toBe(
      '/api/v1/images/radar/20261022T0930Z.jpg',
    )
  })
})
