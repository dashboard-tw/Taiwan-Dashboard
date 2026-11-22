const TZ = 'Asia/Taipei'

export function formatNumber(v: number | null | undefined, decimals = 0): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '—'
  return v.toLocaleString('zh-TW', {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  })
}

function parts(iso: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const p of new Intl.DateTimeFormat('zh-TW', {
    timeZone: TZ,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(new Date(iso))) {
    out[p.type] = p.value
  }
  return out
}

export function formatObserved(iso: string | null | undefined, now = new Date()): string {
  if (!iso) return '—'
  const p = parts(iso)
  const today = parts(now.toISOString())
  const time = `${p.hour}:${p.minute}`
  if (p.year === today.year && p.month === today.month && p.day === today.day) return time
  if (p.year !== today.year) return `${p.year}/${Number(p.month)}/${Number(p.day)}`
  if (time === '00:00') return `${Number(p.month)}/${Number(p.day)}`
  return `${Number(p.month)}/${Number(p.day)} ${time}`
}

export function formatFreq(seconds: number | undefined): string {
  if (!seconds) return ''
  const m = seconds / 60
  if (m < 60) return `每${m}分鐘更新`
  const h = m / 60
  if (h < 24) return `每${h}小時更新`
  const d = h / 24
  if (d === 1) return '每天更新'
  if (d === 7) return '每週更新'
  if (d >= 28 && d <= 31) return '每月更新'
  if (d >= 360) return '每年更新'
  return `每${d}天更新`
}

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const p = parts(iso)
  return `${p.year}/${p.month}/${p.day}`
}

export function formatAxisTime(iso: string, step: string | undefined): string {
  const p = parts(iso)
  switch (step) {
    case 'hour':
      return `${p.hour}:${p.minute}`
    case 'month':
      return `${p.year}/${p.month}`
    case 'year':
      return `${p.year}`
    default:
      return `${Number(p.month)}/${Number(p.day)}`
  }
}
