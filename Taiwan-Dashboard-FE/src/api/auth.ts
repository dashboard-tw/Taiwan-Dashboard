import { ApiError, getJson, sendJson } from './client'

export interface Me {
  id: number
  display_name: string
  role: 'user' | 'admin'
}

export interface Provider {
  id: string
  label: string
  dev: boolean
}

export async function getMe(): Promise<Me | null> {
  return (await getJson<Me | null>('/auth/session')).data
}

export async function getProviders(): Promise<Provider[]> {
  return (await getJson<Provider[]>('/auth/providers')).data
}

function post<T>(path: string): Promise<T | null> {
  return sendJson<T>('POST', path)
}

export function signIn(
  provider: string,
  redirect = '/',
): Promise<Me | { redirect_url: string } | null> {
  return sendJson<Me | { redirect_url: string }>(
    'POST',
    `/auth/${encodeURIComponent(provider)}/start`,
    { redirect },
  )
}

export async function signOut(): Promise<void> {
  await post('/auth/logout')
}

export interface PendingLink {
  provider: string
  existing: string[]
  expires_at: string
}

export async function getPendingLink(): Promise<PendingLink | null> {
  try {
    return (await getJson<PendingLink>('/auth/link')).data
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null
    throw e
  }
}

export async function cancelPendingLink(): Promise<void> {
  await sendJson('DELETE', '/auth/link')
}
