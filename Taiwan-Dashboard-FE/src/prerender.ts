import { createSSRApp, type Component } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { renderToString } from 'vue/server-renderer'
import PrivacyView from './views/PrivacyView.vue'
import TermsView from './views/TermsView.vue'

export const pages: Record<string, Component> = {
  privacy: PrivacyView,
  terms: TermsView,
}

export async function render(view: Component, path: string): Promise<string> {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:p(.*)*', component: { render: () => null } }],
  })
  const app = createSSRApp(view).use(router)
  await router.push(path)
  await router.isReady()
  return renderToString(app)
}
