<script setup lang="ts">
import { openAuthPopup, waitForAuthPopup } from '../../lib/authPopup'
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ApiError } from '../../api/client'
import { getProviders, type Provider } from '../../api/auth'
import {
  deleteAccount,
  getAccount,
  renameAccount,
  startLinkProvider,
  unlinkProvider,
  type Account,
} from '../../api/me'
import { usePageTitle } from '../../composables/usePageTitle'
import { useAuth } from '../../stores/auth'

usePageTitle('我的帳號')
const auth = useAuth()
const route = useRoute()
const router = useRouter()

const PROVIDERS: Record<string, string> = {
  dev: '開發用登入',
  github: 'GitHub',
  google: 'Google',
  apple: 'Apple',
  gov: '政府單一身分驗證',
}

const linked = typeof route.query.linked === 'string' ? route.query.linked : ''
const linkError = typeof route.query.link_error === 'string' ? route.query.link_error : ''
const linkNotice = linked ? `已綁定 ${PROVIDERS[linked] ?? linked}` : ''
const LINK_ERRORS: Record<string, string> = {
  taken: '綁定失敗，這個帳號已用於另一個臺灣儀表板帳號。',
  provider: '綁定失敗，請先解除目前綁定的帳號。',
  signed_out: '綁定失敗，請重新登入後再試。',
  cancelled: '已取消綁定。',
  too_new: '綁定失敗，GitHub 帳號需註冊滿 30 天。',
  expired: '綁定逾時，請再試一次。',
}
const linkWarning = linkError ? (LINK_ERRORS[linkError] ?? '綁定失敗，請再試一次。') : ''
if (linked || linkError) void router.replace({ query: {} })

const account = ref<Account | null>(null)
const error = ref('')
const providers = ref<Provider[]>([])
onMounted(async () => {
  try {
    account.value = await getAccount()
  } catch {
    error.value = '無法載入帳號資料，請重新整理。'
  }
  try {
    providers.value = (await getProviders()).filter((p) => !p.dev)
  } catch {
    providers.value = []
  }
})

const methods = computed(() => {
  const have = account.value?.providers ?? []
  const ids = [
    ...new Set([...providers.value.map((p) => p.id), ...have.filter((p) => p !== 'dev')]),
  ]
  return ids.map((id) => ({ id, name: PROVIDERS[id] ?? id, linked: have.includes(id) }))
})
const linkedCount = computed(() => account.value?.providers.length ?? 0)
const methodBusy = ref('')
const methodError = ref('')
async function bind(id: string) {
  methodBusy.value = id
  methodError.value = ''
  if (typeof BroadcastChannel !== 'undefined') {
    const popup = openAuthPopup({ mode: 'link', provider: id })
    if (popup) {
      const abort = new AbortController()
      try {
        const result = await waitForAuthPopup(abort.signal)
        if ('error' in result) methodError.value = result.error
        else window.location.assign(result.url)
      } finally {
        methodBusy.value = ''
      }
      return
    }
  }
  try {
    window.location.href = await startLinkProvider(id)
  } catch (e) {
    methodError.value = e instanceof ApiError ? e.message : '綁定失敗，請再試一次。'
    methodBusy.value = ''
  }
}
async function unbind(id: string) {
  if (!window.confirm(`確定要解除綁定 ${PROVIDERS[id] ?? id} 嗎？`)) return
  methodBusy.value = id
  methodError.value = ''
  try {
    await unlinkProvider(id)
    account.value = await getAccount()
  } catch (e) {
    methodError.value = e instanceof ApiError ? e.message : '解除失敗，請再試一次。'
  } finally {
    methodBusy.value = ''
  }
}

const MAX_NAME = 30
const editing = ref(false)
const nameDraft = ref('')
const nameError = ref('')
const savingName = ref(false)
const nameInput = ref<HTMLInputElement | null>(null)
async function startRename() {
  nameDraft.value = account.value?.display_name ?? ''
  nameError.value = ''
  editing.value = true
  await nextTick()
  nameInput.value?.select()
}
async function saveName() {
  const name = nameDraft.value.trim()
  if (!name || [...name].length > MAX_NAME) {
    nameError.value = `名稱要 1 到 ${MAX_NAME} 個字`
    return
  }
  savingName.value = true
  nameError.value = ''
  try {
    const saved = await renameAccount(name)
    if (account.value) account.value.display_name = saved
    if (auth.me) auth.me.display_name = saved
    editing.value = false
  } catch (e) {
    nameError.value = e instanceof ApiError ? e.message : '儲存失敗，請再試一次。'
  } finally {
    savingName.value = false
  }
}
const joined = computed(() =>
  account.value
    ? new Date(account.value.created_at).toLocaleDateString('zh-TW', { timeZone: 'Asia/Taipei' })
    : '',
)

const confirmText = ref('')
const deleting = ref(false)
const deleteError = ref('')
async function remove() {
  deleting.value = true
  deleteError.value = ''
  try {
    await deleteAccount()
    auth.me = null
    void router.replace('/')
  } catch (e) {
    deleteError.value = e instanceof ApiError ? e.message : '刪除失敗，請再試一次。'
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">manage_accounts</span>我的帳號</h1>
    <p v-if="error" class="warn" role="alert">{{ error }}</p>
    <p v-if="linkNotice" class="notice" role="status">
      <span class="mi" aria-hidden="true">link</span>{{ linkNotice }}
    </p>
    <p v-if="linkWarning" class="warn" role="alert">{{ linkWarning }}</p>

    <section class="card block">
      <h2>基本資料</h2>
      <dl>
        <dt>名稱</dt>
        <dd>
          <form v-if="editing" class="rename" @submit.prevent="saveName">
            <input
              ref="nameInput"
              v-model="nameDraft"
              class="input"
              :maxlength="MAX_NAME * 2"
              autocomplete="nickname"
              aria-label="新的名稱"
              @keydown.esc="editing = false"
            />
            <button type="submit" class="small-btn primary" :disabled="savingName">
              {{ savingName ? '儲存中' : '儲存' }}
            </button>
            <button type="button" class="small-btn" :disabled="savingName" @click="editing = false">
              取消
            </button>
            <p v-if="nameError" class="warn field-error" role="alert">{{ nameError }}</p>
          </form>
          <template v-else-if="account">
            {{ account.display_name }}
            <button type="button" class="edit" aria-label="修改名稱" @click="startRename">
              <span class="mi" aria-hidden="true">edit</span>修改
            </button>
          </template>
          <span v-else class="skeleton line"></span>
        </dd>
        <dt>身分</dt>
        <dd>
          <template v-if="account">{{
            account.role === 'admin' ? '管理員' : '一般使用者'
          }}</template>
          <span v-else class="skeleton line"></span>
        </dd>
        <dt>建立日期</dt>
        <dd>
          <template v-if="account">{{ joined }}</template>
          <span v-else class="skeleton line"></span>
        </dd>
        <dt>登入裝置</dt>
        <dd><RouterLink to="/me/devices" class="link">管理登入裝置</RouterLink></dd>
      </dl>
      <p class="muted small">
        Email 加密保存，只在帳號有異常時用來聯絡。<RouterLink to="/privacy">隱私權政策</RouterLink>
      </p>
    </section>

    <section class="card block">
      <h2>登入方式</h2>
      <p class="muted small">綁定後，可以用任一種方式登入。</p>
      <ul v-if="account" class="methods">
        <li v-for="m in methods" :key="m.id" class="method">
          <span class="method-name">{{ m.name }}</span>
          <span v-if="m.linked" class="state linked">
            <span class="mi" aria-hidden="true">check_circle</span>已綁定
          </span>
          <span v-else class="state">未綁定</span>
          <button
            v-if="!m.linked"
            type="button"
            class="small-btn primary"
            :disabled="!!methodBusy"
            @click="bind(m.id)"
          >
            {{ methodBusy === m.id ? '前往中…' : '綁定' }}
          </button>
          <button
            v-else-if="linkedCount > 1"
            type="button"
            class="small-btn"
            :disabled="!!methodBusy"
            @click="unbind(m.id)"
          >
            {{ methodBusy === m.id ? '解除中' : '解除綁定' }}
          </button>
          <span v-else class="muted small">至少保留一種</span>
        </li>
      </ul>
      <span v-else class="skeleton line"></span>
      <p v-if="methodError" class="warn" role="alert">{{ methodError }}</p>
    </section>

    <section class="card block danger">
      <h2>刪除帳號</h2>
      <p>帳號、API 憑證和問卷資料會全部刪除，無法復原。</p>
      <p v-if="auth.isAdmin" class="muted">管理員帳號無法在這裡刪除。</p>
      <template v-else>
        <label class="confirm">
          <span>輸入「刪除」以確認</span>
          <input v-model="confirmText" class="input" autocomplete="off" />
        </label>
        <p v-if="deleteError" class="warn" role="alert">{{ deleteError }}</p>
        <button
          type="button"
          class="delete"
          :disabled="confirmText.trim() !== '刪除' || deleting"
          @click="remove"
        >
          <span class="mi" aria-hidden="true">delete_forever</span
          >{{ deleting ? '刪除中' : '刪除帳號' }}
        </button>
      </template>
    </section>
  </div>
</template>

<style scoped>
.block {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  max-width: 40rem;
  margin-bottom: var(--font-m);
  padding: var(--font-m);
}
h2 {
  font-size: var(--font-ms);
}
dl {
  display: grid;
  grid-template-columns: 6rem 1fr;
  gap: 0.5rem 1rem;
  margin: 0;
}
dt {
  color: var(--text-muted);
}
dd {
  margin: 0;
}
.line {
  display: inline-block;
  width: 8rem;
  height: 0.9rem;
}
.small {
  font-size: var(--font-s);
}
.small a,
.link {
  color: var(--accent);
}
.danger {
  border: 1px solid color-mix(in srgb, var(--danger-text) 50%, transparent);
}
.confirm {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-width: 16rem;
  font-size: 0.875rem;
}
.confirm span {
  color: var(--text-muted);
}
.input {
  padding: 4px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font: inherit;
}
.delete {
  display: inline-flex;
  align-items: center;
  align-self: flex-start;
  gap: 4px;
  padding: 4px 12px;
  border: 0;
  border-radius: var(--radius);
  background: var(--danger-text);
  color: #ffffff;
  font: inherit;
  cursor: pointer;
}
.delete:disabled {
  opacity: 0.45;
  cursor: default;
}
.notice {
  display: flex;
  align-items: center;
  gap: 6px;
  max-width: 40rem;
  margin-bottom: var(--font-m);
  color: var(--accent);
}
.edit {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  margin-left: 8px;
  padding: 0 6px;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  color: var(--accent);
  font: inherit;
  font-size: var(--font-s);
  cursor: pointer;
}
.edit .mi {
  font-size: 1rem;
}
.edit:hover {
  background: var(--surface-muted);
}
.rename {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.rename .input {
  flex: 1 1 10rem;
  min-width: 0;
}
.small-btn {
  padding: 4px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  cursor: pointer;
}
.small-btn.primary {
  border-color: var(--accent);
  background: var(--accent);
  color: #ffffff;
}
.small-btn:disabled {
  opacity: 0.5;
  cursor: default;
}
.methods {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.method {
  display: grid;
  grid-template-columns: 6rem 1fr auto;
  align-items: center;
  gap: 0.5rem 1rem;
  padding: 0.6rem 0;
  border-top: 1px solid var(--border);
}
.method:first-child {
  border-top: 0;
}
.method-name {
  font-weight: 600;
}
.state {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--text-muted);
  font-size: 0.875rem;
}
.state.linked {
  color: var(--accent);
}
.state .mi {
  font-size: 1rem;
}
.muted {
  color: var(--text-muted);
}
.field-error {
  flex-basis: 100%;
  margin: 0;
  font-size: var(--font-s);
}
</style>
