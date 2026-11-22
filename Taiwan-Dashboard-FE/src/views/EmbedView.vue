<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getComponent, peekComponent } from '../api/client'
import type { ComponentInfo } from '../api/types'
import ComponentCard from '../components/ComponentCard.vue'
import CardSkeleton from '../components/skeleton/CardSkeleton.vue'
import { forceTheme } from '../theme'
import { useCatalog } from '../stores/catalog'

const props = defineProps<{ slug: string }>()
const route = useRoute()
const router = useRouter()

const theme = route.query.theme
if (theme === 'light' || theme === 'dark') forceTheme(theme)
const catalog = useCatalog()
void catalog.load()

const county = computed(() => (typeof route.query.county === 'string' ? route.query.county : null))
const component = ref<ComponentInfo | null>(peekComponent(props.slug))
const failed = ref(false)

const detailHref = computed(
  () =>
    router.resolve({
      name: 'component',
      params: { slug: props.slug },
      query: county.value ? { county: county.value } : {},
    }).href,
)

onMounted(async () => {
  try {
    component.value = await getComponent(props.slug)
  } catch {
    if (!component.value) failed.value = true
  }
})
</script>

<template>
  <div class="embed">
    <ComponentCard v-if="component" :component="component" :county="county" mode="embed" />
    <p v-else-if="failed" class="muted">圖表暫時無法載入（dashboard.tw）</p>
    <CardSkeleton v-else class="embed-skeleton">
      <template #foot-right>
        <a class="info" :href="detailHref" target="_blank" rel="noopener"
          >dashboard.tw<span class="mi">arrow_circle_right</span></a
        >
      </template>
    </CardSkeleton>
  </div>
</template>

<style scoped>
.embed-skeleton {
  height: 100vh;
  border-radius: 0;
}
.info {
  display: flex;
  align-items: center;
  gap: 4px;
  color: var(--accent);
  font-size: var(--font-ms);
  text-decoration: none;
}
.embed p {
  padding: 1rem;
}
</style>
