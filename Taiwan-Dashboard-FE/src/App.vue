<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import SiteFooter from './components/layout/SiteFooter.vue'
import SiteHeader from './components/layout/SiteHeader.vue'
import RefreshClock from './components/layout/RefreshClock.vue'
import { restartCountdown } from './composables/useRefreshClock'
import { refreshMs } from './lib/topics'
import SiteSidebar from './components/layout/SiteSidebar.vue'
import ComponentInfoDialog from './components/dialogs/ComponentInfoDialog.vue'
import DownloadDialog from './components/dialogs/DownloadDialog.vue'
import EmbedDialog from './components/dialogs/EmbedDialog.vue'
import IntroDialog from './components/dialogs/IntroDialog.vue'
import LoginDialog from './components/dialogs/LoginDialog.vue'
import { useDialogs } from './stores/dialog'
import { useMediaQuery } from './composables/useMediaQuery'
import { prefetchDashboards } from './api/client'

const route = useRoute()
const bare = computed(() => route.meta.bare === true)

const narrow = useMediaQuery('(max-width: 999px)')
const COLLAPSED_KEY = 'dashboardtw:sidebar-collapsed'
const collapsed = ref(readCollapsed())
const drawerOpen = ref(false)

function readCollapsed(): boolean {
  try {
    return localStorage.getItem(COLLAPSED_KEY) === '1'
  } catch {
    return false
  }
}

function toggleCollapsed() {
  collapsed.value = !collapsed.value
  try {
    localStorage.setItem(COLLAPSED_KEY, collapsed.value ? '1' : '0')
  } catch {}
}

const dialogs = useDialogs()
onMounted(() => {
  if (bare.value || route.path.startsWith('/admin')) return
  dialogs.showIntroOnce()
})

onMounted(() => {
  if (bare.value) return
  const idle = window.requestIdleCallback ?? ((cb: () => void) => setTimeout(cb, 1500))
  idle(() => void prefetchDashboards())
})

watch(
  () => route.fullPath,
  () => (drawerOpen.value = false),
)

watch(
  () => route.path,
  () => restartCountdown(refreshMs(route.params.topic as string | undefined)),
  { immediate: true },
)
</script>

<template>
  <LoginDialog v-if="!route.path.startsWith('/embed/')" />
  <IntroDialog v-if="!bare" />
  <RouterView v-if="bare" />
  <template v-else>
    <a class="skip" href="#main">跳到主要內容</a>
    <SiteHeader @toggle-sidebar="drawerOpen = !drawerOpen" />
    <div class="shell">
      <SiteSidebar
        :collapsed="collapsed && !narrow"
        :open="drawerOpen && narrow"
        @toggle-collapsed="toggleCollapsed"
        @close="drawerOpen = false"
      />
      <div class="content">
        <main id="main">
          <RouterView />
        </main>
        <SiteFooter v-if="!route.path.endsWith('/map')" />
      </div>
    </div>
    <RefreshClock :raised="route.path.endsWith('/map')" />
    <ComponentInfoDialog />
    <DownloadDialog />
    <EmbedDialog />
  </template>
</template>

<style scoped>
.shell {
  display: flex;
  align-items: stretch;
}
.content {
  flex: 1;
  min-width: 0;
}
.skip {
  position: absolute;
  left: -999px;
}
.skip:focus {
  left: 1rem;
  top: 0.5rem;
  z-index: 100;
  padding: 0.4rem 0.8rem;
  background: var(--surface);
  border-radius: 6px;
}
</style>
