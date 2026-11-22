import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { handleAuthPopup, openAuthPopup, waitForAuthPopup } from './authPopup'

const MARK = 'dashboardtw:auth-popup'

function listen(): Promise<unknown> {
  return new Promise((resolve) => {
    const ch = new BroadcastChannel('dashboardtw-auth')
    ch.onmessage = (e) => {
      ch.close()
      resolve(e.data)
    }
  })
}

beforeEach(() => {
  document.body.innerHTML = '<div id="app"></div>'
  sessionStorage.clear()
})
afterEach(() => {
  vi.restoreAllMocks()
  window.history.replaceState(null, '', '/')
})

describe('openAuthPopup', () => {
  it('opens our own popup page, sized like a sign-in window', () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    openAuthPopup({ mode: 'login', provider: 'github', redirect: '/aqi/63000' })
    const [url, name, features] = open.mock.calls[0]!
    expect(url).toBe('/auth/popup?mode=login&provider=github&redirect=%2Faqi%2F63000')
    expect(name).toBe('dashboardtw-auth')
    expect(features).toContain('popup=yes')
    expect(features).toContain('width=480')
  })

  it('reports a blocked popup as null so the caller can fall back', () => {
    vi.spyOn(window, 'open').mockReturnValue(null)
    expect(openAuthPopup({ mode: 'link', provider: 'google' })).toBeNull()
  })
})

describe('handleAuthPopup', () => {
  it('leaves ordinary pages alone', () => {
    window.history.replaceState(null, '', '/aqi')
    expect(handleAuthPopup()).toBe(false)
  })

  it('sends the landing page back to the opener and closes', async () => {
    const close = vi.spyOn(window, 'close').mockImplementation(() => {})
    sessionStorage.setItem(MARK, '1')
    window.history.replaceState(null, '', '/login?error=cancelled')
    const got = listen()
    expect(handleAuthPopup()).toBe(true)
    await expect(got).resolves.toEqual({ url: '/login?error=cancelled' })
    expect(close).toHaveBeenCalled()
    expect(sessionStorage.getItem(MARK)).toBeNull()
    expect(document.getElementById('app')!.textContent).toContain('登入完成')
  })
})

describe('waitForAuthPopup', () => {
  it('resolves with what the popup sends', async () => {
    const wait = waitForAuthPopup(new AbortController().signal)
    const ch = new BroadcastChannel('dashboardtw-auth')
    ch.postMessage({ url: '/me' })
    ch.close()
    await expect(wait).resolves.toEqual({ url: '/me' })
  })

  it('stops waiting when cancelled', async () => {
    const ac = new AbortController()
    const wait = waitForAuthPopup(ac.signal)
    ac.abort()
    await expect(wait).rejects.toThrow('cancelled')
  })
})
