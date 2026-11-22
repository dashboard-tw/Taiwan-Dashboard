const CHANNEL = 'dashboardtw-auth'
const MARK = 'dashboardtw:auth-popup'
export const POPUP_PATH = '/auth/popup'

export type PopupStart =
  { mode: 'login'; provider: string; redirect: string } | { mode: 'link'; provider: string }

export type PopupResult = { url: string } | { error: string }

export function openAuthPopup(start: PopupStart): Window | null {
  const q = new URLSearchParams({ mode: start.mode, provider: start.provider })
  if (start.mode === 'login') q.set('redirect', start.redirect)
  const w = 480
  const h = 680
  const left = Math.max(0, Math.round(window.screenX + (window.outerWidth - w) / 2))
  const top = Math.max(0, Math.round(window.screenY + (window.outerHeight - h) / 2))
  return window.open(
    `${POPUP_PATH}?${q}`,
    'dashboardtw-auth',
    `popup=yes,width=${w},height=${h},left=${left},top=${top}`,
  )
}

export function waitForAuthPopup(signal: AbortSignal): Promise<PopupResult> {
  return new Promise((resolve, reject) => {
    const ch = new BroadcastChannel(CHANNEL)
    const done = () => ch.close()
    ch.onmessage = (e: MessageEvent<PopupResult>) => {
      done()
      resolve(e.data)
    }
    signal.addEventListener('abort', () => {
      done()
      reject(new DOMException('cancelled', 'AbortError'))
    })
  })
}

function report(result: PopupResult) {
  const ch = new BroadcastChannel(CHANNEL)
  ch.postMessage(result)
  ch.close()
}

function showMessage(text: string) {
  const el = document.getElementById('app')
  if (!el) return
  el.textContent = ''
  const p = document.createElement('p')
  p.textContent = text
  p.style.cssText = 'margin:3rem auto;max-width:20rem;text-align:center;font:15px/1.7 sans-serif'
  el.append(p)
}

export function handleAuthPopup(): boolean {
  if (typeof window === 'undefined' || typeof BroadcastChannel === 'undefined') return false
  const path = window.location.pathname
  let marked = false
  try {
    marked = sessionStorage.getItem(MARK) === '1'
  } catch {
    marked = false
  }

  if (path === POPUP_PATH) {
    try {
      sessionStorage.setItem(MARK, '1')
    } catch {}
    showMessage('正在前往登入頁面…')
    void startFromPopup()
    return true
  }
  if (!marked) return false

  try {
    sessionStorage.removeItem(MARK)
  } catch {}
  report({ url: window.location.pathname + window.location.search + window.location.hash })
  showMessage('登入完成，可以關閉這個視窗。')
  window.close()
  return true
}

async function startFromPopup() {
  const q = new URLSearchParams(window.location.search)
  const provider = q.get('provider') ?? ''
  try {
    let url: string | null = null
    if (q.get('mode') === 'link') {
      const { startLinkProvider } = await import('../api/me')
      url = await startLinkProvider(provider)
    } else {
      const { signIn } = await import('../api/auth')
      const r = await signIn(provider, q.get('redirect') ?? '/')
      url = r && 'redirect_url' in r ? r.redirect_url : null
    }
    if (!url) throw new Error('no redirect')
    window.location.assign(url)
  } catch (e) {
    try {
      sessionStorage.removeItem(MARK)
    } catch {}
    const message =
      e instanceof Error && e.message !== 'no redirect' ? e.message : '登入失敗，請再試一次。'
    report({ error: message })
    showMessage(message)
    window.close()
  }
}
