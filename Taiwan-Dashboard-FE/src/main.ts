import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { i18n } from './i18n'
import { handleAuthPopup } from './lib/authPopup'
import { router } from './router'
import { reloadForNewBuild } from './lib/staleBuild'
import { initTheme } from './theme'
import { watchNewWindowLinks } from './lib/newWindow'
import { startPresence } from './lib/presence'
import './style.css'

initTheme()
window.addEventListener('vite:preloadError', (e) => {
  if (reloadForNewBuild()) e.preventDefault()
})
if (!handleAuthPopup()) {
  const app = createApp(App).use(createPinia()).use(router).use(i18n)
  void router.isReady().then(() => {
    app.mount('#app')
    watchNewWindowLinks()
    const path = window.location.pathname
    if (!path.startsWith('/embed/') && !path.startsWith('/oauth/')) startPresence()
  })
}
