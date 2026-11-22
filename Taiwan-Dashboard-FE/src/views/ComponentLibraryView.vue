<script setup lang="ts">
import { computed, ref } from 'vue'
import { getComponents, peekComponents } from '../api/client'
import type { ComponentInfo, Topic } from '../api/types'
import ComponentCard from '../components/ComponentCard.vue'
import CardSkeleton from '../components/skeleton/CardSkeleton.vue'
import { usePageTitle } from '../composables/usePageTitle'
import { slotOf, UNKNOWN_SLOTS } from '../lib/layoutHints'
import { TOPICS as TOPIC_LIST } from '../lib/topics'

usePageTitle('元件庫')

const TOPICS: { id: Topic | ''; label: string }[] = [
  { id: '', label: '全部' },
  ...TOPIC_LIST.map((t) => ({ id: t.slug, label: t.title })),
]

const list = ref<ComponentInfo[] | null>(peekComponents())
const error = ref(false)
getComponents()
  .then((l) => (list.value = l))
  .catch(() => (error.value = !list.value))

const q = ref('')
const topic = ref<Topic | ''>('')
const shown = computed(() =>
  (list.value ?? []).filter((c) => {
    if (topic.value && c.topic !== topic.value) return false
    const s = q.value.trim()
    return !s || c.name.includes(s) || c.short_desc.includes(s) || c.slug.includes(s)
  }),
)
const skeleton = computed(() => (list.value ? list.value.map(slotOf) : UNKNOWN_SLOTS))
</script>

<template>
  <div class="page">
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">apps</span>元件庫
      <span class="tools">
        <input
          v-model="q"
          class="search"
          type="search"
          placeholder="搜尋元件"
          aria-label="搜尋元件"
        />
      </span>
    </h1>
    <div class="filters" role="group" aria-label="依主題篩選">
      <button
        v-for="t in TOPICS"
        :key="t.id"
        type="button"
        :class="{ active: topic === t.id }"
        :aria-pressed="topic === t.id"
        @click="topic = t.id"
      >
        {{ t.label }}
      </button>
    </div>

    <div v-if="error" class="placeholder">
      <span class="mi" aria-hidden="true">error</span>
      <p>元件庫載入失敗</p>
    </div>
    <div v-else-if="!list" class="component-grid">
      <CardSkeleton
        v-for="(s, i) in skeleton"
        :key="i"
        :class="{ wide: s.wide }"
        :kind="s.kind"
        :pills="s.pills"
      />
    </div>
    <div v-else class="component-grid">
      <ComponentCard v-for="c in shown" :key="c.slug" :component="c" />
      <p v-if="shown.length === 0" class="muted">沒有符合的元件</p>
    </div>
  </div>
</template>

<style scoped>
.search {
  width: 12rem;
  padding: 3px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text-primary);
  font: inherit;
  font-size: 0.875rem;
}
.filters {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin: calc(-1 * var(--font-m) / 2) 0 var(--font-m);
}
.filters button {
  padding: 2px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: none;
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.875rem;
  cursor: pointer;
}
.filters button.active {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
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
@media (max-width: 759px) {
  .search {
    width: 8rem;
  }
}
</style>
