<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { getShowcaseApps, reviewApp, type AdminApp } from '../../api/admin'
import {
  APP_STATUS,
  appImageUrl,
  categoryLabel,
  type AppContent,
  type AppStatus,
} from '../../api/apps'
import { ApiError } from '../../api/client'
import { usePageTitle } from '../../composables/usePageTitle'
import { topicNames } from '../../lib/apiSurvey'

usePageTitle('應用審核')

const FILTERS: { value: '' | AppStatus; label: string }[] = [
  { value: 'pending', label: '待審核' },
  { value: 'approved', label: '已通過' },
  { value: 'rejected', label: '未通過' },
  { value: 'removed', label: '已下架' },
  { value: '', label: '全部' },
]
const filter = ref<'' | AppStatus>('pending')
const apps = ref<AdminApp[] | null>(null)
const error = ref('')
const notes = ref<Record<number, string>>({})
const busy = ref<number | null>(null)

async function load() {
  try {
    apps.value = await getShowcaseApps(filter.value)
    error.value = ''
  } catch {
    error.value = '無法載入申請'
  }
}
onMounted(load)
watch(filter, () => {
  apps.value = null
  load()
})

async function act(a: AdminApp, action: 'approve' | 'reject' | 'remove') {
  const note = (notes.value[a.id] ?? '').trim()
  if (action !== 'approve' && !note) {
    error.value = '請填寫原因'
    return
  }
  if (action === 'remove' && !confirm(`確定要下架「${a.published?.name ?? a.name}」嗎？`)) return
  busy.value = a.id
  try {
    await reviewApp(a.id, action, note)
    notes.value[a.id] = ''
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '操作失敗，請再試一次'
  } finally {
    busy.value = null
  }
}

function links(c: AppContent) {
  return [
    { label: '網站', url: c.web_url },
    { label: 'App Store', url: c.ios_url },
    { label: 'Google Play', url: c.android_url },
  ].filter((l) => l.url)
}
function changes(a: AdminApp) {
  const p = a.published
  if (!p || a.status !== 'pending') return []
  const out: string[] = []
  if (p.category !== a.category) out.push('類別')
  if (p.name !== a.name) out.push('名稱')
  if (p.description !== a.description) out.push('介紹')
  if (p.web_url !== a.web_url || p.ios_url !== a.ios_url || p.android_url !== a.android_url)
    out.push('連結')
  if (p.image !== a.image) out.push('圖片')
  if (p.topics.join() !== a.topics.join()) out.push('用到的資料')
  if (p.show_author !== a.show_author) out.push('是否顯示作者')
  return out
}
const empty = computed(() => apps.value?.length === 0)
function time(iso: string) {
  return new Date(iso).toLocaleString('zh-TW', { timeZone: 'Asia/Taipei', hour12: false })
}
</script>

<template>
  <div>
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">apps</span>應用審核
      <RouterLink to="/showcase/app" class="tools muted">查看應用活化</RouterLink>
    </h1>
    <nav class="filters" aria-label="篩選">
      <button
        v-for="f in FILTERS"
        :key="f.value"
        type="button"
        class="btn"
        :class="{ on: filter === f.value }"
        @click="filter = f.value"
      >
        {{ f.label }}
      </button>
    </nav>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>
    <p v-if="!apps && !error" class="muted">載入中</p>
    <p v-if="empty" class="muted">沒有符合的申請</p>

    <section v-for="a in apps ?? []" :key="a.id" class="card block app">
      <img v-if="a.image" :src="appImageUrl(a.image)!" alt="" class="shot" />
      <div v-else class="shot empty mi" aria-hidden="true">apps</div>
      <div class="body">
        <h2>
          {{ a.name }}
          <span
            class="badge"
            :class="{
              ok: a.status === 'approved',
              bad: a.status === 'rejected' || a.status === 'removed',
            }"
            >{{ APP_STATUS[a.status] }}</span
          >
          <span v-if="a.published && a.status !== 'approved'" class="badge warn">上架中</span>
        </h2>
        <p class="pre">{{ a.description }}</p>
        <p class="small">
          <a
            v-for="l in links(a)"
            :key="l.label"
            :href="l.url"
            target="_blank"
            rel="noopener noreferrer"
            class="link"
            >{{ l.label }}<span class="mi" aria-hidden="true">open_in_new</span></a
          >
        </p>
        <p class="small muted">
          類別：{{ categoryLabel(a.category) }}・用到的資料：{{ topicNames(a.topics) }}
        </p>
        <p class="small muted">
          申請者：<RouterLink :to="{ path: '/admin/users', query: { id: a.user_id } }"
            >{{ a.display_name }}（#{{ a.user_id }}）</RouterLink
          >
          ・{{ a.show_author ? '顯示作者' : '不顯示作者' }}・更新於 {{ time(a.updated_at) }}
        </p>
        <p v-if="changes(a).length" class="small warn">修改項目：{{ changes(a).join('、') }}</p>
        <p v-if="a.review_note" class="small bad">原因：{{ a.review_note }}</p>

        <div v-if="a.status === 'pending' || a.published" class="review">
          <input
            v-model="notes[a.id]"
            class="input"
            maxlength="200"
            placeholder="不通過或下架的原因"
          />
          <template v-if="a.status === 'pending'">
            <button type="button" class="btn" :disabled="busy === a.id" @click="act(a, 'approve')">
              <span class="mi" aria-hidden="true">check</span>通過
            </button>
            <button type="button" class="btn" :disabled="busy === a.id" @click="act(a, 'reject')">
              <span class="mi" aria-hidden="true">undo</span>不通過
            </button>
          </template>
          <button
            v-if="a.published"
            type="button"
            class="btn"
            :disabled="busy === a.id"
            @click="act(a, 'remove')"
          >
            <span class="mi" aria-hidden="true">block</span>下架
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.filters {
  display: flex;
  gap: 0.5rem;
  margin-bottom: var(--font-m);
}
.filters .on {
  border-color: var(--accent);
  color: var(--accent);
}
.app {
  display: flex;
  gap: var(--font-m);
}
.shot {
  flex-shrink: 0;
  width: 240px;
  height: 150px;
  border-radius: var(--radius);
  object-fit: contain;
  background: var(--surface-muted);
}
.shot.empty {
  display: grid;
  place-items: center;
  font-size: 2rem;
  color: var(--text-muted);
}
.body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.body h2 {
  gap: 0.5rem;
  margin-bottom: 0.25rem;
}
.body p {
  margin: 0;
}
.pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-right: 1rem;
}
.link .mi {
  font-size: 0.9rem;
}
.review {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.5rem;
}
.review .input {
  flex: 1 1 16rem;
}
.small {
  font-size: var(--font-s);
}
</style>
