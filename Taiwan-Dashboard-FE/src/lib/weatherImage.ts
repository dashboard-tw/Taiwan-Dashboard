import { API_BASE } from '../api/client'

export function imageFileName(iso: string): string {
  const d = new Date(iso).toISOString() // 2026-10-22T09:30:00.000Z
  return `${d.slice(0, 4)}${d.slice(5, 7)}${d.slice(8, 10)}T${d.slice(11, 13)}${d.slice(14, 16)}Z.jpg`
}

export function imageUrl(product: string, iso: string): string {
  return `${API_BASE}/images/${encodeURIComponent(product)}/${imageFileName(iso)}`
}
