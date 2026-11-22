<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { DASHBOARD_ICONS, TOPIC_GROUPS, TOPICS } from '../../lib/topics'
import { useCatalog } from '../../stores/catalog'
import { useTyphoonAlert } from '../../composables/useTyphoonAlert'

defineProps<{ collapsed: boolean; open: boolean }>()
defineEmits<{ toggleCollapsed: []; close: [] }>()

const { t } = useI18n()
const route = useRoute()
const catalog = useCatalog()
void catalog.load()
const typhoon = useTyphoonAlert()

const groups = TOPIC_GROUPS.map((g) => ({ ...g, topics: TOPICS.filter((tp) => tp.group === g.id) }))

function topicActive(slug: string) {
  return route.params.topic === slug
}
</script>

<template>
  <div v-if="open" class="scrim" @click="$emit('close')"></div>
  <aside :class="['sidebar', { collapsed, open }]" :aria-label="t('sidebar.label')">
    <div class="head">
      <h2 v-if="!collapsed">{{ t('sidebar.public') }}</h2>
      <button
        type="button"
        class="collapse"
        :aria-label="collapsed ? t('sidebar.expand') : t('sidebar.collapse')"
        :title="collapsed ? t('sidebar.expand') : t('sidebar.collapse')"
        @click="$emit('toggleCollapsed')"
      >
        <span class="mi">{{
          collapsed ? 'keyboard_double_arrow_right' : 'keyboard_double_arrow_left'
        }}</span>
      </button>
    </div>

    <nav v-if="typhoon.active.value" class="alert" aria-label="颱風專區">
      <RouterLink
        to="/typhoon"
        :aria-current="topicActive('typhoon') ? 'page' : undefined"
        :title="`${typhoon.title.value}（${typhoon.detail.value}）`"
      >
        <span class="mi">{{ DASHBOARD_ICONS.typhoon }}</span
        ><span class="alert-label"
          >{{ typhoon.title.value }}<small>{{ typhoon.detail.value }}</small></span
        >
      </RouterLink>
    </nav>
    <nav>
      <RouterLink
        to="/"
        :aria-current="route.path === '/' ? 'page' : undefined"
        :title="t('sidebar.home')"
      >
        <span class="mi">{{ DASHBOARD_ICONS.home }}</span
        ><span>{{ t('sidebar.home') }}</span>
      </RouterLink>
    </nav>

    <template v-for="g in groups" :key="g.id">
      <p v-if="!collapsed" class="group">{{ g.label }}</p>
      <hr v-else class="divider" />
      <nav :aria-label="g.label">
        <RouterLink
          v-for="tp in g.topics"
          :key="tp.slug"
          :to="`/${tp.slug}`"
          :aria-current="topicActive(tp.slug) ? 'page' : undefined"
          :title="tp.title"
        >
          <span class="mi">{{ DASHBOARD_ICONS[tp.slug] }}</span
          ><span>{{ tp.title }}</span>
        </RouterLink>
      </nav>
    </template>

    <hr v-if="collapsed" class="divider" />
    <nav>
      <RouterLink
        to="/mapdata/map"
        :aria-current="topicActive('mapdata') ? 'page' : undefined"
        :title="t('sidebar.mapdata')"
      >
        <span class="mi">{{ DASHBOARD_ICONS.mapdata }}</span
        ><span>{{ t('sidebar.mapdata') }}</span>
      </RouterLink>
      <RouterLink
        to="/typhoons"
        :aria-current="
          route.path === '/typhoons' || route.name === 'typhoon-event' ? 'page' : undefined
        "
        :title="t('sidebar.typhoons')"
      >
        <span class="mi">{{ DASHBOARD_ICONS.typhoons }}</span
        ><span>{{ t('sidebar.typhoons') }}</span>
      </RouterLink>
      <RouterLink
        to="/components"
        :aria-current="route.path === '/components' ? 'page' : undefined"
        :title="t('sidebar.library')"
      >
        <span class="mi">apps</span><span>{{ t('sidebar.library') }}</span>
      </RouterLink>
    </nav>

    <template v-if="!collapsed">
      <p class="group">{{ t('sidebar.counties') }}</p>
      <nav class="counties">
        <span v-for="i in catalog.counties.length ? 0 : 10" :key="`s${i}`" class="county-skeleton">
          <span class="skeleton"></span>
        </span>
        <RouterLink
          v-for="c in catalog.counties"
          :key="c.code"
          :to="`/county/${c.code}`"
          :aria-current="route.params.code === c.code ? 'page' : undefined"
        >
          <span class="mi">{{ DASHBOARD_ICONS.county }}</span
          ><span>{{ c.name }}</span>
        </RouterLink>
      </nav>
    </template>
  </aside>
</template>

<style scoped>
.sidebar {
  position: sticky;
  top: var(--header-h);
  align-self: start;
  width: 196px;
  height: calc(100vh - var(--header-h));
  overflow-y: auto;
  margin: var(--font-m) 0;
  padding: 0 var(--font-ms) 0 var(--font-m);
  border-right: 1px solid var(--border);
  background: var(--bg);
  flex-shrink: 0;
  scrollbar-width: none;
}
.sidebar.collapsed {
  width: 64px;
  padding-right: 8px;
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
}
.collapsed .head {
  justify-content: center;
}
h2 {
  font-size: var(--font-l);
  font-weight: 700;
}
.collapse {
  display: grid;
  place-items: center;
  border: 0;
  background: transparent;
  color: var(--text-primary);
  cursor: pointer;
  transition: color 0.2s;
}
.collapse .mi {
  font-size: var(--font-l);
}
.collapse:hover {
  color: var(--accent);
}
.group {
  margin: 0.4rem 0 0.2rem;
  padding-left: var(--font-ms);
  font-size: var(--font-l);
  color: var(--text-muted);
}
.alert {
  margin-bottom: 4px;
}
.alert a,
.alert a:hover,
.alert a[aria-current='page'] {
  color: var(--danger-text);
}
.alert a {
  min-height: 44px;
}
.alert-label {
  display: flex;
  flex-direction: column;
  line-height: 1.25;
  font-weight: 700;
}
.alert-label small {
  font-size: var(--font-s);
  font-weight: 400;
}
.divider {
  margin: 6px 8px;
  border: 0;
  border-top: 1px solid var(--border);
}
nav {
  display: flex;
  flex-direction: column;
}
nav a {
  display: flex;
  align-items: center;
  gap: 0.7rem;
  min-height: 36px;
  padding-left: var(--font-ms);
  border-left: 4px solid transparent;
  border-radius: var(--radius);
  color: var(--text-primary);
  text-decoration: none;
  font-size: var(--font-m);
  transition: color 0.2s;
}
nav a .mi {
  font-size: 1.25rem;
}
.counties a {
  min-height: 32px;
  font-size: var(--font-ms);
}
nav a:hover {
  color: var(--accent);
}
nav a[aria-current='page'] {
  border-left-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}
.collapsed nav a {
  justify-content: center;
  padding-left: 0;
}
.collapsed nav a span:not(.mi) {
  display: none;
}
.county-skeleton {
  display: flex;
  align-items: center;
  min-height: 32px;
  padding-left: calc(var(--font-ms) + 4px);
}
.county-skeleton .skeleton {
  width: 5rem;
  height: 0.8rem;
}
.scrim {
  display: none;
}

@media (max-width: 999px) {
  .sidebar,
  .sidebar.collapsed {
    position: fixed;
    z-index: 60;
    left: 0;
    width: 240px;
    margin: 0;
    padding-top: var(--font-m);
    transform: translateX(-100%);
    transition: transform 0.2s;
  }
  .sidebar.open {
    transform: none;
  }
  .collapse {
    display: none;
  }
  .scrim {
    display: block;
    position: fixed;
    inset: var(--header-h) 0 0 0;
    z-index: 55;
    background: var(--overlay);
  }
}
</style>
