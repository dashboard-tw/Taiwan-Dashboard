import { API_BASE, ApiError, getJson, sendJson } from './client'

export type AppCategory = 'app' | 'web' | 'innovation'

export interface AppFields {
  category: AppCategory
  name: string
  description: string
  web_url: string
  ios_url: string
  android_url: string
  topics: string[]
  show_author: boolean
}

export interface AppContent extends AppFields {
  image: string
  screenshot: string
}

export type AppStatus = 'pending' | 'approved' | 'rejected' | 'removed'

export interface MyApp extends AppContent {
  id: number
  status: AppStatus
  review_note: string
  published: AppContent | null
  published_at: string | null
  created_at: string
  updated_at: string
}

export interface PublicApp {
  id: number
  category: AppCategory
  name: string
  description: string
  web_url: string
  ios_url: string
  android_url: string
  image_url: string | null
  screenshot_url: string | null
  topics: string[]
  author: string | null
  published_at: string
}

export const APP_CATEGORIES: { value: AppCategory; label: string; icon: string; hint: string }[] = [
  { value: 'app', label: 'App', icon: 'smartphone', hint: 'iOS 或 Android App' },
  { value: 'web', label: '網頁應用', icon: 'language', hint: '網站、網頁工具' },
  {
    value: 'innovation',
    label: '創意應用',
    icon: 'lightbulb',
    hint: '研究報告、新聞報導、資料視覺化、聊天機器人',
  },
]

export function categoryLabel(c: string): string {
  return APP_CATEGORIES.find((x) => x.value === c)?.label ?? c
}

export const MAX_APP_IMAGE_BYTES = 2 * 1024 * 1024

export const APP_STATUS: Record<AppStatus, string> = {
  pending: '審核中',
  approved: '已通過',
  rejected: '未通過',
  removed: '已下架',
}

export function appImageUrl(image: string): string | null {
  return image ? `${API_BASE}/app-images/${image}` : null
}

export async function getPublicApps(): Promise<PublicApp[]> {
  return (await getJson<PublicApp[]>('/apps')).data
}

export async function getMyApps(): Promise<{ apps: MyApp[] }> {
  const r = await getJson<MyApp[]>('/me/apps', { credentials: 'same-origin' })
  return { apps: r.data }
}

export async function createApp(f: AppFields): Promise<MyApp[]> {
  return (await sendJson<MyApp[]>('POST', '/me/apps', { ...f, agree_rules: true })) ?? []
}

export async function updateApp(id: number, f: AppFields): Promise<MyApp[]> {
  return (await sendJson<MyApp[]>('PUT', `/me/apps/${id}`, { ...f, agree_rules: true })) ?? []
}

export async function deleteApp(id: number): Promise<void> {
  await sendJson('DELETE', `/me/apps/${id}`)
}

export async function uploadAppImage(
  id: number,
  file: Blob,
  kind: 'image' | 'screenshot' = 'image',
): Promise<MyApp[]> {
  const res = await fetch(`${API_BASE}/me/apps/${id}/${kind}`, {
    method: 'PUT',
    headers: {
      Accept: 'application/json',
      'Content-Type': file.type || 'application/octet-stream',
    },
    body: file,
    credentials: 'same-origin',
  })
  const body = (await res.json().catch(() => null)) as
    { data: MyApp[] } | { error: { code: string; message: string } } | null
  if (!res.ok || !body || !('data' in body)) {
    const err =
      body && 'error' in body ? body.error : { code: 'http_error', message: '圖片上傳失敗' }
    throw new ApiError(res.status, err.code, err.message)
  }
  return body.data
}
