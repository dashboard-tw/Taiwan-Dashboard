export interface Level {
  min: number
  label: string
  color: string
  ink: string
  short?: string
}

export const AQI_LEVELS: Level[] = [
  { min: 0, label: '良好', color: '#009865', ink: '#ffffff' },
  { min: 51, label: '普通', color: '#fffb26', ink: '#1c1917' },
  { min: 101, label: '對敏感族群不健康', color: '#ff9835', ink: '#1c1917' },
  { min: 151, label: '對所有族群不健康', color: '#ca0034', ink: '#ffffff' },
  { min: 201, label: '非常不健康', color: '#670099', ink: '#ffffff' },
  { min: 301, label: '危害', color: '#7e0123', ink: '#ffffff' },
]

export function aqiLevel(aqi: number): Level {
  let level = AQI_LEVELS[0]!
  for (const l of AQI_LEVELS) if (Math.round(aqi) >= l.min) level = l
  return level
}

export const WATER_LEVELS: Level[] = [
  { min: 0, label: '30% 以下', color: 'var(--water-1)', ink: 'var(--text-primary)' },
  { min: 30, label: '30–50%', color: 'var(--water-2)', ink: 'var(--text-primary)' },
  { min: 50, label: '50–70%', color: 'var(--water-3)', ink: '#ffffff' },
  { min: 70, label: '70–90%', color: 'var(--water-4)', ink: '#ffffff' },
  { min: 90, label: '90% 以上', color: 'var(--water-5)', ink: '#ffffff' },
]

export function waterLevel(pct: number): Level {
  let level = WATER_LEVELS[0]!
  for (const l of WATER_LEVELS) if (pct >= l.min) level = l
  return level
}

export const PM25_LEVELS: Level[] = [
  { min: 0, label: '10 以下', color: '#ede9fe', ink: '#1c1917' },
  { min: 10, label: '10–20', color: '#c4b5fd', ink: '#1c1917' },
  { min: 20, label: '20–30', color: '#8b5cf6', ink: '#ffffff' },
  { min: 30, label: '30–50', color: '#6d28d9', ink: '#ffffff' },
  { min: 50, label: '50 以上', color: '#3b0764', ink: '#ffffff' },
]

export const UV_LEVELS: Level[] = [
  { min: 0, label: '低量（0–2）', color: '#289500', ink: '#ffffff' },
  { min: 3, label: '中量（3–5）', color: '#f7e400', ink: '#1c1917' },
  { min: 6, label: '高量（6–7）', color: '#f85900', ink: '#ffffff' },
  { min: 8, label: '過量（8–10）', color: '#d8001d', ink: '#ffffff' },
  { min: 11, label: '危險（11 以上）', color: '#6b49c8', ink: '#ffffff' },
]

export const RIVER_ALERT_LEVELS: Level[] = [
  { min: 0, label: '未達警戒', color: '#289500', ink: '#ffffff' },
  { min: 1, label: '三級警戒', color: '#f7e400', ink: '#1c1917' },
  { min: 2, label: '二級警戒', color: '#f85900', ink: '#ffffff' },
  { min: 3, label: '一級警戒', color: '#d8001d', ink: '#ffffff' },
]

export const RIVER_MARGIN_LEVELS: Level[] = [
  { min: -Infinity, label: '已達警戒', color: '#d8001d', ink: '#ffffff' },
  { min: 0, label: '1 公尺內', color: '#f85900', ink: '#ffffff' },
  { min: 1, label: '1–3 公尺', color: '#f7e400', ink: '#1c1917' },
  { min: 3, label: '3 公尺以上', color: '#289500', ink: '#ffffff' },
]

const OUTAGE_RAMP = ['#f0a070', '#e07840', '#c85424', '#943a14'] as const

export const OUTAGE_COUNT_LEVELS: Level[] = [
  { min: 0, label: '沒有', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 1, label: '1 件', color: OUTAGE_RAMP[0], ink: '#1c1917' },
  { min: 2, label: '2–3 件', color: OUTAGE_RAMP[1], ink: '#1c1917' },
  { min: 4, label: '4–5 件', color: OUTAGE_RAMP[2], ink: '#ffffff' },
  { min: 6, label: '6 件以上', color: OUTAGE_RAMP[3], ink: '#ffffff' },
]

export const OUTAGE_HOUSEHOLD_LEVELS: Level[] = [
  { min: 0, label: '沒有', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 1, label: '1–99 戶', color: OUTAGE_RAMP[0], ink: '#1c1917' },
  { min: 100, label: '100–499 戶', color: OUTAGE_RAMP[1], ink: '#1c1917' },
  { min: 500, label: '500–1,999 戶', color: OUTAGE_RAMP[2], ink: '#ffffff' },
  { min: 2000, label: '2,000 戶以上', color: OUTAGE_RAMP[3], ink: '#ffffff' },
]

const QUAKE_RAMP = ['#ee9298', '#e06a74', '#cd4654', '#b0303f', '#922232'] as const

export const QUAKE_INTENSITY_LEVELS: Level[] = [
  { min: 1, label: '1級', color: QUAKE_RAMP[0], ink: '#1c1917' },
  { min: 2, label: '2級', color: QUAKE_RAMP[1], ink: '#1c1917' },
  { min: 3, label: '3級', color: QUAKE_RAMP[2], ink: '#ffffff' },
  { min: 4, label: '4級', color: QUAKE_RAMP[3], ink: '#ffffff' },
  ...['5弱', '5強', '6弱', '6強', '7級'].map((label, i) => ({
    min: 5 + i,
    label,
    color: QUAKE_RAMP[4],
    ink: '#ffffff',
  })),
]

export const QUAKE_MAGNITUDE_LEVELS: Level[] = [
  { min: 0, label: '規模 4 以下', color: QUAKE_RAMP[0], ink: '#1c1917' },
  { min: 4, label: '4–5', color: QUAKE_RAMP[1], ink: '#1c1917' },
  { min: 5, label: '5–6', color: QUAKE_RAMP[2], ink: '#ffffff' },
  { min: 6, label: '6–7', color: QUAKE_RAMP[3], ink: '#ffffff' },
  { min: 7, label: '7 以上', color: QUAKE_RAMP[4], ink: '#ffffff' },
]

export const QUAKE_MAX_INTENSITY_LEVELS: Level[] = [
  { min: 0, label: '沒有', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 1, label: '1–2 級', color: QUAKE_RAMP[0], ink: '#1c1917' },
  { min: 3, label: '3 級', color: QUAKE_RAMP[1], ink: '#1c1917' },
  { min: 4, label: '4 級', color: QUAKE_RAMP[2], ink: '#ffffff' },
  { min: 5, label: '5 級', color: QUAKE_RAMP[3], ink: '#ffffff' },
  { min: 6, label: '6 級以上', color: QUAKE_RAMP[4], ink: '#ffffff' },
]

export const QUAKE_COUNT_LEVELS: Level[] = [
  { min: 0, label: '沒有', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 1, label: '1 次', color: QUAKE_RAMP[0], ink: '#1c1917' },
  { min: 2, label: '2–3 次', color: QUAKE_RAMP[1], ink: '#1c1917' },
  { min: 4, label: '4–6 次', color: QUAKE_RAMP[2], ink: '#ffffff' },
  { min: 7, label: '7 次以上', color: QUAKE_RAMP[3], ink: '#ffffff' },
]

const RAIN_RAMP = ['#55bfb3', '#37a69a', '#0d8d82', '#00756b'] as const

export const RAIN_24H_LEVELS: Level[] = [
  { min: 0, label: '未達大雨', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 80, label: '大雨（80）', color: RAIN_RAMP[0], ink: '#1c1917' },
  { min: 200, label: '豪雨（200）', color: RAIN_RAMP[1], ink: '#1c1917' },
  { min: 350, label: '大豪雨（350）', color: RAIN_RAMP[2], ink: '#ffffff' },
  { min: 500, label: '超大豪雨（500）', color: RAIN_RAMP[3], ink: '#ffffff' },
]

export const RAIN_1H_LEVELS: Level[] = [
  { min: 0, label: '10 以下', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 10, label: '10–20', color: RAIN_RAMP[0], ink: '#1c1917' },
  { min: 20, label: '20–40', color: RAIN_RAMP[1], ink: '#1c1917' },
  { min: 40, label: '大雨（40）', color: RAIN_RAMP[2], ink: '#ffffff' },
  { min: 80, label: '80 以上', color: RAIN_RAMP[3], ink: '#ffffff' },
]

const WARNING_RAMP = ['#c4a8ef', '#a27fe2', '#8158cf', '#6440b0'] as const

export const WEATHER_WARNING_LEVELS: Level[] = [
  { min: 0, label: '沒有', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 1, label: '1 項', color: WARNING_RAMP[0], ink: '#1c1917' },
  { min: 2, label: '2 項', color: WARNING_RAMP[1], ink: '#1c1917' },
  { min: 3, label: '3 項', color: WARNING_RAMP[2], ink: '#ffffff' },
  { min: 4, label: '4 項以上', color: WARNING_RAMP[3], ink: '#ffffff' },
]

const TEMP_COLD = ['#2f5f9e', '#4f86c6', '#8fb5dc'] as const
const TEMP_HOT = ['#eda36a', '#dc6a3c', '#b03a22'] as const

export const TEMP_LEVELS: Level[] = [
  { min: -Infinity, label: '10 以下', color: TEMP_COLD[0], ink: '#ffffff' },
  { min: 10, label: '10–15', color: TEMP_COLD[1], ink: '#ffffff' },
  { min: 15, label: '15–20', color: TEMP_COLD[2], ink: '#1c1917' },
  { min: 20, label: '20–25', color: '#b9b4ab', ink: '#1c1917' },
  { min: 25, label: '25–30', color: TEMP_HOT[0], ink: '#1c1917' },
  { min: 30, label: '30–35', color: TEMP_HOT[1], ink: '#1c1917' },
  { min: 35, label: '35 以上', color: TEMP_HOT[2], ink: '#ffffff' },
]

// 與平年相比的氣溫差
export const TEMP_ANOMALY_LEVELS: Level[] = [
  { min: -Infinity, label: '低 2 度以上', color: TEMP_COLD[0], ink: '#ffffff' },
  { min: -2, label: '低 1–2 度', color: TEMP_COLD[1], ink: '#ffffff' },
  { min: -1, label: '低 0.5–1 度', color: TEMP_COLD[2], ink: '#1c1917' },
  { min: -0.5, label: '差不多', color: '#b9b4ab', ink: '#1c1917' },
  { min: 0.5, label: '高 0.5–1 度', color: TEMP_HOT[0], ink: '#1c1917' },
  { min: 1, label: '高 1–2 度', color: TEMP_HOT[1], ink: '#1c1917' },
  { min: 2, label: '高 2 度以上', color: TEMP_HOT[2], ink: '#ffffff' },
]

const WIND_RAMP = ['#8cc47c', '#5aa553', '#378a3f', '#25732f'] as const

export const WIND_LEVELS: Level[] = [
  { min: 0, label: '3 級以下', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 5.5, label: '4–5 級', color: WIND_RAMP[0], ink: '#1c1917' },
  { min: 10.8, label: '6–7 級（強風）', color: WIND_RAMP[1], ink: '#1c1917' },
  { min: 17.2, label: '8–9 級', color: WIND_RAMP[2], ink: '#ffffff' },
  { min: 24.5, label: '10 級以上', color: WIND_RAMP[3], ink: '#ffffff' },
]

export const RAIN_CHANCE_LEVELS: Level[] = [
  { min: 0, label: '30% 以下', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 30, label: '30–50%', color: RAIN_RAMP[0], ink: '#1c1917' },
  { min: 50, label: '50–70%', color: RAIN_RAMP[1], ink: '#1c1917' },
  { min: 70, label: '70–90%', color: RAIN_RAMP[2], ink: '#ffffff' },
  { min: 90, label: '90% 以上', color: RAIN_RAMP[3], ink: '#ffffff' },
]

// 冷傷害指數越低越冷
export const COLD_INJURY_LEVELS: Level[] = [
  { min: -Infinity, label: '10 以下', color: TEMP_COLD[0], ink: '#ffffff' },
  { min: 10, label: '10–12', color: TEMP_COLD[1], ink: '#ffffff' },
  { min: 12, label: '12–14', color: TEMP_COLD[2], ink: '#1c1917' },
  { min: 14, label: '14 以上', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
]

// 熱傷害指數越高越容易中暑
export const HEAT_INJURY_LEVELS: Level[] = [
  { min: -Infinity, label: '26 以下', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 26, label: '26–29', color: TEMP_HOT[0], ink: '#1c1917' },
  { min: 29, label: '29–32', color: TEMP_HOT[1], ink: '#1c1917' },
  { min: 32, label: '32 以上', color: TEMP_HOT[2], ink: '#ffffff' },
]

// 今年累積雨量占平年的百分比
export const RAIN_NORMAL_LEVELS: Level[] = [
  { min: 0, label: '50% 以下', color: TEMP_HOT[2], ink: '#ffffff' },
  { min: 50, label: '50–80%', color: TEMP_HOT[0], ink: '#1c1917' },
  { min: 80, label: '80–120%', color: '#b9b4ab', ink: '#1c1917' },
  { min: 120, label: '120–150%', color: RAIN_RAMP[1], ink: '#1c1917' },
  { min: 150, label: '150% 以上', color: RAIN_RAMP[3], ink: '#ffffff' },
]

export const CLOSURE_LEVELS: Level[] = [
  {
    min: 0,
    label: '照常上班上課',
    color: 'var(--map-empty)',
    ink: 'var(--text-primary)',
    short: '照常',
  },
  { min: 1, label: '部分停止', color: OUTAGE_RAMP[0], ink: '#1c1917', short: '部分' },
  { min: 2, label: '停止上班上課', color: OUTAGE_RAMP[2], ink: '#ffffff', short: '停' },
]

export const DEBRIS_LEVELS: Level[] = [
  { min: 0, label: '沒有', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 1, label: '1–5 處', color: OUTAGE_RAMP[0], ink: '#1c1917' },
  { min: 6, label: '6–20 處', color: OUTAGE_RAMP[1], ink: '#1c1917' },
  { min: 21, label: '21–50 處', color: OUTAGE_RAMP[2], ink: '#ffffff' },
  { min: 51, label: '51 處以上', color: OUTAGE_RAMP[3], ink: '#ffffff' },
]

export const SPEED_LEVELS: Level[] = [
  { min: 0, label: '20 以下（壅塞）', color: '#a3201c', ink: '#ffffff' },
  { min: 20, label: '20–40（壅塞）', color: '#e0452b', ink: '#ffffff' },
  { min: 40, label: '40–60（車多）', color: '#f08a24', ink: '#1c1917' },
  { min: 60, label: '60–80（車多）', color: '#d4b106', ink: '#1c1917' },
  { min: 80, label: '80 以上（順暢）', color: '#3aa655', ink: '#ffffff' },
]

const FACILITY_RAMP = ['#9ba8ee', '#7385e0', '#5468cd', '#3f52b7'] as const

type Ramp = readonly [string, string, string, string]

function countLevels(
  b1: number,
  b2: number,
  b3: number,
  unit: string,
  ramp: Ramp = FACILITY_RAMP,
  continuous = false,
): Level[] {
  const top = (b: number) => (continuous ? b : b - 1)
  return [
    { min: 0, label: `未滿 ${b1} ${unit}`, color: ramp[0], ink: '#1c1917' },
    { min: b1, label: `${b1}–${top(b2)} ${unit}`, color: ramp[1], ink: '#1c1917' },
    { min: b2, label: `${b2}–${top(b3)} ${unit}`, color: ramp[2], ink: '#ffffff' },
    { min: b3, label: `${b3} ${unit}以上`, color: ramp[3], ink: '#ffffff' },
  ]
}

export const FACILITY_LEVELS: Record<string, Level[]> = {
  'hospital-by-county': countLevels(10, 20, 50, '家'),
  'clinic-by-county': countLevels(300, 1000, 2500, '家'),
  'pharmacy-by-county': countLevels(150, 500, 1000, '家'),
  'ltc-by-county': countLevels(200, 500, 1000, '處'),
  'shelter-by-county': countLevels(150, 250, 400, '處'),
  'medical-per-10k': countLevels(8, 10, 13, '家', FACILITY_RAMP, true),
  'population-by-county': countLevels(50, 100, 200, '萬人', FACILITY_RAMP, true),
  'population-density': countLevels(300, 1000, 3000, '人/km²'),
  'attraction-by-county': countLevels(150, 250, 400, '處', RAIN_RAMP),
  'hotel-by-county': countLevels(200, 500, 1000, '家', RAIN_RAMP),
  'accident-a1-by-county': countLevels(20, 50, 100, '人', OUTAGE_RAMP),
  'accident-a1-rate': countLevels(3, 6, 9, '人', OUTAGE_RAMP, true),
  'school-by-county': countLevels(100, 200, 300, '所'),
  'youbike-by-county': countLevels(500, 2000, 8000, '輛', WIND_RAMP),
  'tra-delay-top': countLevels(5, 15, 30, '分鐘', OUTAGE_RAMP),
  'birth-rate-by-county': countLevels(3, 4, 5, '‰', WARNING_RAMP, true),
  'childcare-by-county': countLevels(30, 60, 120, '家', WARNING_RAMP),
  'residential-power-per-household': countLevels(500, 600, 700, '度', OUTAGE_RAMP),
  'waste-per-capita': countLevels(1.2, 1.4, 1.6, '公斤', RAIN_RAMP, true),
  'fire-by-county': countLevels(500, 1000, 2000, '件', OUTAGE_RAMP),
  'housing-price-by-county': countLevels(20, 25, 40, '萬元/坪', WARNING_RAMP, true),
  'fuel-station-by-county': countLevels(30, 80, 150, '站', RAIN_RAMP),
  'dengue-by-county': countLevels(5, 20, 50, '例', OUTAGE_RAMP),
  'ili-by-county': countLevels(1, 2, 3, '%', OUTAGE_RAMP, true),
  'income-by-county': countLevels(90, 110, 130, '萬元', FACILITY_RAMP, true),
  'animal-by-county': countLevels(100, 300, 600, '隻', WARNING_RAMP),
  'flight-by-county': countLevels(20, 100, 500, '班', WIND_RAMP),
  'debris-alert-by-county': countLevels(5, 20, 50, '處', OUTAGE_RAMP),
  'flood-by-county': countLevels(1, 5, 20, '站', RAIN_RAMP),
  'ev-station-by-county': countLevels(100, 300, 600, '站', WIND_RAMP),
  'typhoon-event-rain': countLevels(200, 500, 1000, '毫米', RAIN_RAMP),
  'typhoon-event-outage': countLevels(1000, 10000, 50000, '戶', OUTAGE_RAMP),
  'typhoon-event-shelter': countLevels(50, 200, 1000, '人次', WARNING_RAMP),
}

export const SHELTER_PEOPLE_LEVELS: Level[] = [
  { min: 0, label: '沒有', color: 'var(--map-empty)', ink: 'var(--text-primary)' },
  { min: 1, label: '1–49 人', color: WARNING_RAMP[0], ink: '#1c1917' },
  { min: 50, label: '50–199 人', color: WARNING_RAMP[1], ink: '#1c1917' },
  { min: 200, label: '200–499 人', color: WARNING_RAMP[2], ink: '#ffffff' },
  { min: 500, label: '500 人以上', color: WARNING_RAMP[3], ink: '#ffffff' },
]

export const RPI_LEVELS: Level[] = [
  { min: 0, label: '未（稍）受污染', color: AQI_LEVELS[0]!.color, ink: '#ffffff' },
  { min: 2.01, label: '輕度污染', color: AQI_LEVELS[1]!.color, ink: '#1c1917' },
  { min: 3.01, label: '中度污染', color: AQI_LEVELS[2]!.color, ink: '#1c1917' },
  { min: 6.01, label: '嚴重污染', color: AQI_LEVELS[3]!.color, ink: '#ffffff' },
]

export const STATUS_LEVELS: Record<string, Level[]> = {
  'river-quality-status': RPI_LEVELS,
  'aqi-status': AQI_LEVELS,
  'river-alert-status': RIVER_ALERT_LEVELS,
  'quake-intensity-dist': QUAKE_INTENSITY_LEVELS,
}

export function levelsFor(slug: string, topic: string, unit: string): Level[] | null {
  if (slug === 'river-alert-margin') return RIVER_MARGIN_LEVELS
  if (slug === 'quake-max-intensity-by-county') return QUAKE_MAX_INTENSITY_LEVELS
  if (topic === 'earthquake' && unit === '規模') return QUAKE_MAGNITUDE_LEVELS
  if (slug === 'rain-by-county') return RAIN_24H_LEVELS
  if (slug === 'rain-1hr-top') return RAIN_1H_LEVELS
  if (slug === 'weather-warning-by-county') return WEATHER_WARNING_LEVELS
  if (slug === 'temp-vs-normal') return TEMP_ANOMALY_LEVELS
  if (slug === 'rain-vs-normal') return RAIN_NORMAL_LEVELS
  if (slug === 'cold-injury') return COLD_INJURY_LEVELS
  if (slug === 'heat-injury') return HEAT_INJURY_LEVELS
  if (unit === '°C') return TEMP_LEVELS
  if (slug === 'forecast-rain-chance') return RAIN_CHANCE_LEVELS
  if (
    slug === 'work-closure-by-county' ||
    slug === 'work-closure-tomorrow' ||
    slug === 'typhoon-event-closure'
  )
    return CLOSURE_LEVELS
  if (FACILITY_LEVELS[slug]) return FACILITY_LEVELS[slug]!
  if (slug === 'shelter-open') return SHELTER_PEOPLE_LEVELS
  if (slug === 'river-quality-by-county' || slug === 'river-quality-worst') return RPI_LEVELS
  if (slug === 'debris-flow-alert') return DEBRIS_LEVELS
  if (unit === '公里/小時') return SPEED_LEVELS
  if (unit === '公尺/秒' && topic === 'weather') return WIND_LEVELS
  if (topic === 'earthquake' && unit === '次') return QUAKE_COUNT_LEVELS
  if (slug.startsWith('pm25')) return PM25_LEVELS
  if (unit === 'UVI') return UV_LEVELS
  if (topic === 'aqi' && unit === '') return AQI_LEVELS
  if (topic === 'reservoir' && unit === '%') return WATER_LEVELS
  if (topic === 'outage' && slug !== 'water-outage-upcoming') {
    if (unit === '件') return OUTAGE_COUNT_LEVELS
    if (unit === '戶') return OUTAGE_HOUSEHOLD_LEVELS
  }
  return null
}

export function levelIn(levels: Level[], v: number): Level {
  let level = levels[0]!
  const rounded = levels === AQI_LEVELS || levels === UV_LEVELS
  for (const l of levels) if ((rounded ? Math.round(v) : v) >= l.min) level = l
  return level
}

export const SERIES_COLORS = [
  '#0f766e',
  '#2563eb',
  '#d97706',
  '#7c3aed',
  '#db2777',
  '#65a30d',
  '#0891b2',
  '#dc2626',
  '#57534e',
  '#ca8a04',
]

export const FUEL_COLORS: Record<string, string> = {
  燃氣: '#f97316',
  燃煤: '#57534e',
  燃油: '#a16207',
  汽電共生: '#a855f7',
  核能: '#dc2626',
  太陽能: '#eab308',
  風力: '#06b6d4',
  水力: '#2563eb',
  儲能: '#16a34a',
  其他再生能源: '#84cc16',
}

export const REGION_COLORS: Record<string, string> = {
  北部: '#2563eb',
  中部: '#0f766e',
  南部: '#d97706',
  東部: '#7c3aed',
}

export const POWER_COLORS: Record<string, string> = { ...FUEL_COLORS, ...REGION_COLORS }

export const RENEWABLE_FUELS = new Set(['太陽能', '風力', '水力', '其他再生能源'])
