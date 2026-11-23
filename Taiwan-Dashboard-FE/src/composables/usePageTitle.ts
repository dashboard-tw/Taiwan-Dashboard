import { toValue, watchEffect, type MaybeRefOrGetter } from 'vue'

export function usePageTitle(title: MaybeRefOrGetter<string>) {
  if (typeof document === 'undefined') return
  watchEffect(() => {
    const t = toValue(title)
    document.title = t ? `${t}｜臺灣儀表板` : '臺灣儀表板｜dashboard.tw'
  })
}
