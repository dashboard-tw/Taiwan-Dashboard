<script setup lang="ts">
import { usePageTitle } from '../composables/usePageTitle'
import { TOPIC_GROUPS, TOPICS } from '../lib/topics'
import { useCatalog } from '../stores/catalog'

usePageTitle('網站導覽')

const catalog = useCatalog()
void catalog.load()

const groups = TOPIC_GROUPS.map((g) => ({ ...g, topics: TOPICS.filter((tp) => tp.group === g.id) }))

const others = [
  { to: '/', label: '首頁總覽' },
  { to: '/mapdata/map', label: '圖資' },
  { to: '/typhoons', label: '歷次颱風' },
  { to: '/components', label: '元件庫' },
]

const services = [
  { to: '/developers', label: '開放資料 API' },
  { to: '/showcase/app', label: '應用活化・App' },
  { to: '/showcase/web', label: '應用活化・網頁應用' },
  { to: '/showcase/innovation', label: '應用活化・創意應用' },
]

const about = [
  { to: '/about', label: '關於' },
  { to: '/about#sources', label: '資料來源與授權' },
  { to: '/privacy', label: '隱私權政策' },
  { to: '/terms', label: '使用條款' },
]
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">account_tree</span>網站導覽</h1>

    <div class="doc">
      <p>本站分成主題儀表板、縣市總覽、開放資料與網站資訊。每個主題頁都可以再選擇縣市。</p>

      <h2>儀表板</h2>
      <ul class="links">
        <li v-for="l in others" :key="l.to">
          <RouterLink :to="l.to">{{ l.label }}</RouterLink>
        </li>
      </ul>
      <template v-for="g in groups" :key="g.id">
        <h3>{{ g.label }}</h3>
        <ul class="links">
          <li v-for="tp in g.topics" :key="tp.slug">
            <RouterLink :to="`/${tp.slug}`">{{ tp.title }}</RouterLink>
          </li>
        </ul>
      </template>

      <h2>縣市總覽</h2>
      <ul class="links">
        <li v-for="c in catalog.counties" :key="c.code">
          <RouterLink :to="`/county/${c.code}`">{{ c.name }}</RouterLink>
        </li>
      </ul>

      <h2>開放資料</h2>
      <ul class="links">
        <li v-for="l in services" :key="l.to">
          <RouterLink :to="l.to">{{ l.label }}</RouterLink>
        </li>
      </ul>

      <h2>網站資訊</h2>
      <ul class="links">
        <li v-for="l in about" :key="l.to">
          <RouterLink :to="l.to">{{ l.label }}</RouterLink>
        </li>
      </ul>
    </div>
  </div>
</template>

<style scoped>
.doc h3 {
  margin: 1rem 0 0.4rem;
  font-size: var(--font-m);
}
.links {
  display: flex;
  flex-wrap: wrap;
  gap: 0.3rem 1.2rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
.links li {
  margin: 0;
}
</style>
