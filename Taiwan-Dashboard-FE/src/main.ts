import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { i18n } from './i18n'
import { handleAuthPopup } from './lib/authPopup'
import { router } from './router'
import { initTheme } from './theme'
import './style.css'

initTheme()
if (!handleAuthPopup()) {
  const app = createApp(App).use(createPinia()).use(router).use(i18n)
  void router.isReady().then(() => app.mount('#app'))
}
