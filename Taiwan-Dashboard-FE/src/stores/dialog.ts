import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ComponentInfo } from '../api/types'

interface Target {
  component: ComponentInfo
  county: string | null
}

export const useDialogs = defineStore('dialogs', () => {
  const info = ref<Target | null>(null)
  const target = ref<Target | null>(null)
  const download = ref(false)
  const embed = ref(false)

  function openInfo(component: ComponentInfo, county: string | null) {
    info.value = { component, county }
  }
  function openDownload(component: ComponentInfo, county: string | null) {
    target.value = { component, county }
    download.value = true
  }
  function openEmbed(component: ComponentInfo, county: string | null) {
    target.value = { component, county }
    embed.value = true
  }

  const login = ref<{ redirect: string; error?: string } | null>(null)
  function openLogin(redirect = '/', error?: string) {
    const safe = redirect.startsWith('/') && !redirect.startsWith('//') ? redirect : '/'
    login.value = { redirect: safe, error }
  }
  function closeLogin() {
    login.value = null
  }

  const intro = ref(false)
  const INTRO_KEY = 'dashboardtw:intro-hidden'
  function introHidden(): boolean {
    try {
      return localStorage.getItem(INTRO_KEY) === '1'
    } catch {
      return false
    }
  }
  function showIntroOnce() {
    if (!introHidden()) intro.value = true
  }
  function dismissIntro() {
    try {
      localStorage.setItem(INTRO_KEY, '1')
    } catch {}
  }

  function closeAll() {
    info.value = null
    download.value = false
    embed.value = false
    login.value = null
    intro.value = false
  }

  return {
    info,
    target,
    download,
    embed,
    login,
    openInfo,
    openDownload,
    openEmbed,
    openLogin,
    closeLogin,
    intro,
    showIntroOnce,
    dismissIntro,
    closeAll,
  }
})
