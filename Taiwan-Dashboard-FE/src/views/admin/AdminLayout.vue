<script setup lang="ts">
import { useRoute } from 'vue-router'
import './admin.css'

const route = useRoute()
function on(t: { to: string; exact?: boolean }) {
  return t.exact ? route.path === t.to : route.path.startsWith(t.to)
}

const TABS = [
  { to: '/admin', icon: 'space_dashboard', label: '總覽', exact: true },
  { to: '/admin/pipeline', icon: 'monitor_heart', label: '資料管線' },
  { to: '/admin/components', icon: 'widgets', label: '元件' },
  { to: '/admin/reservoirs', icon: 'water_drop', label: '水庫主檔' },
  { to: '/admin/power-units', icon: 'bolt', label: '發電機組' },
  { to: '/admin/users', icon: 'group', label: '用戶' },
  { to: '/admin/apps', icon: 'apps', label: '應用審核' },
  { to: '/admin/api-usage', icon: 'query_stats', label: 'API 用量' },
  { to: '/admin/api-survey', icon: 'poll', label: 'API 使用調查' },
  { to: '/admin/mail', icon: 'mail', label: '信件模板' },
  { to: '/admin/channels', icon: 'campaign', label: '通知頻道' },
]
</script>

<template>
  <div class="page admin">
    <nav class="admin-nav" aria-label="後台">
      <RouterLink
        v-for="t in TABS"
        :key="t.to"
        :to="t.to"
        :class="{ on: on(t) }"
        :aria-current="on(t) ? 'page' : undefined"
      >
        <span class="mi" aria-hidden="true">{{ t.icon }}</span
        >{{ t.label }}
      </RouterLink>
    </nav>
    <p class="mobile-note muted">後台只支援桌機版面。</p>
    <RouterView />
  </div>
</template>

<style scoped>
.admin-nav {
  display: flex;
  gap: 4px;
  padding-top: 0.75rem;
}
.admin-nav a {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  border-radius: var(--radius);
  color: var(--text-muted);
  font-size: 0.875rem;
  text-decoration: none;
}
.admin-nav a:hover {
  color: var(--text-primary);
}
.admin-nav a.on {
  background: var(--accent-soft);
  color: var(--accent);
}
.admin-nav .mi {
  font-size: 1.1rem;
}
.mobile-note {
  display: none;
}
@media (max-width: 62.4375em) {
  .mobile-note {
    display: block;
    margin-top: 0.5rem;
  }
}
</style>
