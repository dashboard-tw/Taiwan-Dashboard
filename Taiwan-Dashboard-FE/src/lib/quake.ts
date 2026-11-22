import { QUAKE_INTENSITY_LEVELS, type Level } from './levels'

export function quakePlace(location: string | null): string {
  if (!location) return '—'
  const m = /位於([^)）]+)/.exec(location)
  return m ? m[1]!.trim() : location
}

export function quakeTime(iso: string): string {
  return new Date(iso).toLocaleString('zh-TW', {
    timeZone: 'Asia/Taipei',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

export function intensityLevel(name: string | null): Level | null {
  if (!name) return null
  return QUAKE_INTENSITY_LEVELS.find((l) => l.label === name.trim()) ?? null
}

export const QUAKE_KINDS: Record<string, string> = {
  significant: '顯著有感',
  local: '小區域有感',
}
