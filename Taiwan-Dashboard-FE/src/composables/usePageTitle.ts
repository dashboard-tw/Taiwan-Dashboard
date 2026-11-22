import { toValue, watchEffect, type MaybeRefOrGetter } from 'vue'

export function usePageTitle(title: MaybeRefOrGetter<string>) {
  watchEffect(() => {
    const t = toValue(title)
    document.title = t ? `${t}｜臺灣儀表板` : '臺灣全國儀表板｜dashboard.tw'
  })
}
