<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ApiError } from '../api/client'
import { getOAuthApps, revokeOAuthApp, type OAuthApp } from '../api/me'

const apps = ref<OAuthApp[] | null>(null)
const error = ref('')
const busy = ref<number | null>(null)
const notice = ref('')

async function load() {
  try {
    apps.value = await getOAuthApps()
    error.value = ''
  } catch {
    error.value = '無法載入已授權的 AI 工具，請重新整理。'
  }
}
onMounted(load)

const fmt = new Intl.DateTimeFormat('zh-TW', {
  timeZone: 'Asia/Taipei',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})
function when(iso: string | null) {
  return iso ? fmt.format(new Date(iso)) : '—'
}

async function revoke(a: OAuthApp) {
  if (!window.confirm(`取消 ${a.name} 的授權？它之後要重新連接才能使用。`)) return
  busy.value = a.id
  notice.value = ''
  try {
    await revokeOAuthApp(a.id)
    notice.value = `已取消 ${a.name} 的授權`
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '取消失敗，請再試一次。'
  } finally {
    busy.value = null
  }
}
</script>

<template>
  <section class="apps" aria-labelledby="oauth-apps-title">
    <h2 id="oauth-apps-title">
      <span class="mi" aria-hidden="true">smart_toy</span>已授權的 AI 工具
    </h2>
    <p class="muted intro">透過 MCP 連接的工具，若使用 API 獲取資料將照常計算資源用量。</p>
    <p v-if="error" class="warn" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>
    <ul v-if="apps && apps.length" class="list">
      <li v-for="a in apps" :key="a.id" class="card app">
        <div class="info">
          <p class="name">{{ a.name }}</p>
          <dl>
            <dt>最後使用</dt>
            <dd>{{ when(a.last_used_at) }}</dd>
            <dt>授權時間</dt>
            <dd>{{ when(a.created_at) }}</dd>
            <template v-if="a.client_uri">
              <dt>網站</dt>
              <dd>
                <a :href="a.client_uri" target="_blank" rel="noopener noreferrer">{{
                  a.client_uri
                }}</a>
              </dd>
            </template>
          </dl>
        </div>
        <button type="button" class="out" :disabled="busy !== null" @click="revoke(a)">
          <span class="mi" aria-hidden="true">link_off</span
          >{{ busy === a.id ? '處理中' : '取消授權' }}
        </button>
      </li>
    </ul>
    <p v-else-if="apps" class="muted small">還沒有授權任何 AI 工具。</p>
  </section>
</template>

<style scoped>
.apps {
  max-width: 40rem;
  margin: calc(var(--font-m) * 1.5) 0 var(--font-m);
}
h2 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 0 0 0.5rem;
  font-size: var(--font-m);
  font-weight: 400;
}
.intro {
  margin: 0 0 var(--font-m);
}
.list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
.app {
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: start;
  gap: 1rem;
  padding: var(--font-m);
}
.info {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-width: 0;
}
.name {
  margin: 0;
  font-weight: 600;
  overflow-wrap: anywhere;
}
dl {
  display: grid;
  grid-template-columns: 5rem 1fr;
  gap: 0.25rem 0.75rem;
  margin: 0;
  font-size: 0.875rem;
}
dt {
  color: var(--text-muted);
}
dd {
  margin: 0;
  overflow-wrap: anywhere;
}
dd a {
  color: var(--accent);
}
.out {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  white-space: nowrap;
  cursor: pointer;
}
.out:hover:not(:disabled) {
  border-color: var(--danger-text);
  color: var(--danger-text);
}
.out:disabled {
  opacity: 0.45;
  cursor: default;
}
.notice {
  color: var(--accent);
}
.small {
  font-size: var(--font-s);
}
@media (max-width: 32.5em) {
  .app {
    grid-template-columns: 1fr;
  }
  .app .out {
    justify-self: start;
  }
}
</style>
