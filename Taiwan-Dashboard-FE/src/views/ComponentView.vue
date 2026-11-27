<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ApiError, getComponent, peekComponent } from '../api/client'
import type { ComponentInfo } from '../api/types'
import CountySelect from '../components/CountySelect.vue'
import ComponentInfoBody from '../components/dialogs/ComponentInfoBody.vue'
import InfoSkeleton from '../components/skeleton/InfoSkeleton.vue'
import { usePageTitle } from '../composables/usePageTitle'
import { DASHBOARD_ICONS } from '../lib/topics'
import { useCatalog } from '../stores/catalog'

const props = defineProps<{ slug: string }>()
const route = useRoute()
const router = useRouter()
const catalog = useCatalog()
void catalog.load()

const component = ref<ComponentInfo | null>(null)
const notFound = ref(false)
const failed = ref(false)
watch(
  () => props.slug,
  async (slug) => {
    component.value = peekComponent(slug)
    notFound.value = failed.value = false
    try {
      component.value = await getComponent(slug)
    } catch (e) {
      if (component.value) return
      if (e instanceof ApiError && e.status === 404) notFound.value = true
      else failed.value = true
    }
  },
  { immediate: true },
)

const county = computed(() => {
  const c = typeof route.query.county === 'string' ? route.query.county : null
  return c && component.value?.scopes.includes('county') ? c : null
})
usePageTitle(
  computed(() =>
    component.value
      ? `${component.value.name}${county.value ? `（${catalog.countyName(county.value)}）` : ''}`
      : '',
  ),
)
function changeCounty(c: string | null) {
  void router.replace({ query: c ? { county: c } : {} })
}
</script>

<template>
  <div class="page">
    <div class="page-title">
      <h1>
        <span class="mi" aria-hidden="true">{{
          component ? DASHBOARD_ICONS[component.topic] : 'widgets'
        }}</span>
        <template v-if="component">{{ component.name }}</template>
        <span
          v-else-if="!notFound && !failed"
          class="skeleton title-skeleton"
          aria-hidden="true"
        ></span>
      </h1>
      <span class="tools">
        <CountySelect
          v-if="component?.scopes.includes('county')"
          :model-value="county"
          @update:model-value="changeCounty"
        />
      </span>
    </div>
    <p v-if="notFound" class="muted">查無此元件：{{ slug }}</p>
    <p v-else-if="failed" class="muted">元件載入失敗</p>
    <div v-else-if="component" class="card panel">
      <ComponentInfoBody :component="component" :county="county" />
    </div>
    <InfoSkeleton v-else />
  </div>
</template>

<style scoped>
.title-skeleton {
  width: 9rem;
  height: 1.2rem;
}
.panel {
  width: fit-content;
  max-width: 100%;
  padding: var(--font-m);
  border: 1px solid var(--border);
}
</style>
