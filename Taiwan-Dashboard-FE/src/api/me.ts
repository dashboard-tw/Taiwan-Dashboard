import { getJson, sendJson } from './client'

export interface Account {
  id: number
  display_name: string
  role: 'user' | 'admin'
  created_at: string
  providers: string[]
}

export async function getAccount(): Promise<Account> {
  return (await getJson<Account>('/me/account')).data
}

export async function renameAccount(displayName: string): Promise<string> {
  const r = await sendJson<{ display_name: string }>('PUT', '/me/account', {
    display_name: displayName,
  })
  return r?.display_name ?? displayName
}

export async function startLinkProvider(provider: string): Promise<string> {
  const r = await sendJson<{ redirect_url: string }>(
    'POST',
    `/auth/${encodeURIComponent(provider)}/link`,
  )
  return r!.redirect_url
}

export async function unlinkProvider(provider: string): Promise<void> {
  await sendJson('DELETE', `/me/identities/${encodeURIComponent(provider)}`)
}

export async function deleteAccount(): Promise<void> {
  await sendJson('DELETE', '/me', { confirm: true })
}

export interface Device {
  id: number
  created_at: string
  last_seen_at: string | null
  expires_at: string
  ip: string
  user_agent: string
  current: boolean
}

export async function getDevices(): Promise<Device[]> {
  return (await getJson<Device[]>('/me/devices')).data
}

export async function signOutDevice(id: number): Promise<void> {
  await sendJson('DELETE', `/me/devices/${id}`)
}

export async function signOutOtherDevices(): Promise<number> {
  return (await sendJson<{ signed_out: number }>('DELETE', '/me/devices'))?.signed_out ?? 0
}

export interface ApiClient {
  id: number
  name: string
  client_id: string
  secret_hint: string
  enabled: boolean
  created_at: string
  last_used_at: string | null
}

export interface DayUsage {
  day: string
  requests: number
  bytes: number
}

export interface ApiSurvey {
  identity: string
  purpose: string
  app_name: string
  app_url: string
  topics: string[]
  suggestion: string
  wants_push?: string | null
  terms_version?: string
  updated_at?: string
}

export interface ApiClientsState {
  clients: ApiClient[]
  max_clients: number
  used_today: { requests: number; bytes: number }
  usage: DayUsage[]
  tier: 'free' | 'partner'
  limits: { PerDay: number; PerMinute: number; BytesPerDay: number }
  partner_limits: { PerDay: number; PerMinute: number; BytesPerDay: number }
  token_ttl: number
  survey: ApiSurvey | null
  terms_version: string
}

export async function saveApiSurvey(s: ApiSurvey): Promise<void> {
  await sendJson('PUT', '/me/api-survey', {
    identity: s.identity,
    purpose: s.purpose,
    app_name: s.app_name,
    app_url: s.app_url,
    topics: s.topics,
    suggestion: s.suggestion,
    wants_push: s.wants_push,
    agree_terms: true,
  })
}

export async function getApiClients(): Promise<ApiClientsState> {
  return (await getJson<ApiClientsState>('/me/api-clients')).data
}

export async function createApiClient(
  name: string,
): Promise<{ client: ApiClient; client_secret: string }> {
  return (await sendJson<{ client: ApiClient; client_secret: string }>('POST', '/me/api-clients', {
    name,
  }))!
}

export async function rotateClientSecret(id: number): Promise<string> {
  return (await sendJson<{ client_secret: string }>('POST', `/me/api-clients/${id}/secret`))!
    .client_secret
}

export async function setClientEnabled(id: number, enabled: boolean): Promise<void> {
  await sendJson('PATCH', `/me/api-clients/${id}`, { enabled })
}

export async function deleteApiClient(id: number): Promise<void> {
  await sendJson('DELETE', `/me/api-clients/${id}`)
}
