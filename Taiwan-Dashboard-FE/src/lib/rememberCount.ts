const PREFIX = 'dashboardtw:rows:'

export function rememberedCount(key: string, fallback: number): number {
  try {
    const n = Number(localStorage.getItem(PREFIX + key))
    return Number.isInteger(n) && n > 0 ? Math.min(n, 60) : fallback
  } catch {
    return fallback
  }
}

export function rememberCount(key: string, n: number) {
  try {
    localStorage.setItem(PREFIX + key, String(n))
  } catch {}
}
