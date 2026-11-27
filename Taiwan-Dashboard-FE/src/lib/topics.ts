import type { Topic } from '../api/types'

export interface TopicInfo {
  slug: Topic
  label: string
  title: string
  group: TopicGroup
  mapComponent: string | null
  statusComponent: string
}

export type TopicGroup =
  | 'weather'
  | 'disaster'
  | 'environment'
  | 'energy'
  | 'utilities'
  | 'medical'
  | 'transport'
  | 'society'

export const TOPIC_GROUPS: { id: TopicGroup; label: string }[] = [
  { id: 'weather', label: '天氣' },
  { id: 'disaster', label: '防災' },
  { id: 'environment', label: '環境' },
  { id: 'energy', label: '能源' },
  { id: 'utilities', label: '民生' },
  { id: 'medical', label: '醫療衛生' },
  { id: 'transport', label: '交通' },
  { id: 'society', label: '人口與社會' },
]

export const TOPICS: TopicInfo[] = [
  {
    slug: 'weather',
    label: '天氣',
    title: '天氣',
    group: 'weather',
    mapComponent: 'temp-by-county',
    statusComponent: 'temp-by-county',
  },
  {
    slug: 'forecast',
    label: '天氣預報',
    title: '天氣預報',
    group: 'weather',
    mapComponent: 'county-forecast',
    statusComponent: 'week-forecast',
  },
  {
    slug: 'health',
    label: '健康氣象',
    title: '健康氣象',
    group: 'weather',
    mapComponent: 'cold-injury',
    statusComponent: 'health-warning-list',
  },
  {
    slug: 'ocean',
    label: '海洋天文',
    title: '海洋與天文',
    group: 'weather',
    mapComponent: 'tide-forecast',
    statusComponent: 'sun-moon',
  },
  {
    slug: 'warning',
    label: '颱風警特報',
    title: '颱風與警特報',
    group: 'disaster',
    mapComponent: 'weather-warning-by-county',
    statusComponent: 'weather-warning-by-county',
  },
  {
    slug: 'flood',
    label: '土石流淹水',
    title: '土石流與淹水',
    group: 'disaster',
    mapComponent: 'flood-by-county',
    statusComponent: 'flood-summary',
  },
  {
    slug: 'earthquake',
    label: '地震',
    title: '地震',
    group: 'disaster',
    mapComponent: 'quake-felt-by-county',
    statusComponent: 'quake-recent',
  },
  {
    slug: 'fire',
    label: '火災',
    title: '火災',
    group: 'disaster',
    mapComponent: 'fire-by-county',
    statusComponent: 'fire-summary',
  },
  {
    slug: 'aqi',
    label: '空品',
    title: '空氣品質',
    group: 'environment',
    mapComponent: 'aqi-by-county',
    statusComponent: 'aqi-by-county',
  },
  {
    slug: 'reservoir',
    label: '水情',
    title: '水情',
    group: 'environment',
    mapComponent: 'reservoir-by-county',
    statusComponent: 'reservoir-storage',
  },
  {
    slug: 'sustain',
    label: '環境品質',
    title: '環境品質',
    group: 'environment',
    mapComponent: 'waste-per-capita',
    statusComponent: 'recycling-rate',
  },
  {
    slug: 'power',
    label: '電力',
    title: '電力',
    group: 'energy',
    mapComponent: null,
    statusComponent: 'power-gen-mix',
  },
  {
    slug: 'power-use',
    label: '用電統計',
    title: '用電統計',
    group: 'energy',
    mapComponent: 'residential-power-per-household',
    statusComponent: 'power-sales-mix',
  },
  {
    slug: 'outage',
    label: '停水停電',
    title: '停水停電',
    group: 'utilities',
    mapComponent: 'water-outage-by-county',
    statusComponent: 'water-outage-by-county',
  },
  {
    slug: 'resources',
    label: '醫療長照',
    title: '醫療與長照',
    group: 'medical',
    mapComponent: 'hospital-by-county',
    statusComponent: 'hospital-by-county',
  },
  {
    slug: 'disease',
    label: '傳染病',
    title: '傳染病',
    group: 'medical',
    mapComponent: 'dengue-by-county',
    statusComponent: 'dengue-summary',
  },
  {
    slug: 'agri',
    label: '農產價格',
    title: '農產品價格',
    group: 'utilities',
    mapComponent: null,
    statusComponent: 'farm-summary',
  },
  {
    slug: 'food',
    label: '食品安全',
    title: '食品安全',
    group: 'medical',
    mapComponent: null,
    statusComponent: 'food-rejection-summary',
  },
  {
    slug: 'housing',
    label: '房價',
    title: '房價',
    group: 'utilities',
    mapComponent: 'housing-price-by-county',
    statusComponent: 'housing-price-by-county',
  },
  {
    slug: 'traffic',
    label: '道路交通',
    title: '道路交通',
    group: 'transport',
    mapComponent: 'freeway-speed-by-county',
    statusComponent: 'freeway-congestion',
  },
  {
    slug: 'fuel',
    label: '加油充電',
    title: '加油與充電',
    group: 'energy',
    mapComponent: 'fuel-station-by-county',
    statusComponent: 'fuel-price-now',
  },
  {
    slug: 'rail',
    label: '鐵路',
    title: '鐵路',
    group: 'transport',
    mapComponent: null,
    statusComponent: 'tra-delay-now',
  },
  {
    slug: 'aviation',
    label: '航空',
    title: '航空',
    group: 'transport',
    mapComponent: 'flight-by-county',
    statusComponent: 'flight-summary',
  },
  {
    slug: 'bike',
    label: '共享單車',
    title: '共享單車',
    group: 'transport',
    mapComponent: 'youbike-by-county',
    statusComponent: 'youbike-summary',
  },
  {
    slug: 'population',
    label: '人口',
    title: '人口',
    group: 'society',
    mapComponent: 'population-by-county',
    statusComponent: 'population-by-county',
  },
  {
    slug: 'education',
    label: '教育托育',
    title: '教育與托育',
    group: 'society',
    mapComponent: 'school-by-county',
    statusComponent: 'school-by-county',
  },
  {
    slug: 'economy',
    label: '經濟就業',
    title: '經濟與就業',
    group: 'society',
    mapComponent: 'income-by-county',
    statusComponent: 'econ-summary',
  },
  {
    slug: 'animal',
    label: '動物保護',
    title: '動物保護',
    group: 'society',
    mapComponent: 'animal-by-county',
    statusComponent: 'animal-summary',
  },
  {
    slug: 'tourism',
    label: '觀光',
    title: '觀光',
    group: 'society',
    mapComponent: 'hotel-by-county',
    statusComponent: 'attraction-by-county',
  },
]

export const EXTRA_DASHBOARDS = ['typhoon', 'mapdata'] as const

export const REFRESH_MINUTES: Record<string, number> = {
  traffic: 5,
  rail: 5,
  aviation: 10,
  earthquake: 10,
  warning: 10,
  typhoon: 10,
  resources: 60,
  population: 60,
  education: 60,
  'power-use': 60,
  fire: 60,
  food: 60,
  housing: 60,
  disease: 60,
  economy: 60,
  animal: 60,
  sustain: 60,
  tourism: 60,
  health: 60,
  ocean: 60,
  flood: 10,
  fuel: 60,
}

export function refreshMs(slug: string | undefined): number {
  return (REFRESH_MINUTES[slug ?? ''] ?? 20) * 60_000
}

export const TOPIC_LABELS: Record<string, string> = Object.fromEntries(
  TOPICS.map((t) => [t.slug, t.label]),
)

export const TOPIC_TITLES: Record<string, string> = Object.fromEntries(
  TOPICS.map((t) => [t.slug, t.title]),
)

export function topicInfo(slug: string): TopicInfo | undefined {
  return TOPICS.find((t) => t.slug === slug)
}

export function isTopic(slug: string): slug is Topic {
  return TOPICS.some((t) => t.slug === slug)
}

export const DASHBOARD_ICONS: Record<string, string> = {
  home: 'dashboard',
  aqi: 'air',
  reservoir: 'water_drop',
  power: 'bolt',
  outage: 'warning',
  earthquake: 'vibration',
  weather: 'wb_cloudy',
  forecast: 'calendar_month',
  health: 'health_and_safety',
  ocean: 'waves',
  flood: 'flood',
  fuel: 'local_gas_station',
  warning: 'cyclone',
  typhoon: 'crisis_alert',
  'typhoon-event': 'cyclone',
  typhoons: 'history',
  mapdata: 'layers',
  traffic: 'directions_car',
  rail: 'train',
  bike: 'pedal_bike',
  safety: 'car_crash',
  resources: 'local_hospital',
  population: 'groups',
  tourism: 'luggage',
  'power-use': 'electric_meter',
  education: 'school',
  fire: 'local_fire_department',
  agri: 'agriculture',
  food: 'restaurant',
  housing: 'home_work',
  disease: 'coronavirus',
  aviation: 'flight',
  economy: 'trending_up',
  animal: 'pets',
  sustain: 'recycling',
  county: 'location_on',
}
