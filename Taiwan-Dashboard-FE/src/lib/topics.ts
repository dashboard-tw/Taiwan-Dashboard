import type { Topic } from '../api/types'

export interface TopicInfo {
  slug: Topic
  label: string
  title: string
  group: TopicGroup
  mapComponent: string | null
  statusComponent: string
}

export type TopicGroup = 'weather' | 'environment' | 'utilities' | 'transport' | 'society'

export const TOPIC_GROUPS: { id: TopicGroup; label: string }[] = [
  { id: 'weather', label: '天氣與防災' },
  { id: 'environment', label: '環境' },
  { id: 'utilities', label: '民生' },
  { id: 'transport', label: '交通' },
  { id: 'society', label: '人口與社會' },
]

export const TOPICS: TopicInfo[] = [
  {
    slug: 'weather',
    label: '天氣',
    title: '天氣',
    group: 'weather',
    mapComponent: 'county-forecast',
    statusComponent: 'temp-by-county',
  },
  {
    slug: 'warning',
    label: '颱風警特報',
    title: '颱風與警特報',
    group: 'weather',
    mapComponent: 'weather-warning-by-county',
    statusComponent: 'weather-warning-by-county',
  },
  {
    slug: 'earthquake',
    label: '地震',
    title: '地震',
    group: 'weather',
    mapComponent: 'quake-felt-by-county',
    statusComponent: 'quake-recent',
  },
  {
    slug: 'fire',
    label: '火災',
    title: '火災',
    group: 'weather',
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
    label: '永續環境',
    title: '永續環境',
    group: 'environment',
    mapComponent: 'residential-power-per-household',
    statusComponent: 'power-sales-mix',
  },
  {
    slug: 'power',
    label: '電力',
    title: '電力',
    group: 'utilities',
    mapComponent: null,
    statusComponent: 'power-gen-mix',
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
    group: 'utilities',
    mapComponent: 'hospital-by-county',
    statusComponent: 'hospital-by-county',
  },
  {
    slug: 'disease',
    label: '傳染病',
    title: '傳染病',
    group: 'utilities',
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
    group: 'utilities',
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
    slug: 'safety',
    label: '交通安全',
    title: '交通安全',
    group: 'transport',
    mapComponent: 'accident-a1-by-county',
    statusComponent: 'accident-a1-by-county',
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
    slug: 'family',
    label: '婦幼家庭',
    title: '婦幼與家庭',
    group: 'society',
    mapComponent: 'birth-rate-by-county',
    statusComponent: 'vital-12m',
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
  family: 60,
  fire: 60,
  food: 60,
  housing: 60,
  disease: 60,
  economy: 60,
  animal: 60,
  sustain: 60,
  tourism: 60,
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
  family: 'stroller',
  fire: 'local_fire_department',
  agri: 'agriculture',
  food: 'restaurant',
  housing: 'home_work',
  disease: 'coronavirus',
  aviation: 'flight',
  economy: 'trending_up',
  animal: 'pets',
  sustain: 'eco',
  county: 'location_on',
}
