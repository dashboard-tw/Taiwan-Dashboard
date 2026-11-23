const KEY = 'dashboardtw:stale-reload'

function recentlyReloaded(): boolean {
  try {
    const at = Number(sessionStorage.getItem(KEY) ?? 0)
    return Date.now() - at < 10_000
  } catch {
    return false
  }
}

export function reloadForNewBuild(path?: string) {
  if (recentlyReloaded()) return false
  try {
    sessionStorage.setItem(KEY, String(Date.now()))
  } catch {}
  if (path) window.location.assign(path)
  else window.location.reload()
  return true
}

export function isChunkError(err: unknown): boolean {
  const msg = err instanceof Error ? err.message : String(err)
  return /dynamically imported module|Importing a module script failed|Unable to preload CSS|error loading dynamically imported module/i.test(
    msg,
  )
}
