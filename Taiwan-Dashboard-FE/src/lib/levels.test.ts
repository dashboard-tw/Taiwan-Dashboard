import { describe, expect, it } from 'vitest'
import {
  AQI_LEVELS,
  OUTAGE_COUNT_LEVELS,
  QUAKE_COUNT_LEVELS,
  QUAKE_MAX_INTENSITY_LEVELS,
  RAIN_1H_LEVELS,
  RAIN_24H_LEVELS,
  RAIN_CHANCE_LEVELS,
  CLOSURE_LEVELS,
  DEBRIS_LEVELS,
  SPEED_LEVELS,
  TEMP_LEVELS,
  WIND_LEVELS,
  QUAKE_MAGNITUDE_LEVELS,
  OUTAGE_HOUSEHOLD_LEVELS,
  PM25_LEVELS,
  RIVER_MARGIN_LEVELS,
  UV_LEVELS,
  aqiLevel,
  levelIn,
  levelsFor,
} from './levels'

describe('AQI levels follow the Ministry of Environment', () => {
  it.each([
    [0, '良好'],
    [50, '良好'],
    [51, '普通'],
    [100.4, '普通'],
    [101, '對敏感族群不健康'],
    [151, '對所有族群不健康'],
    [201, '非常不健康'],
    [301, '危害'],
    [520, '危害'],
  ])('%s is %s', (aqi, label) => {
    expect(aqiLevel(aqi).label).toBe(label)
    expect(levelIn(AQI_LEVELS, aqi).label).toBe(label)
  })
})

describe('levelsFor', () => {
  it('picks the scale for each component', () => {
    expect(levelsFor('aqi-by-county', 'aqi', '')).toBe(AQI_LEVELS)
    expect(levelsFor('pm25-by-county', 'aqi', 'μg/m³')).toBe(PM25_LEVELS)
    expect(levelsFor('reservoir-by-county', 'reservoir', '%')?.[0]?.label).toBe('30% 以下')
    expect(levelsFor('power-gen-mix', 'power', 'MW')).toBeNull()
    expect(levelsFor('river-alert-margin', 'reservoir', '公尺')).toBe(RIVER_MARGIN_LEVELS)
    expect(levelsFor('water-outage-by-county', 'outage', '件')).toBe(OUTAGE_COUNT_LEVELS)
    expect(levelsFor('power-outage-by-county', 'outage', '戶')).toBe(OUTAGE_HOUSEHOLD_LEVELS)
    expect(levelsFor('water-outage-upcoming', 'outage', '件')).toBeNull()
    expect(levelsFor('quake-recent', 'earthquake', '規模')).toBe(QUAKE_MAGNITUDE_LEVELS)
    expect(levelsFor('quake-felt-by-county', 'earthquake', '次')).toBe(QUAKE_COUNT_LEVELS)
    expect(levelsFor('quake-max-intensity-by-county', 'earthquake', '級')).toBe(
      QUAKE_MAX_INTENSITY_LEVELS,
    )
    expect(levelIn(QUAKE_MAX_INTENSITY_LEVELS, 0).label).toBe('沒有')
    expect(levelIn(QUAKE_MAX_INTENSITY_LEVELS, 5).label).toBe('5 級')
    expect(levelIn(QUAKE_MAX_INTENSITY_LEVELS, 7).label).toBe('6 級以上')
  })
})

describe('UV_LEVELS', () => {
  it('is used for UVI components and rounds like the official scale', () => {
    expect(levelsFor('uv-by-county', 'weather', 'UVI')).toBe(UV_LEVELS)
    expect(levelIn(UV_LEVELS, 2.4).label).toMatch('低量')
    expect(levelIn(UV_LEVELS, 2.5).label).toMatch('中量')
    expect(levelIn(UV_LEVELS, 7.4).label).toMatch('高量')
    expect(levelIn(UV_LEVELS, 11).label).toMatch('危險')
  })
})

describe('RIVER_MARGIN_LEVELS', () => {
  it.each([
    [-0.5, '已達警戒'],
    [0, '1 公尺內'],
    [0.99, '1 公尺內'],
    [1, '1–3 公尺'],
    [12, '3 公尺以上'],
  ])('%s m below the alert level is %s', (m, label) => {
    expect(levelIn(RIVER_MARGIN_LEVELS, m).label).toBe(label)
  })
})

describe('outage levels', () => {
  it.each([
    [0, '沒有'],
    [1, '1 件'],
    [3, '2–3 件'],
    [6, '6 件以上'],
  ])('%s cases is %s', (n, label) => {
    expect(levelIn(OUTAGE_COUNT_LEVELS, n).label).toBe(label)
  })
  it('steps households at 100, 500 and 2,000', () => {
    expect(levelIn(OUTAGE_HOUSEHOLD_LEVELS, 99).label).toBe('1–99 戶')
    expect(levelIn(OUTAGE_HOUSEHOLD_LEVELS, 1999).label).toBe('500–1,999 戶')
    expect(levelIn(OUTAGE_HOUSEHOLD_LEVELS, 2000).label).toBe('2,000 戶以上')
  })
})

describe('earthquake levels', () => {
  it('steps magnitude at whole numbers', () => {
    expect(levelIn(QUAKE_MAGNITUDE_LEVELS, 3.9).label).toBe('規模 4 以下')
    expect(levelIn(QUAKE_MAGNITUDE_LEVELS, 4.0).label).toBe('4–5')
    expect(levelIn(QUAKE_MAGNITUDE_LEVELS, 7.2).label).toBe('7 以上')
  })
  it('counts felt earthquakes', () => {
    expect(levelIn(QUAKE_COUNT_LEVELS, 0).label).toBe('沒有')
    expect(levelIn(QUAKE_COUNT_LEVELS, 5).label).toBe('4–6 次')
  })
})

describe('rain levels', () => {
  it('follow the CWA 24-hour thresholds', () => {
    expect(levelIn(RAIN_24H_LEVELS, 79.9).label).toBe('未達大雨')
    expect(levelIn(RAIN_24H_LEVELS, 80).label).toBe('大雨（80）')
    expect(levelIn(RAIN_24H_LEVELS, 350).label).toBe('大豪雨（350）')
    expect(levelIn(RAIN_24H_LEVELS, 612).label).toBe('超大豪雨（500）')
  })
  it('mark 40 mm in an hour as heavy rain', () => {
    expect(levelIn(RAIN_1H_LEVELS, 39.5).label).toBe('20–40')
    expect(levelIn(RAIN_1H_LEVELS, 40).label).toBe('大雨（40）')
  })
  it('apply to the rain components only', () => {
    expect(levelsFor('rain-by-county', 'weather', '毫米')).toBe(RAIN_24H_LEVELS)
    expect(levelsFor('rain-1hr-top', 'weather', '毫米')).toBe(RAIN_1H_LEVELS)
    expect(levelsFor('typhoon-track', 'weather', '公尺/秒')).toBeNull()
  })
})

describe('weather levels', () => {
  it('picks temperature, wind and rain-chance scales', () => {
    expect(levelsFor('temp-by-county', 'weather', '°C')).toBe(TEMP_LEVELS)
    expect(levelsFor('county-forecast', 'weather', '°C')).toBe(TEMP_LEVELS)
    expect(levelsFor('wind-top', 'weather', '公尺／秒')).toBe(WIND_LEVELS)
    expect(levelsFor('forecast-rain-chance', 'weather', '%')).toBe(RAIN_CHANCE_LEVELS)
  })
  it.each([
    [-3, '10 以下'],
    [19.9, '15–20'],
    [20, '20–25'],
    [35, '35 以上'],
  ])('%s °C is %s', (c, label) => {
    expect(levelIn(TEMP_LEVELS, c).label).toBe(label)
  })
  it('uses the Beaufort scale for wind', () => {
    expect(levelIn(WIND_LEVELS, 10.7).label).toBe('4–5 級')
    expect(levelIn(WIND_LEVELS, 10.8).label).toBe('6–7 級（強風）')
  })
})

describe('alert and traffic levels', () => {
  it('picks the closure, debris and speed scales', () => {
    expect(levelsFor('work-closure-by-county', 'warning', '')).toBe(CLOSURE_LEVELS)
    expect(levelsFor('work-closure-tomorrow', 'warning', '')).toBe(CLOSURE_LEVELS)
    expect(levelsFor('debris-flow-alert', 'warning', '處')).toBe(DEBRIS_LEVELS)
    expect(levelsFor('freeway-speed-by-county', 'traffic', '公里／小時')).toBe(SPEED_LEVELS)
  })
  it('labels closures with words on the map', () => {
    expect(levelIn(CLOSURE_LEVELS, 2).short).toBe('停')
    expect(levelIn(CLOSURE_LEVELS, 0).short).toBe('照常')
  })
  it('follows the Freeway Bureau speed bands', () => {
    expect(levelIn(SPEED_LEVELS, 79).label).toMatch('車多')
    expect(levelIn(SPEED_LEVELS, 80).label).toMatch('順暢')
    expect(levelIn(SPEED_LEVELS, 19).label).toMatch('壅塞')
  })
})

describe('facility levels', () => {
  it('uses count breaks per component and the shelter scale', () => {
    expect(levelIn(levelsFor('clinic-by-county', 'resources', '家')!, 4056).label).toBe(
      '2500 家以上',
    )
    expect(levelIn(levelsFor('hospital-by-county', 'resources', '家')!, 9).label).toBe('未滿 10 家')
    expect(levelIn(levelsFor('shelter-open', 'warning', '人')!, 0).label).toBe('沒有')
  })

  it('labels rates up to the next break', () => {
    const rate = levelsFor('medical-per-10k', 'resources', '家')!
    expect(levelIn(rate, 9.5).label).toBe('8–10 家')
    expect(levelIn(rate, 10).label).toBe('10–13 家')
    expect(levelIn(levelsFor('hotel-by-county', 'tourism', '家')!, 499).label).toBe('200–499 家')
  })
})
