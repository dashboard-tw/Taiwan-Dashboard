<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAuth } from '../../stores/auth'
import { useDialogs } from '../../stores/dialog'
import { cycleTheme, themeChoice } from '../../theme'
import TaiwanLogo from './TaiwanLogo.vue'

defineEmits<{ toggleSidebar: [] }>()

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuth()
const dialogs = useDialogs()
void auth.load()

const menuOpen = ref(false)
const menuEl = ref<HTMLElement | null>(null)
function closeMenu(e: MouseEvent) {
  if (menuEl.value && !menuEl.value.contains(e.target as Node)) menuOpen.value = false
}
onMounted(() => document.addEventListener('click', closeMenu))
onBeforeUnmount(() => document.removeEventListener('click', closeMenu))
async function logout() {
  menuOpen.value = false
  await auth.logout()
  if (route.meta.admin || route.meta.auth) void router.push('/')
}

const topic = computed(() => (typeof route.params.topic === 'string' ? route.params.topic : null))
const county = computed(() =>
  typeof route.params.county === 'string' ? route.params.county : null,
)
const base = computed(() => `/${topic.value ?? 'weather'}${county.value ? `/${county.value}` : ''}`)
const isMap = computed(() => route.path.endsWith('/map'))
const onDashboard = computed(
  () =>
    route.path === '/' ||
    !!topic.value ||
    String(route.name ?? '').startsWith('typhoon-event') ||
    route.path.startsWith('/county/'),
)
const section = computed(() =>
  route.path.startsWith('/developers')
    ? 'developers'
    : route.path.startsWith('/showcase')
      ? 'showcase'
      : null,
)

const themeLabel = computed(
  () =>
    ({ system: t('theme.system'), light: t('theme.light'), dark: t('theme.dark') })[
      themeChoice.value
    ],
)
</script>

<template>
  <header class="site-header">
    <button
      type="button"
      class="icon-btn menu-toggle"
      :aria-label="t('nav.menu')"
      @click="$emit('toggleSidebar')"
    >
      <span class="mi">menu</span>
    </button>

    <RouterLink to="/" class="brand" :aria-label="`${t('site.title')} 首頁`">
      <TaiwanLogo class="logo" />
      <span class="names">
        <span class="name-zh">{{ t('site.title') }}</span>
        <span class="name-en">{{ t('site.nameEn') }}</span>
      </span>
    </RouterLink>

    <nav class="tabs" aria-label="顯示模式">
      <RouterLink
        :to="route.path === '/' ? '/' : base"
        :aria-current="onDashboard && !isMap ? 'page' : undefined"
        >{{ t('header.overview') }}</RouterLink
      >
      <RouterLink :to="`${base}/map`" :aria-current="onDashboard && isMap ? 'page' : undefined">{{
        t('header.mapMode')
      }}</RouterLink>
    </nav>

    <div class="tools">
      <RouterLink
        to="/developers"
        class="text-link"
        :class="{ on: section === 'developers' }"
        :aria-current="section === 'developers' ? 'page' : undefined"
        :title="t('header.api')"
        ><span class="mi" aria-hidden="true">code</span
        ><span class="label">{{ t('header.api') }}</span></RouterLink
      >
      <RouterLink
        to="/showcase/app"
        class="text-link"
        :class="{ on: section === 'showcase' }"
        :aria-current="section === 'showcase' ? 'page' : undefined"
        :title="t('header.showcase')"
        ><span class="mi" aria-hidden="true">apps</span
        ><span class="label">{{ t('header.showcase') }}</span></RouterLink
      >
      <button
        type="button"
        class="icon-btn"
        :title="`${t('theme.label')}：${themeLabel}`"
        :aria-label="`${t('theme.label')}：${themeLabel}`"
        @click="cycleTheme"
      >
        <span class="mi">contrast</span>
      </button>
      <button
        type="button"
        class="icon-btn"
        title="使用說明"
        aria-label="使用說明"
        @click="dialogs.intro = true"
      >
        <span class="mi">help</span>
      </button>
      <RouterLink to="/about" class="icon-btn" :title="t('nav.about')" :aria-label="t('nav.about')">
        <span class="mi">info</span>
      </RouterLink>
      <div v-if="auth.me" ref="menuEl" class="user">
        <button
          type="button"
          class="user-btn"
          :title="auth.me.display_name"
          :aria-expanded="menuOpen"
          aria-haspopup="menu"
          @click="menuOpen = !menuOpen"
        >
          <span class="mi">account_circle</span
          ><span class="user-name">{{ auth.me.display_name }}</span>
        </button>
        <ul v-if="menuOpen" class="user-menu" role="menu">
          <li role="none">
            <RouterLink role="menuitem" to="/me/api-key" @click="menuOpen = false">
              <span class="mi">key</span>API 憑證
            </RouterLink>
          </li>
          <li role="none">
            <RouterLink role="menuitem" to="/me/showcase" @click="menuOpen = false">
              <span class="mi">apps</span>申請應用活化
            </RouterLink>
          </li>
          <li role="none">
            <RouterLink role="menuitem" to="/me" @click="menuOpen = false">
              <span class="mi">manage_accounts</span>我的帳號
            </RouterLink>
          </li>
          <li role="none">
            <RouterLink role="menuitem" to="/me/devices" @click="menuOpen = false">
              <span class="mi">devices</span>登入裝置
            </RouterLink>
          </li>
          <li v-if="auth.isAdmin" role="none">
            <RouterLink role="menuitem" to="/admin" @click="menuOpen = false">
              <span class="mi">admin_panel_settings</span>後台
            </RouterLink>
          </li>
          <li role="none">
            <button role="menuitem" type="button" @click="logout">
              <span class="mi">logout</span>登出
            </button>
          </li>
        </ul>
      </div>
      <button
        v-else-if="auth.loaded"
        type="button"
        class="login-link"
        aria-haspopup="dialog"
        @click="dialogs.openLogin(route.fullPath)"
      >
        登入
      </button>
    </div>
  </header>
</template>

<style scoped>
.site-header {
  position: sticky;
  top: 0;
  z-index: 50;
  display: flex;
  align-items: center;
  gap: 0.6rem;
  height: var(--header-h);
  padding: 0 1rem;
  background: var(--header-bg);
  color: #ffffff;
  border-bottom: 1px solid var(--header-border);
}
.brand {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 0.6rem;
  color: inherit;
  text-decoration: none;
}
.logo {
  width: 22px;
  height: 40px;
}
.names {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
}
.name-zh {
  font-size: 1.2rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  white-space: nowrap;
}
.name-en {
  font-size: 0.75rem;
  color: #c3c3c3;
}
.tabs {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  gap: 1.25rem;
  height: 100%;
}
.tabs a {
  display: flex;
  align-items: center;
  padding: 0 0.25rem;
  color: #ffffff;
  text-decoration: none;
  border-bottom: 2px solid transparent;
}
.tabs a[aria-current='page'] {
  color: var(--header-accent);
  border-bottom-color: var(--header-accent);
}
.tabs a:hover {
  color: var(--header-accent);
}
.tools {
  display: flex;
  min-width: 0;
  gap: 0.25rem;
  margin-left: auto;
}
.text-link {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  gap: 4px;
  height: 2.4rem;
  padding: 0 0.6rem;
  border-radius: 8px;
  color: #ffffff;
  font-size: var(--font-ms);
  text-decoration: none;
  white-space: nowrap;
}
.text-link .mi {
  font-size: 1.15rem;
}
.text-link:hover {
  background: rgb(255 255 255 / 10%);
}
.text-link.on {
  color: var(--header-accent);
}
@media (max-width: 1199px) {
  .text-link .label {
    display: none;
  }
  .text-link {
    justify-content: center;
    width: 2.4rem;
    padding: 0;
  }
}
.icon-btn {
  display: grid;
  flex-shrink: 0;
  place-items: center;
  width: 2.4rem;
  height: 2.4rem;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #ffffff;
  font-size: 1.15rem;
  cursor: pointer;
}
.icon-btn:hover {
  background: rgb(255 255 255 / 10%);
}
.user {
  position: relative;
  min-width: 0;
}
.user-btn {
  max-width: min(14rem, 100%);
  min-width: 0;
}
.user-btn .mi {
  flex-shrink: 0;
}
.user-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.user-btn,
.login-link {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 2.4rem;
  padding: 0 0.6rem;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: #ffffff;
  font-size: var(--font-ms);
  text-decoration: none;
  cursor: pointer;
}
.login-link {
  flex-shrink: 0;
  font-size: var(--font-m);
  white-space: nowrap;
}
.user-btn:hover,
.login-link:hover {
  background: rgb(255 255 255 / 10%);
}
.user-menu {
  position: absolute;
  right: 0;
  top: calc(100% + 4px);
  min-width: 9rem;
  margin: 0;
  padding: 4px;
  list-style: none;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  box-shadow: 0 0 5px rgb(0 0 0 / 50%);
}
.user-menu a,
.user-menu button {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 8px;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-primary);
  font-size: 0.925rem;
  text-decoration: none;
  cursor: pointer;
}
.user-menu a:hover,
.user-menu button:hover {
  background: var(--surface-muted);
}
.menu-toggle {
  display: none;
  margin-left: -0.5rem;
}
@media (max-width: 999px) {
  .menu-toggle {
    display: grid;
  }
}
@media (max-width: 719px) {
  .tabs,
  .name-en,
  .text-link {
    display: none;
  }
  .name-zh {
    font-size: 1.05rem;
  }
}
</style>
