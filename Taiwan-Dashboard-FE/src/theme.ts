import { computed, ref } from 'vue'

export type ThemeChoice = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'dashboardtw:theme'

const choice = ref<ThemeChoice>(readChoice())
const systemDark = ref(prefersDark())

if (typeof window !== 'undefined' && window.matchMedia) {
  window
    .matchMedia('(prefers-color-scheme: dark)')
    .addEventListener('change', (e) => (systemDark.value = e.matches))
}

export const resolvedTheme = computed<'light' | 'dark'>(() =>
  choice.value === 'system' ? (systemDark.value ? 'dark' : 'light') : choice.value,
)

export const themeChoice = computed(() => choice.value)

export function setTheme(next: ThemeChoice) {
  choice.value = next
  apply(next)
  try {
    if (next === 'system') localStorage.removeItem(STORAGE_KEY)
    else localStorage.setItem(STORAGE_KEY, next)
  } catch {}
}

export function cycleTheme() {
  setTheme(choice.value === 'system' ? 'light' : choice.value === 'light' ? 'dark' : 'system')
}

export function initTheme() {
  apply(choice.value)
}

export function forceTheme(theme: 'light' | 'dark') {
  choice.value = theme
  apply(theme)
}

function apply(c: ThemeChoice) {
  if (typeof document === 'undefined') return
  if (c === 'system') document.documentElement.removeAttribute('data-theme')
  else document.documentElement.setAttribute('data-theme', c)
}

function readChoice(): ThemeChoice {
  try {
    const v = localStorage.getItem(STORAGE_KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {}
  return 'system'
}

function prefersDark(): boolean {
  return (
    typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-color-scheme: dark)').matches
  )
}
