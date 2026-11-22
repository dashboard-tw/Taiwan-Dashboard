import type {
  ComponentData,
  ComponentInfo,
  County,
  Dashboard,
  DashboardSummary,
  DataMeta,
} from './types'
import { pageTyphoon } from '../lib/pageContext'

export interface ApiEnvelope<T> {
  data: T
  meta: Record<string, unknown>
}

export interface ApiErrorBody {
  error: { code: string; message: string }
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export const API_BASE = '/api/v1'

export async function getJson<T>(path: string, init?: RequestInit): Promise<ApiEnvelope<T>> {
  const res = await fetch(`${API_BASE}${path}`, {
    cache: 'no-cache',
    ...init,
    headers: { Accept: 'application/json', ...init?.headers },
  })
  const body: unknown = await res.json().catch(() => null)

  if (!res.ok && !isEnvelope(body)) {
    const err = isErrorBody(body) ? body.error : { code: 'http_error', message: res.statusText }
    throw new ApiError(res.status, err.code, err.message)
  }
  if (!isEnvelope<T>(body)) {
    throw new ApiError(res.status, 'invalid_response', '伺服器回應格式不正確')
  }
  return body
}

export async function sendJson<T>(method: string, path: string, body?: unknown): Promise<T | null> {
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: {
      Accept: 'application/json',
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  if (res.status === 204) return null
  const parsed: unknown = await res.json().catch(() => null)
  if (!res.ok || !isEnvelope<T>(parsed)) {
    const err = isErrorBody(parsed) ? parsed.error : { code: 'http_error', message: res.statusText }
    throw new ApiError(res.status, err.code, err.message)
  }
  return parsed.data
}

function isEnvelope<T>(v: unknown): v is ApiEnvelope<T> {
  return typeof v === 'object' && v !== null && 'data' in v
}

function isErrorBody(v: unknown): v is ApiErrorBody {
  return typeof v === 'object' && v !== null && 'error' in v
}

export interface HealthStatus {
  status: 'ok' | 'degraded'
  database: 'ok' | 'unavailable'
  version: string
}

export function getHealth(): Promise<ApiEnvelope<HealthStatus>> {
  return getJson<HealthStatus>('/health')
}

const FRESH_MS = 60_000

interface Remembered {
  at: number
  value: ApiEnvelope<unknown>
}

const remembered = new Map<string, Remembered>()
const inflight = new Map<string, Promise<ApiEnvelope<unknown>>>()

export async function getCached<T>(path: string, force = false): Promise<ApiEnvelope<T>> {
  const prev = remembered.get(path)
  if (!force && prev && Date.now() - prev.at < FRESH_MS) return prev.value as ApiEnvelope<T>

  let p = inflight.get(path)
  if (!p) {
    p = getJson<unknown>(path).finally(() => inflight.delete(path))
    inflight.set(path, p)
  }
  try {
    const value = await p
    remembered.set(path, { at: Date.now(), value })
    persist(path, value)
    persistData(path, value)
    return value as ApiEnvelope<T>
  } catch (err) {
    if (!(err instanceof ApiError && err.status < 500)) {
      const last = prev?.value ?? storedData(path)
      if (last) return { ...last, meta: { ...last.meta, stale: true } } as ApiEnvelope<T>
    }
    throw err
  }
}

const DATA_STORE = 'dashboardtw:data:'
const DATA_PATH = /^\/components\/[^/?]+\/data(\?|$)/
const DATA_MAX_CHARS = 64_000

function persistData(path: string, value: ApiEnvelope<unknown>) {
  if (!DATA_PATH.test(path)) return
  try {
    const s = JSON.stringify(value)
    if (s.length <= DATA_MAX_CHARS) localStorage.setItem(DATA_STORE + path, s)
  } catch {}
}

function storedData(path: string): ApiEnvelope<unknown> | null {
  if (!DATA_PATH.test(path)) return null
  try {
    const v = localStorage.getItem(DATA_STORE + path)
    return v ? (JSON.parse(v) as ApiEnvelope<unknown>) : null
  } catch {
    return null
  }
}

const STORE = 'dashboardtw:config:'
const CONFIG_PATH = /^\/(dashboards(\/[^/?]+)?|components(\/[^/?]+)?)$/

function persist(path: string, value: ApiEnvelope<unknown>) {
  if (!CONFIG_PATH.test(path)) return
  try {
    localStorage.setItem(STORE + path, JSON.stringify(value.data))
  } catch {}
}

export function peek<T>(path: string): T | null {
  const mem = remembered.get(path)
  if (mem) return mem.value.data as T
  if (!CONFIG_PATH.test(path)) return null
  try {
    const v = localStorage.getItem(STORE + path)
    return v ? (JSON.parse(v) as T) : null
  } catch {
    return null
  }
}

export function clearCache() {
  remembered.clear()
  inflight.clear()
}

export async function getCounties(): Promise<County[]> {
  return (await getCached<County[]>('/counties')).data
}

export async function getDashboards(): Promise<DashboardSummary[]> {
  return (await getCached<DashboardSummary[]>('/dashboards')).data
}

export function dashboardPath(slug: string) {
  return `/dashboards/${encodeURIComponent(slug)}`
}

export function componentPath(slug: string) {
  return `/components/${encodeURIComponent(slug)}`
}

export function peekDashboard(slug: string): Dashboard | null {
  return peek<Dashboard>(dashboardPath(slug))
}

export function peekComponent(slug: string): ComponentInfo | null {
  return peek<ComponentInfo>(componentPath(slug))
}

export async function prefetchDashboards(): Promise<void> {
  try {
    for (const d of await getDashboards()) await getDashboard(d.slug)
  } catch {}
}

export async function getDashboard(slug: string): Promise<Dashboard> {
  return (await getCached<Dashboard>(dashboardPath(slug))).data
}

export async function getComponents(): Promise<ComponentInfo[]> {
  return (await getCached<ComponentInfo[]>('/components')).data
}

export function peekComponents(): ComponentInfo[] | null {
  return peek<ComponentInfo[]>('/components')
}

export async function getComponent(slug: string): Promise<ComponentInfo> {
  return (await getCached<ComponentInfo>(componentPath(slug))).data
}

export interface DataQuery {
  county?: string
  from?: string
  to?: string
  typhoon?: string
}

export function dataPath(
  slug: string,
  kind: 'data' | 'history',
  q: DataQuery = {},
  format?: 'csv',
) {
  const params = new URLSearchParams()
  if (q.county) params.set('county', q.county)
  if (q.from) params.set('from', q.from)
  if (q.to) params.set('to', q.to)
  const typhoon = q.typhoon ?? pageTyphoon.value
  if (typhoon) params.set('typhoon', typhoon)
  if (format) params.set('format', format)
  const qs = params.toString()
  return `/components/${encodeURIComponent(slug)}/${kind}${qs ? `?${qs}` : ''}`
}

export function layerUrl(slug: string, id: number, county?: string | null): string {
  const params = new URLSearchParams()
  if (county) params.set('county', county)
  if (pageTyphoon.value) params.set('typhoon', pageTyphoon.value)
  const qs = params.toString()
  return `${API_BASE}/components/${encodeURIComponent(slug)}/layers/${id}.geojson${qs ? `?${qs}` : ''}`
}

export interface TyphoonEvent {
  id: string
  ty_no: number | null
  name_zh: string | null
  name_en: string | null
  starts_at: string
  ends_at?: string
  active: boolean
  last_alert: string
  warnings: number
}

export async function getTyphoons(force = false): Promise<TyphoonEvent[]> {
  const env = await getCached<TyphoonEvent[]>('/typhoons', force)
  return env.data
}

export async function getComponentData(
  slug: string,
  kind: 'data' | 'history' = 'data',
  q: DataQuery = {},
  force = false,
): Promise<ComponentData> {
  const env = await getCached<ComponentData['data']>(dataPath(slug, kind, q), force)
  return { data: env.data, meta: env.meta as unknown as DataMeta }
}

export interface CountyFeature {
  type: 'Feature'
  id: string
  geometry: GeoJSON.Geometry
  properties: { code: string; name: string; short_name: string }
}

export interface CountyCollection {
  type: 'FeatureCollection'
  features: CountyFeature[]
}

let geo: Promise<CountyCollection> | null = null

async function fetchCountiesGeo(): Promise<CountyCollection> {
  const r = await fetch(`${API_BASE}/geo/counties.geojson`, { cache: 'no-cache' })
  if (!r.ok) throw new ApiError(r.status, 'http_error', r.statusText)
  return (await r.json()) as CountyCollection
}

export function getCountiesGeo(): Promise<CountyCollection> {
  if (!geo) {
    geo = (async () => {
      for (const wait of [0, 1000, 3000]) {
        if (wait) await new Promise((r) => setTimeout(r, wait))
        try {
          return await fetchCountiesGeo()
        } catch (e) {
          if (wait === 3000) throw e
        }
      }
      throw new Error('unreachable')
    })()
    geo.catch(() => (geo = null))
  }
  return geo
}

export interface Quake {
  id: string
  kind: 'significant' | 'local'
  no: number | null
  origin_time: string
  location: string | null
  magnitude: number | null
  depth_km: number | null
  max_intensity: string | null
  max_intensity_rank: number | null
  color: string | null
}

export interface QuakeDetail extends Quake {
  issued_at: string | null
  content: string | null
  report_url: string | null
  image_url: string | null
  lon: number | null
  lat: number | null
  intensities: { county_code: string; county: string; intensity: string; rank: number }[]
}

export async function getQuakes(force = false): Promise<Quake[]> {
  return (await getCached<Quake[]>('/earthquakes', force)).data
}

export async function getQuake(id: string): Promise<QuakeDetail> {
  return (await getJson<QuakeDetail>(`/earthquakes/${encodeURIComponent(id)}`)).data
}
