// 頁尾的瀏覽人次與在線人數。
// 每次開啟網站算一次瀏覽（同一個分頁切換頁面、重新整理都不重複計算），
// 開著網頁時每 2 分鐘回報一次在線。訪客代碼是隨機字串，不含任何個人資料。
import { ref } from 'vue'
import { API_BASE } from '../api/client'

export const siteStats = ref<{ visits: number; online: number } | null>(null)

const ID_KEY = 'dashboardtw:visitor'
const VISIT_KEY = 'dashboardtw:visited'
const BEAT_MS = 2 * 60_000

function read(store: Storage, key: string): string | null {
  try {
    return store.getItem(key)
  } catch {
    return null
  }
}

function write(store: Storage, key: string, value: string) {
  try {
    store.setItem(key, value)
  } catch {}
}

function visitorId(): string {
  let id = read(localStorage, ID_KEY)
  if (!id || !/^[A-Za-z0-9_-]{8,40}$/.test(id)) {
    id = crypto.randomUUID().replace(/-/g, '')
    write(localStorage, ID_KEY, id)
  }
  return id
}

async function report(id: string, isNew: boolean) {
  try {
    const res = await fetch(`${API_BASE}/presence`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ id, new: isNew }),
    })
    if (!res.ok) return
    const body = (await res.json()) as { data?: { visits: number; online: number } }
    if (body.data) siteStats.value = body.data
  } catch {}
}

export function startPresence() {
  const id = visitorId()
  const isNew = read(sessionStorage, VISIT_KEY) === null
  write(sessionStorage, VISIT_KEY, '1')
  void report(id, isNew)
  setInterval(() => {
    if (document.visibilityState === 'visible') void report(id, false)
  }, BEAT_MS)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') void report(id, false)
  })
}
