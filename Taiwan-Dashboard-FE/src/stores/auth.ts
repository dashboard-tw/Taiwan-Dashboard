import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { getMe, signIn, signOut, type Me } from '../api/auth'

export const useAuth = defineStore('auth', () => {
  const me = ref<Me | null>(null)
  const loaded = ref(false)
  let loading: Promise<void> | null = null

  const isAdmin = computed(() => me.value?.role === 'admin')

  function load(force = false): Promise<void> {
    if (loaded.value && !force) return Promise.resolve()
    if (!loading) {
      loading = getMe()
        .then((u) => {
          me.value = u
        })
        .catch(() => {
          me.value = null
        })
        .finally(() => {
          loaded.value = true
          loading = null
        })
    }
    return loading
  }

  async function login(provider: string, redirect = '/'): Promise<boolean> {
    const r = await signIn(provider, redirect)
    if (r && 'redirect_url' in r) {
      window.location.assign(r.redirect_url)
      return false
    }
    me.value = r
    loaded.value = true
    return true
  }

  async function logout() {
    try {
      await signOut()
    } finally {
      me.value = null
    }
  }

  return { me, loaded, isAdmin, load, login, logout }
})
