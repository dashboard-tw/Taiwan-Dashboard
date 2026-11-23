<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { ApiError } from '../api/client'
import { decideOAuth, getOAuthRequest, type OAuthRequestInfo } from '../api/me'
import BrandMark from '../components/layout/BrandMark.vue'
import LoginPanel from '../components/LoginPanel.vue'
import DialogContainer from '../components/dialogs/DialogContainer.vue'
import { usePageTitle } from '../composables/usePageTitle'
import { useAuth } from '../stores/auth'

usePageTitle('授權 AI 工具')
const auth = useAuth()
const router = useRouter()

const query = window.location.search
const here = window.location.pathname + query
const info = ref<OAuthRequestInfo | null>(null)
const error = ref('')
const busy = ref<'approve' | 'deny' | null>(null)
const leaving = ref(false)
const ready = ref(false)

const mode = computed(() => {
  if (!ready.value) return 'loading'
  if (leaving.value) return 'leaving'
  if (!auth.me) return 'login'
  if (info.value?.client) return 'consent'
  if (error.value) return 'error'
  return 'loading'
})

function leave(url: string) {
  leaving.value = true
  window.location.assign(url)
}

async function loadRequest() {
  try {
    const r = await getOAuthRequest(query)
    if (r.error && r.redirect) {
      leave(r.redirect)
      return
    }
    if (r.error) {
      error.value = r.message || '此授權請求有問題，請重新連接。'
      return
    }
    info.value = r
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '載入失敗，請重新整理。'
  }
}

onMounted(async () => {
  await auth.load()
  ready.value = true
  if (auth.me) await loadRequest()
})

async function signedIn() {
  await auth.load(true)
  if (auth.me) await loadRequest()
}

async function decide(approve: boolean) {
  busy.value = approve ? 'approve' : 'deny'
  error.value = ''
  try {
    leave(await decideOAuth(query, approve))
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '送出失敗，請再試一次。'
    busy.value = null
  }
}

function close() {
  if (busy.value || leaving.value) return
  if (mode.value === 'consent') {
    void decide(false)
    return
  }
  void router.replace('/')
}
</script>

<template>
  <div class="page"></div>
  <DialogContainer :open="true" label="授權 AI 工具" @close="close">
    <div class="auth-dialog">
      <button
        v-if="mode !== 'leaving'"
        type="button"
        class="close"
        aria-label="關閉"
        @click="close"
      >
        <span class="mi" aria-hidden="true">close</span>
      </button>
      <header class="brand">
        <BrandMark class="logo" />
        <div>
          <p class="name">臺灣儀表板</p>
          <p class="name-en">Taiwan Dashboard</p>
        </div>
      </header>

      <template v-if="mode === 'login'">
        <p class="lead">請先登入帳戶再授權 AI 工具</p>
        <LoginPanel :redirect="here" @done="signedIn" />
        <p class="terms">
          登入即代表同意
          <RouterLink to="/terms">使用條款</RouterLink>
          與
          <RouterLink to="/privacy">隱私權政策</RouterLink>
        </p>
      </template>

      <template v-else-if="mode === 'consent' && info?.client">
        <p class="title">
          <strong>{{ info.client.name }}</strong
          ><br />想要使用你的帳號
        </p>
        <p v-if="info.client.verified_host" class="from">
          <span class="mi" aria-hidden="true">verified</span>{{ info.client.verified_host }}
        </p>
        <ul class="can">
          <li><span class="mi" aria-hidden="true">storage</span>查詢開放資料和圖表元件</li>
          <li><span class="mi" aria-hidden="true">data_usage</span>使用你的帳號獲取資料</li>
          <li><span class="mi" aria-hidden="true">menu_book</span>讀取 API 開發文件</li>
        </ul>
        <p class="account">{{ auth.me?.display_name }} · 授權後回到 {{ info.redirect_host }}</p>
        <p v-if="error" class="warn" role="alert">{{ error }}</p>
        <div class="actions">
          <button type="button" class="deny" :disabled="busy !== null" @click="decide(false)">
            {{ busy === 'deny' ? '處理中' : '拒絕' }}
          </button>
          <button type="button" class="approve" :disabled="busy !== null" @click="decide(true)">
            {{ busy === 'approve' ? '處理中' : '授權' }}
          </button>
        </div>
        <p class="terms">
          授權即代表同意
          <RouterLink to="/terms">使用條款</RouterLink><br />可隨時至
          <RouterLink to="/me/devices">登入裝置</RouterLink>
          取消授權
        </p>
      </template>

      <template v-else-if="mode === 'error'">
        <p class="warn center" role="alert">{{ error }}</p>
        <RouterLink to="/" class="deny home">回首頁</RouterLink>
      </template>

      <p v-else-if="mode === 'leaving'" class="lead" role="status">正在回到應用…</p>

      <div v-else class="loading">
        <span class="skeleton bar wide"></span>
        <span class="skeleton bar"></span>
      </div>
    </div>
  </DialogContainer>
</template>

<style scoped>
.page {
  min-height: 60vh;
}
.auth-dialog {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 18px;
  width: min(340px, calc(100vw - 4rem));
  padding: 18px 8px 4px;
}
.close {
  position: absolute;
  top: -6px;
  right: -6px;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
}
.close:hover {
  background: var(--surface-muted);
  color: var(--text-primary);
}
.brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
}
.logo {
  width: 2.25rem;
  height: 2.25rem;
  color: var(--text-primary);
}
.name {
  font-size: 1.35rem;
  font-weight: 700;
  line-height: 1.2;
}
.name-en {
  color: var(--text-secondary);
  font-size: 0.8rem;
  letter-spacing: 0.02em;
}
.lead {
  color: var(--text-muted);
  font-size: 0.88rem;
  text-align: center;
}
.title {
  font-size: 1.05rem;
  line-height: 1.6;
  text-align: center;
}
.from {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  margin-top: -10px;
  color: var(--text-muted);
  font-size: 0.8rem;
}
.from .mi {
  color: var(--accent);
  font-size: 1rem;
}
.can {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 14px 16px;
  border-radius: 10px;
  background: var(--surface-muted);
  font-size: 0.9rem;
  list-style: none;
}
.can li {
  display: flex;
  align-items: center;
  gap: 10px;
}
.can .mi {
  color: var(--accent);
  font-size: 1.15rem;
}
.account {
  color: var(--text-muted);
  font-size: 0.8rem;
  text-align: center;
  overflow-wrap: anywhere;
}
.actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}
.approve,
.deny {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 44px;
  border-radius: 999px;
  font: inherit;
  font-size: 0.95rem;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
  transition: opacity 0.2s;
}
.approve {
  border: 0;
  background: var(--accent-strong);
  color: #ffffff;
}
.deny {
  border: 1px solid var(--border);
  background: transparent;
  color: var(--text-primary);
}
.approve:hover:not(:disabled),
.deny:hover:not(:disabled) {
  opacity: 0.85;
}
button:disabled {
  opacity: 0.5;
  cursor: default;
}
.home {
  width: 100%;
}
.terms {
  color: var(--text-muted);
  font-size: 0.75rem;
  line-height: 1.75;
  text-align: center;
}
.terms a {
  color: var(--accent);
}
.warn {
  font-size: 0.85rem;
}
.center {
  text-align: center;
}
.loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  padding: 10px 0 16px;
}
.bar {
  display: block;
  width: 60%;
  height: 1rem;
}
.bar.wide {
  width: 85%;
  height: 1.3rem;
}
</style>
