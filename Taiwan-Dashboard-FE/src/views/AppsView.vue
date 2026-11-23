<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { APP_CATEGORIES, getPublicApps, type AppCategory, type PublicApp } from '../api/apps'
import { usePageTitle } from '../composables/usePageTitle'
import { TOPICS } from '../lib/apiSurvey'

const props = defineProps<{ category: AppCategory }>()
const current = computed(
  () => APP_CATEGORIES.find((c) => c.value === props.category) ?? APP_CATEGORIES[0]!,
)
usePageTitle(() => `應用活化・${current.value.label}`)

const apps = ref<PublicApp[] | null>(null)
const error = ref('')
onMounted(async () => {
  try {
    apps.value = await getPublicApps()
  } catch {
    error.value = '載入失敗，請稍後再試。'
  }
})

const shown = computed(() => apps.value?.filter((a) => a.category === props.category) ?? null)
const counts = computed(() => {
  const n: Record<string, number> = {}
  for (const a of apps.value ?? []) n[a.category] = (n[a.category] ?? 0) + 1
  return n
})

function links(a: PublicApp) {
  return [
    { label: '網站', icon: 'language', url: a.web_url },
    { label: 'App Store', icon: 'phone_iphone', url: a.ios_url },
    { label: 'Google Play', icon: 'android', url: a.android_url },
  ].filter((l) => l.url)
}
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">apps</span>應用活化</h1>
    <p class="intro">
      使用臺灣儀表板開放資料 API 的應用與作品。也想上架嗎？<RouterLink to="/me/showcase"
        >申請應用活化</RouterLink
      >
    </p>
    <nav class="tabs" aria-label="應用類別">
      <RouterLink
        v-for="c in APP_CATEGORIES"
        :key="c.value"
        :to="`/showcase/${c.value}`"
        :aria-current="c.value === category ? 'page' : undefined"
        :class="{ on: c.value === category }"
        ><span class="mi" aria-hidden="true">{{ c.icon }}</span
        >{{ c.label
        }}<span v-if="counts[c.value]" class="count">{{ counts[c.value] }}</span></RouterLink
      >
    </nav>
    <p v-if="error" class="warn" role="alert">{{ error }}</p>

    <div v-if="!apps && !error" class="grid">
      <div v-for="i in 3" :key="i" class="card app">
        <span class="skeleton shot"></span>
        <span class="skeleton line"></span>
        <span class="skeleton line short"></span>
      </div>
    </div>
    <p v-else-if="shown && !shown.length" class="card empty">目前還沒有應用</p>
    <div v-else class="grid">
      <article v-for="a in shown" :key="a.id" class="card app">
        <img
          v-if="a.screenshot_url"
          :src="a.screenshot_url"
          :alt="`${a.name}的網站截圖`"
          class="shot"
          loading="lazy"
        />
        <img
          v-else-if="a.image_url"
          :src="a.image_url"
          :alt="a.name"
          class="shot logo-only"
          loading="lazy"
        />
        <div v-else class="shot placeholder" aria-hidden="true">{{ a.name.slice(0, 1) }}</div>
        <h2>
          <img
            v-if="a.screenshot_url && a.image_url"
            :src="a.image_url"
            alt=""
            class="logo"
            loading="lazy"
          />{{ a.name }}
        </h2>
        <p class="desc">{{ a.description }}</p>
        <ul class="tags" aria-label="用到的資料">
          <li v-for="t in a.topics" :key="t">{{ TOPICS[t] ?? t }}</li>
        </ul>
        <div class="foot">
          <span v-if="a.author" class="author">{{ a.author }}</span>
          <span class="links">
            <a
              v-for="l in links(a)"
              :key="l.label"
              :href="l.url"
              target="_blank"
              rel="noopener noreferrer nofollow"
              :title="l.label"
              :aria-label="`${a.name}・${l.label}`"
              ><span class="mi" aria-hidden="true">{{ l.icon }}</span></a
            >
          </span>
        </div>
      </article>
    </div>
  </div>
</template>

<style scoped>
.intro {
  max-width: 48rem;
  margin: 0 0 var(--font-m);
  color: var(--text-secondary);
  line-height: 1.7;
}
.tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-bottom: var(--font-m);
}
.tabs a {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 14px;
  border: 1px solid var(--border);
  border-radius: 999px;
  color: var(--text-primary);
  text-decoration: none;
}
.tabs a:hover {
  border-color: var(--accent);
}
.tabs a.on {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}
.tabs .mi {
  font-size: 1.1rem;
}
.count {
  min-width: 1.25rem;
  padding: 0 6px;
  border-radius: 999px;
  background: var(--surface-muted);
  font-size: var(--font-s);
  text-align: center;
}
.small {
  font-size: var(--font-s);
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 17rem), 1fr));
  gap: var(--font-m);
}
.app {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: var(--font-m);
  min-width: 0;
}
.shot {
  display: block;
  width: 100%;
  aspect-ratio: 16 / 10;
  border-radius: var(--radius);
  object-fit: contain;
  background: var(--surface-muted);
}
.shot.logo-only {
  padding: 12%;
}
.app h2 .logo {
  width: 1.5em;
  height: 1.5em;
  margin-right: 0.4em;
  border-radius: 0.3em;
  object-fit: cover;
  vertical-align: -0.35em;
}
.placeholder {
  display: grid;
  place-items: center;
  color: var(--text-muted);
  font-size: 3rem;
  font-weight: 700;
}
h2 {
  margin: 0.25rem 0 0;
  font-size: var(--font-ms);
  overflow-wrap: anywhere;
}
.desc {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 0.875rem;
  line-height: 1.6;
  white-space: pre-line;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 4;
  line-clamp: 4;
}
.tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.tags li {
  padding: 0 8px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: var(--font-s);
}
.foot {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: auto;
  padding-top: 0.5rem;
}
.author {
  min-width: 0;
  overflow: hidden;
  color: var(--text-muted);
  font-size: var(--font-s);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.links {
  display: flex;
  gap: 4px;
  margin-left: auto;
}
.links a {
  display: grid;
  place-items: center;
  width: 2.25rem;
  height: 2.25rem;
  border: 1px solid var(--border);
  border-radius: 50%;
  color: var(--text-primary);
  text-decoration: none;
}
.links a:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.links .mi {
  font-size: 1.15rem;
}
.empty {
  padding: 2rem var(--font-m);
  color: var(--text-secondary);
  text-align: center;
}
.line {
  display: block;
  height: 1rem;
}
.line.short {
  width: 60%;
}
</style>
