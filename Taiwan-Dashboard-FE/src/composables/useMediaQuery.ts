import { onBeforeUnmount, ref } from 'vue'

export function useMediaQuery(query: string) {
  const mql = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia(query) : null
  const matches = ref(mql?.matches ?? false)
  const onChange = (e: MediaQueryListEvent) => (matches.value = e.matches)
  mql?.addEventListener('change', onChange)
  onBeforeUnmount(() => mql?.removeEventListener('change', onChange))
  return matches
}
