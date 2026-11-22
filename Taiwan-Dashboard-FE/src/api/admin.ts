import { getJson, sendJson } from './client'
import type { DataShape, Scope, Topic } from './types'
import type { MyApp } from './apps'

export interface ReservoirRef {
  id: string
  name: string
  county_code: string | null
  effective_capacity: number | null
  capacity_locked: boolean
  capacity_updated_on: string | null
  is_major: boolean
  lon: number | null
  lat: number | null
  storage_pct: number | null
  observed_at: string | null
  has_storage: boolean
}

export type ReservoirInput = Pick<
  ReservoirRef,
  'name' | 'county_code' | 'effective_capacity' | 'capacity_locked' | 'is_major' | 'lon' | 'lat'
>

export interface PowerUnitRef {
  id: number
  unit_name: string
  plant_name: string | null
  fuel_type: string
  is_private: boolean
  county_code: string | null
  net_gen_mw: number | null
}

export interface RunResult {
  rows_fetched: number
  rows_inserted: number
  warnings: string[]
  last_observed_at: string
}

export async function getReservoirs(): Promise<ReservoirRef[]> {
  return (await getJson<ReservoirRef[]>('/admin/ref/reservoirs')).data
}

export async function updateReservoir(id: string, input: ReservoirInput): Promise<void> {
  await sendJson('PUT', `/admin/ref/reservoirs/${encodeURIComponent(id)}`, input)
}

export async function getPowerUnits(): Promise<PowerUnitRef[]> {
  return (await getJson<PowerUnitRef[]>('/admin/ref/power-units')).data
}

export async function updatePowerUnit(id: number, county: string | null): Promise<void> {
  await sendJson('PUT', `/admin/ref/power-units/${id}`, { county_code: county })
}

export async function runDataset(id: string): Promise<RunResult> {
  return (await sendJson<RunResult>('POST', `/admin/datasets/${encodeURIComponent(id)}/run`))!
}

export type RunAllStepStatus = 'pending' | 'running' | 'ok' | 'warning' | 'failed' | 'skipped'

export interface RunAllStep {
  id: string
  name: string
  status: RunAllStepStatus
  rows_fetched: number
  rows_inserted: number
  message?: string
  seconds?: number
}

export interface RunAllState {
  running: boolean
  started_at: string | null
  finished_at: string | null
  done: number
  total: number
  current?: string
  steps: RunAllStep[]
}

export async function getRunAll(): Promise<RunAllState> {
  return (await getJson<RunAllState>('/admin/datasets/run-all')).data
}

export async function startRunAll(): Promise<RunAllState> {
  return (await sendJson<RunAllState>('POST', '/admin/datasets/run-all'))!
}

export interface QuerySpec {
  scope: Scope
  data_shape: DataShape
  chart_sql: string
  history_sql?: string
}

export interface MapLayerSpec {
  title: string
  layer_type: 'circle' | 'symbol' | 'fill' | 'line' | 'heatmap' | 'raster' | 'wind'
  source_type: 'county_boundary' | 'query'
  geojson_sql?: string
  style?: unknown
  popup_fields?: unknown
}

export interface ComponentSpec {
  slug: string
  name: string
  topic: Topic
  short_desc: string
  long_desc: string
  use_case: string
  dataset_ids: string[]
  time_range: string
  chart_types: string[]
  colors: string[]
  unit: string
  decimals: number
  chart_map_link?: unknown
  links: string[]
  status: 'draft' | 'published'
  queries: QuerySpec[]
  map_layers: MapLayerSpec[]
}

export interface ComponentSummary {
  slug: string
  name: string
  topic: Topic
  status: 'draft' | 'published'
  updated_at: string
  versions: number
  dashboards: string[]
}

export interface ComponentVersion {
  id: number
  note: string
  status: string
  created_at: string
  created_by: string | null
}

export interface PreviewCheck {
  label: string
  scope?: Scope
  kind: 'chart' | 'history' | 'layer'
  columns: string[]
  rows: unknown[][]
  row_count: number
  elapsed_ms: number
  data?: unknown
  error?: string
}

export interface Preview {
  county: string
  checks: PreviewCheck[]
}

const base = (slug: string) => `/admin/components/${encodeURIComponent(slug)}`

export async function listComponents(): Promise<ComponentSummary[]> {
  return (await getJson<ComponentSummary[]>('/admin/components')).data
}

export async function getComponentSpec(slug: string) {
  return (await getJson<{ spec: ComponentSpec; version: number }>(base(slug))).data
}

export async function previewComponent(spec: ComponentSpec, county?: string): Promise<Preview> {
  return (await sendJson<Preview>('POST', '/admin/components/preview', { spec, county }))!
}

export async function createComponent(spec: ComponentSpec, note: string): Promise<number> {
  return (await sendJson<{ version: number }>('POST', '/admin/components', { spec, note }))!.version
}

export async function saveComponent(
  spec: ComponentSpec,
  note: string,
  baseVersion: number,
): Promise<number> {
  const r = await sendJson<{ version: number }>('PUT', base(spec.slug), {
    spec,
    note,
    base_version: baseVersion,
  })
  return r!.version
}

export async function setComponentStatus(
  slug: string,
  status: 'draft' | 'published',
  baseVersion: number,
): Promise<number> {
  const r = await sendJson<{ version: number }>('POST', `${base(slug)}/status`, {
    status,
    base_version: baseVersion,
  })
  return r!.version
}

export async function getVersions(slug: string): Promise<ComponentVersion[]> {
  return (await getJson<ComponentVersion[]>(`${base(slug)}/versions`)).data
}

export async function getVersion(slug: string, id: number) {
  return (await getJson<{ spec: ComponentSpec; version: number }>(`${base(slug)}/versions/${id}`))
    .data
}

export async function restoreVersion(slug: string, id: number): Promise<number> {
  return (await sendJson<{ version: number }>('POST', `${base(slug)}/versions/${id}/restore`))!
    .version
}

export interface CountRow {
  value: string
  count: number
  active: number
}

export interface SurveyNote {
  updated_at: string
  identity: string
  purpose: string
  topics: string[]
  app_name: string
  app_url: string
  suggestion: string
  active: boolean
}

export interface SurveySummary {
  respondents: number
  active_respondents: number
  active_keys: number
  identity: CountRow[]
  purpose: CountRow[]
  topics: CountRow[]
  wants_push: CountRow[]
  notes: SurveyNote[]
}

export async function getSurveySummary(): Promise<SurveySummary> {
  return (await getJson<SurveySummary>('/admin/api-survey')).data
}

export type AccountStatus = 'active' | 'idle' | 'revoked'

export interface UsageDay {
  day: string
  requests: number
  rejected: number
  accounts: number
}

export interface UsageAccount {
  user_id: number
  display_name: string
  prefix: string | null
  status: AccountStatus
  requests: number
  rejected: number
  days_used: number
  last_used_at: string | null
}

export interface ApiUsage {
  days: number
  idle_days: number
  daily_limit: number
  accounts: Record<AccountStatus | 'never_used', number>
  totals: {
    requests: number
    rejected: number
    accounts: number
    quota_hits: number
    avg_per_day: number
    avg_per_account: number
  }
  daily: UsageDay[]
  datasets: { dataset: string; requests: number }[]
  top: UsageAccount[]
}

export async function getApiUsage(days: number): Promise<ApiUsage> {
  return (await getJson<ApiUsage>(`/admin/api-usage?days=${days}`)).data
}

export async function revokeUserKey(userId: number): Promise<void> {
  await sendJson('DELETE', `/admin/users/${userId}/api-key`)
}

export interface AdminUser {
  id: number
  display_name: string
  role: 'user' | 'admin'
  status: 'active' | 'disabled'
  created_at: string
  last_login_at: string | null
}

export interface AdminUserDetail {
  user: AdminUser
  is_me: boolean
  has_email: boolean
  active_sessions: number
  last_seen_at: string | null
  providers: { provider: string; created_at: string }[]
  clients: {
    name: string
    client_id: string
    enabled: boolean
    created_at: string
    last_used_at: string | null
  }[]
  usage_today: { requests: number; bytes: number }
  usage_30d: { requests: number; bytes: number }
  survey: {
    identity: string
    purpose: string
    wants_push: string | null
    updated_at: string
  } | null
  events: { action: string; target: string; created_at: string }[]
}

export async function emailDigest(email: string): Promise<string> {
  const bytes = new TextEncoder().encode(email.trim().toLowerCase())
  const hash = await crypto.subtle.digest('SHA-256', bytes)
  return [...new Uint8Array(hash)].map((b) => b.toString(16).padStart(2, '0')).join('')
}

export async function searchUsersByEmail(email: string): Promise<AdminUser[]> {
  const found = await sendJson<AdminUser[]>('POST', '/admin/users/search', {
    email_sha256: await emailDigest(email),
  })
  return found ?? []
}

export async function getAdminUser(id: number): Promise<AdminUserDetail> {
  return (await getJson<AdminUserDetail>(`/admin/users/${id}`)).data
}

export async function updateAdminUser(
  id: number,
  change: Partial<Pick<AdminUser, 'role' | 'status'>>,
): Promise<void> {
  await sendJson('PATCH', `/admin/users/${id}`, change)
}

export interface DatasetHealth {
  id: string
  name: string
  agency: string
  url: string
  status: 'pending' | 'ok' | 'stale' | 'error'
  update_freq_seconds: number
  stale_after_seconds: number
  last_observed_at: string | null
  last_ingested_at: string | null
  last_error: string | null
  runs_today: number
  failures_today: number
}

export interface PipelineRun {
  id: number
  dataset: string
  started_at: string
  finished_at: string | null
  status: 'running' | 'success' | 'partial' | 'failed'
  rows_fetched: number
  rows_inserted: number
  warnings: string[]
  error: string | null
}

export async function getDatasetHealth(): Promise<DatasetHealth[]> {
  return (await getJson<DatasetHealth[]>('/admin/datasets')).data
}

export async function getPipelineRuns(dataset?: string): Promise<PipelineRun[]> {
  const qs = dataset ? `?dataset=${encodeURIComponent(dataset)}` : ''
  return (await getJson<PipelineRun[]>(`/admin/pipeline-runs${qs}`)).data
}

export interface AdminOverview {
  users: {
    total: number
    admins: number
    disabled: number
    new_7d: number
    active_7d: number
    sessions: number
  }
  api: {
    clients: number
    accounts: number
    requests_today: number
    bytes_today: number
    rejected_today: number
    requests_7d: number
  }
  datasets: Record<'ok' | 'stale' | 'error' | 'pending', number>
  problems: {
    id: string
    name: string
    status: 'stale' | 'error'
    last_ingested_at: string | null
    last_error: string | null
  }[]
  runs_24h: { total: number; failed: number }
  failures: { dataset: string; started_at: string; error: string | null }[]
  components: { published: number; draft: number; dashboards: number }
  days: { day: string; new_users: number; api_requests: number; pipeline_failures: number }[]
}

export async function getAdminOverview(): Promise<AdminOverview> {
  return (await getJson<AdminOverview>('/admin/overview')).data
}

export interface AdminApp extends MyApp {
  user_id: number
  display_name: string
}

export async function getShowcaseApps(status = ''): Promise<AdminApp[]> {
  const q = status ? `?status=${encodeURIComponent(status)}` : ''
  return (await getJson<AdminApp[]>(`/admin/apps${q}`, { credentials: 'same-origin' })).data
}

export async function reviewApp(
  id: number,
  action: 'approve' | 'reject' | 'remove',
  note = '',
): Promise<void> {
  await sendJson('POST', `/admin/apps/${id}/review`, { action, note })
}
