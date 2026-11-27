<script setup lang="ts">
import { computed, defineAsyncComponent, h, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getDashboard, getTyphoons, peekDashboard, type TyphoonEvent } from '../api/client'
import EarthquakeReports from '../components/EarthquakeReports.vue'
import { pageTyphoon } from '../lib/pageContext'
import { typhoonName, typhoonPeriod } from '../lib/typhoon'
import type { Dashboard } from '../api/types'
import ComponentCard from '../components/ComponentCard.vue'
import CardSkeleton from '../components/skeleton/CardSkeleton.vue'
import MapModeSkeleton from '../components/skeleton/MapModeSkeleton.vue'
import { slotsOf } from '../lib/layoutHints'

import CountySelect from '../components/CountySelect.vue'
import { usePageTitle } from '../composables/usePageTitle'
import { DASHBOARD_ICONS } from '../lib/topics'
import { useTyphoonAlert } from '../composables/useTyphoonAlert'
import { useCatalog } from '../stores/catalog'

const props = defineProps<{
  slug: string
  county?: string
  map?: boolean
  typhoonId?: string
}>()
const typhoon = useTyphoonAlert()

watch(
  () => props.typhoonId,
  (id) => (pageTyphoon.value = id ?? null),
  { immediate: true },
)
onBeforeUnmount(() => (pageTyphoon.value = null))

const typhoons = ref<TyphoonEvent[] | null>(null)
watch(
  () => [props.typhoonId, props.slug],
  async () => {
    if (!props.typhoonId && props.slug !== 'typhoon') return
    try {
      typhoons.value = await getTyphoons()
    } catch {
      typhoons.value = []
    }
  },
  { immediate: true },
)
const event = computed(() => typhoons.value?.find((t) => t.id === props.typhoonId) ?? null)
const activeEvents = computed(() => (typhoons.value ?? []).filter((t) => t.active))

const router = useRouter()
const route = useRoute()
const catalog = useCatalog()
void catalog.load()

const county = computed(() => props.county ?? null)
// 地震頁的海嘯資訊和地震報告並排
const PAIRED = 'tsunami-recent'
const pairedCard = computed(() =>
  props.slug === 'earthquake'
    ? (dashboard.value?.components.find((c) => c.slug === PAIRED) ?? null)
    : null,
)
const gridComponents = computed(() =>
  (dashboard.value?.components ?? []).filter((c) => c !== pairedCard.value),
)
const countyName = computed(() => catalog.countyName(county.value))
const unknownCounty = computed(
  () => !!county.value && catalog.counties.length > 0 && !countyName.value,
)
const isHome = computed(() => props.slug === 'home')
const MapMode = defineAsyncComponent({
  loader: () => import('../components/map/MapMode.vue'),
  loadingComponent: { render: () => h(MapModeSkeleton, { slots: skeleton.value }) },
  delay: 0,
})

const dashboard = ref<Dashboard | null>(null)
const error = ref(false)
watch(
  () => props.slug,
  async (slug) => {
    dashboard.value = peekDashboard(slug)
    error.value = false
    try {
      dashboard.value = await getDashboard(slug)
    } catch {
      if (!dashboard.value) error.value = true
    }
  },
  { immediate: true },
)
const skeleton = computed(() => slotsOf(dashboard.value))

const title = computed(() => {
  const name = props.typhoonId
    ? event.value
      ? typhoonName(event.value)
      : ''
    : props.slug === 'typhoon' && typhoon.active.value
      ? typhoon.title.value
      : (dashboard.value?.name ?? '')
  return county.value && countyName.value ? `${countyName.value}${name}` : name
})
usePageTitle(computed(() => (isHome.value ? '' : title.value)))

function path(c: string | null) {
  const base = props.typhoonId ? `/typhoon/${props.typhoonId}` : `/${props.slug}`
  return `${base}${c ? `/${c}` : ''}${props.map ? '/map' : ''}`
}
function changeCounty(c: string | null) {
  if (isHome.value) return void router.push(c ? `/county/${c}` : '/')
  void router.push({ path: path(c), query: props.map ? route.query : {} })
}
function selectCounty(code: string) {
  if (isHome.value) return void router.push(`/county/${code}`)
  void router.push({ path: path(code), query: props.map ? route.query : {} })
}

const copied = ref(false)
async function share() {
  const url = location.href
  if (navigator.share) {
    try {
      await navigator.share({ title: `${title.value}｜臺灣儀表板`, url })
      return
    } catch {}
  }
  try {
    await navigator.clipboard.writeText(url)
    copied.value = true
    setTimeout(() => (copied.value = false), 2000)
  } catch {
    window.prompt('請複製網址', url)
  }
}
</script>

<template>
  <div class="page">
    <div class="page-title">
      <h1>
        <span class="mi" aria-hidden="true">{{ DASHBOARD_ICONS[slug] ?? 'dashboard' }}</span>
        <template v-if="title">{{ title }}</template>
        <span v-else class="skeleton title-skeleton" aria-hidden="true"></span>
      </h1>
      <span class="tools">
        <CountySelect
          :model-value="county"
          :national-label="isHome ? '選擇縣市' : '全國'"
          @update:model-value="changeCounty"
        />
        <button
          type="button"
          class="icon-button"
          :title="copied ? '已複製網址' : '分享'"
          :aria-label="copied ? '已複製網址' : '分享這個頁面'"
          @click="share"
        >
          <span class="mi" aria-hidden="true">{{ copied ? 'check' : 'share' }}</span>
        </button>
      </span>
    </div>
    <p
      v-if="slug === 'typhoon' && typhoon.loaded.value && !typhoon.active.value"
      class="typhoon-note"
    >
      目前沒有颱風警報，以下為最新資料。
      <RouterLink to="/typhoons">查看歷次颱風</RouterLink>
    </p>
    <p v-if="slug === 'typhoon' && activeEvents.length" class="typhoon-links">
      <RouterLink v-for="t in activeEvents" :key="t.id" :to="`/typhoon/${t.id}`">
        <span class="mi" aria-hidden="true">history</span>{{ typhoonName(t) }}專區
      </RouterLink>
    </p>
    <p v-if="typhoonId && event" class="typhoon-period">
      {{ typhoonPeriod(event) }}・{{ event.last_alert }}・共 {{ event.warnings }} 報警報
      <RouterLink v-if="event.active" to="/typhoon">查看即時資料</RouterLink>
      <RouterLink to="/typhoons">歷次颱風</RouterLink>
    </p>
    <p v-if="typhoonId && typhoons && !event" class="notice">
      查無此颱風：{{ typhoonId }}。<RouterLink to="/typhoons">看歷次颱風</RouterLink>
    </p>

    <p v-if="unknownCounty" class="notice">查無此縣市代碼：{{ county }}</p>
    <p v-if="slug === 'power' && county" class="notice">電力資料僅有全國數據。</p>

    <div v-if="error" class="placeholder">
      <span class="mi" aria-hidden="true">error</span>
      <p>儀表板載入失敗</p>
    </div>
    <MapModeSkeleton v-else-if="!dashboard && map" :slots="skeleton" />
    <MapMode
      v-else-if="map && dashboard"
      :dashboard="dashboard"
      :county="county"
      @select-county="selectCounty"
    />
    <div v-else-if="!dashboard" class="component-grid">
      <CardSkeleton
        v-for="(s, i) in skeleton"
        :key="i"
        :class="{ wide: s.wide }"
        :kind="s.kind"
        :pills="s.pills"
      />
    </div>
    <div v-else class="component-grid">
      <ComponentCard
        v-for="c in gridComponents"
        :key="c.slug"
        :class="{ wide: c.size === 'wide' }"
        :component="c"
        :county="county"
        @select-county="selectCounty"
      />
    </div>
    <div v-if="slug === 'earthquake' && !map && dashboard" class="quake-pair">
      <ComponentCard
        v-if="pairedCard"
        :component="pairedCard"
        :county="county"
        @select-county="selectCounty"
      />
      <EarthquakeReports scroll />
    </div>
  </div>
</template>

<style scoped>
.quake-pair {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  grid-auto-rows: 26rem;
  gap: var(--font-m);
  margin-top: var(--font-m);
}
@media (min-width: 65.625em) {
  .quake-pair {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
.quake-pair > .component-card {
  height: 100%;
}
.typhoon-links,
.typhoon-period {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 16px;
  margin: 0 0 12px;
  color: var(--text-muted);
  font-size: var(--font-ms);
}
.typhoon-links a {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.typhoon-note {
  margin: 0 0 var(--font-m);
  padding: 0.6rem 0.9rem;
  border-radius: var(--radius);
  background: var(--warning-bg);
  color: var(--warning-text);
  font-size: var(--font-ms);
}
.title-skeleton {
  width: 7rem;
  height: 1.2rem;
}
.notice {
  margin: -0.5rem 0 var(--font-m);
  color: var(--text-muted);
  font-size: 0.9rem;
}
.placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 50vh;
  color: var(--text-muted);
}
.placeholder .mi {
  font-size: 2.5rem;
  margin-bottom: 0.5rem;
}
.icon-button {
  display: grid;
  place-items: center;
  width: 2rem;
  height: 2rem;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}
.icon-button:hover {
  color: var(--text-primary);
}
</style>
