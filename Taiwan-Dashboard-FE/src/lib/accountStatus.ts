import { resolvedTheme } from '../theme'

export const STATUS_LABELS = { active: '使用中', idle: '閒置', revoked: '已停用' } as const
export const STATUS_ICONS = { active: 'check_circle', idle: 'schedule', revoked: 'block' } as const

const LIGHT = { active: '#2a78d6', idle: '#eda100', revoked: '#6250d6' }
const DARK = { active: '#3987e5', idle: '#c98500', revoked: '#9085e9' }

export function statusColor(s: keyof typeof STATUS_LABELS): string {
  return (resolvedTheme.value === 'dark' ? DARK : LIGHT)[s]
}

export function seriesColor(): string {
  return resolvedTheme.value === 'dark' ? DARK.active : LIGHT.active
}
