import type { TyphoonEvent } from '../api/client'

export function typhoonName(t: TyphoonEvent): string {
  const name = t.name_zh ?? t.name_en ?? (t.ty_no ? `第 ${t.ty_no} 號` : t.id)
  return `${name}颱風`
}

function md(iso: string): string {
  const p = new Intl.DateTimeFormat('zh-TW', {
    timeZone: 'Asia/Taipei',
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).formatToParts(new Date(iso))
  const get = (type: string) => p.find((x) => x.type === type)?.value ?? ''
  return `${get('month')}/${get('day')} ${get('hour')}:${get('minute')}`
}

export function typhoonPeriod(t: TyphoonEvent): string {
  const year = new Date(t.starts_at).getFullYear()
  const start = `${year}/${md(t.starts_at)}`
  return t.ends_at ? `${start} – ${md(t.ends_at)}` : `${start} 起（警報中）`
}
