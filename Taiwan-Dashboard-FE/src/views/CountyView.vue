<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { getDashboard, getDashboards, peek, peekDashboard } from '../api/client'
import type { ComponentInfo, Dashboard, DashboardSummary } from '../api/types'
import ComponentCard from '../components/ComponentCard.vue'
import CardSkeleton from '../components/skeleton/CardSkeleton.vue'
import { UNKNOWN_SLOTS } from '../lib/layoutHints'
import CountySelect from '../components/CountySelect.vue'
import { usePageTitle } from '../composables/usePageTitle'
import { useCatalog } from '../stores/catalog'

const props = defineProps<{ code: string }>()
const router = useRouter()
const catalog = useCatalog()
void catalog.load()

const name = computed(() => catalog.countyName(props.code))
const unknown = computed(() => catalog.counties.length > 0 && !name.value)
usePageTitle(computed(() => (name.value ? `${name.value}總覽` : '')))

const NOT_TOPICS = ['home', 'typhoon', 'typhoon-event', 'mapdata']
function merge(boards: Dashboard[]): ComponentInfo[] {
  const seen = new Set<string>()
  return boards.flatMap((b) => b.components).filter((c) => !seen.has(c.slug) && !!seen.add(c.slug))
}
function cachedBoards(): Dashboard[] {
  const list = peek<DashboardSummary[]>('/dashboards') ?? []
  return list
    .filter((d) => !NOT_TOPICS.includes(d.slug))
    .map((d) => peekDashboard(d.slug))
    .filter((d): d is Dashboard => !!d)
}

const components = ref<ComponentInfo[]>(merge(cachedBoards()))
const error = ref(false)
onMounted(async () => {
  try {
    const list = (await getDashboards()).filter((d) => !NOT_TOPICS.includes(d.slug))
    components.value = merge(await Promise.all(list.map((d) => getDashboard(d.slug))))
  } catch {
    if (!components.value.length) error.value = true
  }
})
const skeleton = computed(() => (components.value.length ? [] : UNKNOWN_SLOTS))

function changeCounty(c: string | null) {
  void router.push(c ? `/county/${c}` : '/')
}
</script>

<template>
  <div class="page">
    <div class="page-title">
      <h1>
        <span class="mi" aria-hidden="true">location_on</span>
        {{ name ? `${name}總覽` : '' }}
      </h1>
      <span class="tools">
        <CountySelect
          :model-value="code"
          national-label="首頁總覽"
          @update:model-value="changeCounty"
        />
      </span>
    </div>
    <p v-if="unknown" class="notice">查無此縣市：{{ code }}</p>
    <p v-else-if="error" class="notice">縣市總覽載入失敗</p>
    <div v-else-if="!components.length" class="component-grid">
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
        v-for="c in components"
        :key="c.slug"
        :class="{ wide: c.size === 'wide' }"
        :component="c"
        :county="code"
        @select-county="changeCounty"
      />
    </div>
  </div>
</template>

<style scoped>
.notice {
  color: var(--text-muted);
}
</style>
