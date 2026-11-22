<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listComponents, type ComponentSummary } from '../../api/admin'
import { usePageTitle } from '../../composables/usePageTitle'
import { rememberCount, rememberedCount } from '../../lib/rememberCount'
import { TOPIC_LABELS } from '../../lib/topics'

usePageTitle('元件')
const router = useRouter()

const TOPICS = TOPIC_LABELS
const COLUMNS = ['元件', '主題', '狀態', '所在儀表板', '版本', '最後修改', '']
const skeletonRows = rememberedCount('admin-components', 8)

const list = ref<ComponentSummary[] | null>(null)
const error = ref('')
const q = ref('')

onMounted(async () => {
  try {
    list.value = await listComponents()
    rememberCount('admin-components', list.value.length)
  } catch {
    error.value = '元件清單暫時無法取得'
  }
})

const shown = computed(() => {
  const s = q.value.trim()
  return (list.value ?? []).filter((c) => !s || c.name.includes(s) || c.slug.includes(s))
})

function open(c: ComponentSummary) {
  void router.push({ name: 'admin-component', params: { slug: c.slug } })
}

function time(iso: string) {
  return new Date(iso).toLocaleString('zh-TW', {
    timeZone: 'Asia/Taipei',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}
</script>

<template>
  <div>
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">widgets</span>元件
      <span class="tools">
        <input
          v-model="q"
          class="input search"
          type="search"
          placeholder="搜尋名稱或代碼"
          aria-label="搜尋元件"
        />
        <RouterLink class="btn-highlight" :to="{ name: 'admin-component-new' }">
          <span class="mi" aria-hidden="true">add</span>新增元件
        </RouterLink>
      </span>
    </h1>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>

    <section class="card block">
      <table>
        <thead>
          <tr>
            <th v-for="c in COLUMNS" :key="c">{{ c }}</th>
          </tr>
        </thead>
        <tbody v-if="list">
          <tr
            v-for="c in shown"
            :key="c.slug"
            class="clickable"
            tabindex="0"
            @click="open(c)"
            @keydown.enter="open(c)"
          >
            <td>
              <strong>{{ c.name }}</strong
              ><span class="sub">{{ c.slug }}</span>
            </td>
            <td>{{ TOPICS[c.topic] ?? c.topic }}</td>
            <td>
              <span :class="['badge', c.status === 'published' ? 'ok' : 'muted']">{{
                c.status === 'published' ? '已發布' : '草稿'
              }}</span>
            </td>
            <td :class="{ muted: !c.dashboards.length }">
              {{ c.dashboards.length ? c.dashboards.join('、') : '沒有' }}
            </td>
            <td>{{ c.versions }}</td>
            <td>{{ time(c.updated_at) }}</td>
            <td>
              <RouterLink
                class="btn"
                :to="{ name: 'admin-component-new', query: { from: c.slug } }"
                @click.stop
              >
                <span class="mi" aria-hidden="true">content_copy</span>複製
              </RouterLink>
            </td>
          </tr>
          <tr v-if="shown.length === 0">
            <td :colspan="COLUMNS.length" class="muted">沒有符合的元件</td>
          </tr>
        </tbody>
        <tbody v-else>
          <tr v-for="i in skeletonRows" :key="i">
            <td v-for="c in COLUMNS" :key="c"><span class="skeleton cell"></span></td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<style scoped>
.search {
  width: 12rem;
}
.btn {
  text-decoration: none;
}
</style>
