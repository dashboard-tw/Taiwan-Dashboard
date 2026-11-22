<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  getAdminUser,
  revokeUserKey,
  searchUsersByEmail,
  updateAdminUser,
  type AdminUser,
  type AdminUserDetail,
} from '../../api/admin'
import { ApiError } from '../../api/client'
import { usePageTitle } from '../../composables/usePageTitle'
import { IDENTITIES, PURPOSES, PUSH } from '../../lib/apiSurvey'
import { formatNumber } from '../../lib/format'

usePageTitle('用戶管理')
const route = useRoute()
const router = useRouter()

const email = ref('')
const searching = ref(false)
const results = ref<AdminUser[] | null>(null)
const error = ref('')
const message = ref<{ ok: boolean; text: string } | null>(null)

async function search() {
  const q = email.value.trim()
  if (!q) return
  searching.value = true
  error.value = ''
  message.value = null
  try {
    results.value = await searchUsersByEmail(q)
    if (results.value.length === 1) open(results.value[0]!.id)
  } catch (e) {
    results.value = null
    error.value = e instanceof ApiError ? e.message : '搜尋失敗'
  } finally {
    searching.value = false
  }
}

const selected = computed(() => {
  const id = Number(route.query.id)
  return Number.isInteger(id) && id > 0 ? id : null
})
function open(id: number) {
  void router.replace({ query: { id: String(id) } })
}

const detail = ref<AdminUserDetail | null>(null)
const loading = ref(false)
async function loadDetail() {
  detail.value = null
  if (!selected.value) return
  loading.value = true
  try {
    detail.value = await getAdminUser(selected.value)
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '帳號資料暫時無法取得'
  } finally {
    loading.value = false
  }
}
watch(selected, loadDetail, { immediate: true })

const busy = ref(false)
async function change(label: string, confirmText: string, fn: () => Promise<void>) {
  if (!window.confirm(confirmText)) return
  busy.value = true
  message.value = null
  try {
    await fn()
    message.value = { ok: true, text: label }
    await loadDetail()
  } catch (e) {
    message.value = { ok: false, text: e instanceof ApiError ? e.message : '修改失敗' }
  } finally {
    busy.value = false
  }
}
function toggleAdmin(d: AdminUserDetail) {
  const name = d.user.display_name
  const toAdmin = d.user.role !== 'admin'
  void change(
    toAdmin ? `已將「${name}」設為管理員` : `已取消「${name}」的管理員`,
    toAdmin
      ? `將「${name}」設為管理員？對方可以進入後台，修改資料與元件。`
      : `取消「${name}」的管理員？對方將無法進入後台。`,
    () => updateAdminUser(d.user.id, { role: toAdmin ? 'admin' : 'user' }),
  )
}
function toggleStatus(d: AdminUserDetail) {
  const name = d.user.display_name
  const disable = d.user.status === 'active'
  void change(
    disable ? `已停用「${name}」` : `已啟用「${name}」`,
    disable
      ? `停用「${name}」？對方會在所有裝置登出，API 憑證也會立即失效。`
      : `重新啟用「${name}」？`,
    () => updateAdminUser(d.user.id, { status: disable ? 'disabled' : 'active' }),
  )
}
function revokeClients(d: AdminUserDetail) {
  const name = d.user.display_name
  void change(
    `已刪除「${name}」的所有 API 憑證`,
    `刪除「${name}」的所有 API 憑證？使用中的程式會立即失效，對方之後可以重新建立。`,
    () => revokeUserKey(d.user.id),
  )
}

const PROVIDERS: Record<string, string> = {
  github: 'GitHub',
  google: 'Google',
  apple: 'Apple',
  gov: '政府登入',
  dev: '開發用',
}
const ACTIONS: Record<string, string> = {
  login: '登入',
  logout: '登出',
  'client.create': '建立 API 憑證',
  'client.rotate': '重新產生 Secret',
  'client.disable': '停用 API 憑證',
  'client.enable': '啟用 API 憑證',
  'client.delete': '刪除 API 憑證',
  'admin.user.update': '管理員修改帳號',
  'admin.apikey.revoke': '管理員刪除 API 憑證',
}

function time(iso: string | null, empty = '—') {
  if (!iso) return empty
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
function mb(bytes: number) {
  return formatNumber(bytes / 1024 / 1024, 1)
}
</script>

<template>
  <div>
    <h1 class="page-title"><span class="mi" aria-hidden="true">group</span>用戶管理</h1>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>
    <p v-if="message" :class="['notice', { bad: !message.ok }]" role="status">
      {{ message.text }}
    </p>

    <section class="card block">
      <h2>用 Email 搜尋</h2>
      <form class="search" @submit.prevent="search">
        <input
          v-model="email"
          class="input"
          type="email"
          placeholder="user@example.com"
          aria-label="用戶 Email"
          autocomplete="off"
          required
        />
        <button type="submit" class="btn-highlight" :disabled="searching || !email.trim()">
          <span class="mi" aria-hidden="true">search</span>{{ searching ? '搜尋中' : '搜尋' }}
        </button>
      </form>
      <p class="muted small">Email 會先在瀏覽器轉成雜湊值再送出，伺服器不會收到明文。</p>

      <p v-if="results && !results.length" class="muted">找不到使用這個 Email 的帳號。</p>
      <table v-else-if="results && results.length > 1">
        <thead>
          <tr>
            <th>名稱</th>
            <th>角色</th>
            <th>狀態</th>
            <th>建立時間</th>
            <th>最後登入</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="u in results"
            :key="u.id"
            class="clickable"
            :class="{ active: selected === u.id }"
            @click="open(u.id)"
          >
            <td>
              {{ u.display_name }}<span class="sub">#{{ u.id }}</span>
            </td>
            <td>{{ u.role === 'admin' ? '管理員' : '一般' }}</td>
            <td :class="u.status === 'active' ? 'ok' : 'bad'">
              {{ u.status === 'active' ? '使用中' : '已停用' }}
            </td>
            <td>{{ time(u.created_at) }}</td>
            <td>{{ time(u.last_login_at) }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="loading" class="card block">
      <span class="skeleton cell"></span>
    </section>

    <template v-else-if="detail">
      <section class="card block">
        <div class="head">
          <h2>
            {{ detail.user.display_name }}
            <span class="badge" :class="detail.user.role === 'admin' ? 'warn' : ''">{{
              detail.user.role === 'admin' ? '管理員' : '一般帳號'
            }}</span>
            <span class="badge" :class="detail.user.status === 'active' ? 'ok' : 'bad'">{{
              detail.user.status === 'active' ? '使用中' : '已停用'
            }}</span>
            <span v-if="detail.is_me" class="badge">你自己</span>
          </h2>
          <div class="actions">
            <button
              type="button"
              class="btn"
              :disabled="busy || detail.is_me"
              @click="toggleAdmin(detail)"
            >
              <span class="mi" aria-hidden="true">{{
                detail.user.role === 'admin' ? 'remove_moderator' : 'shield_person'
              }}</span
              >{{ detail.user.role === 'admin' ? '取消管理員' : '設為管理員' }}
            </button>
            <button
              type="button"
              class="btn"
              :class="{ bad: detail.user.status === 'active' }"
              :disabled="busy || detail.is_me"
              @click="toggleStatus(detail)"
            >
              <span class="mi" aria-hidden="true">{{
                detail.user.status === 'active' ? 'block' : 'check_circle'
              }}</span
              >{{ detail.user.status === 'active' ? '停用帳號' : '重新啟用' }}
            </button>
          </div>
        </div>
        <p v-if="detail.is_me" class="muted small">不能修改自己的角色或狀態。</p>

        <dl class="facts">
          <div>
            <dt>用戶代碼</dt>
            <dd>#{{ detail.user.id }}</dd>
          </div>
          <div>
            <dt>建立時間</dt>
            <dd>{{ time(detail.user.created_at) }}</dd>
          </div>
          <div>
            <dt>最後登入</dt>
            <dd>{{ time(detail.user.last_login_at) }}</dd>
          </div>
          <div>
            <dt>最後活動</dt>
            <dd>{{ time(detail.last_seen_at) }}</dd>
          </div>
          <div>
            <dt>登入中的裝置</dt>
            <dd>{{ detail.active_sessions }} 台</dd>
          </div>
          <div>
            <dt>Email</dt>
            <dd>{{ detail.has_email ? '已加密保存' : '沒有保存' }}</dd>
          </div>
          <div>
            <dt>登入方式</dt>
            <dd>
              {{
                detail.providers.map((p) => PROVIDERS[p.provider] ?? p.provider).join('、') || '—'
              }}
            </dd>
          </div>
        </dl>
      </section>

      <div class="two">
        <section class="card block">
          <h2>
            API 憑證
            <button
              v-if="detail.clients.length"
              type="button"
              class="btn bad"
              :disabled="busy"
              @click="revokeClients(detail)"
            >
              <span class="mi" aria-hidden="true">delete</span>全部刪除
            </button>
          </h2>
          <dl class="facts">
            <div>
              <dt>今天</dt>
              <dd>
                {{ formatNumber(detail.usage_today.requests) }} 次，{{
                  mb(detail.usage_today.bytes)
                }}
                MB
              </dd>
            </div>
            <div>
              <dt>近 30 天</dt>
              <dd>
                {{ formatNumber(detail.usage_30d.requests) }} 次，{{ mb(detail.usage_30d.bytes) }}
                MB
              </dd>
            </div>
          </dl>
          <p v-if="!detail.clients.length" class="muted">沒有 API 憑證。</p>
          <table v-else>
            <thead>
              <tr>
                <th>名稱</th>
                <th>狀態</th>
                <th>建立</th>
                <th>最後使用</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="k in detail.clients" :key="k.client_id">
                <td>
                  {{ k.name
                  }}<span class="sub"
                    ><code>{{ k.client_id }}</code></span
                  >
                </td>
                <td :class="k.enabled ? 'ok' : 'bad'">{{ k.enabled ? '啟用' : '停用' }}</td>
                <td>{{ time(k.created_at) }}</td>
                <td>{{ time(k.last_used_at, '從未使用') }}</td>
              </tr>
            </tbody>
          </table>

          <h2 class="gap">使用調查</h2>
          <p v-if="!detail.survey" class="muted">還沒有填寫。</p>
          <dl v-else class="facts">
            <div>
              <dt>身分</dt>
              <dd>{{ IDENTITIES[detail.survey.identity] ?? detail.survey.identity }}</dd>
            </div>
            <div>
              <dt>用途</dt>
              <dd>{{ PURPOSES[detail.survey.purpose] ?? detail.survey.purpose }}</dd>
            </div>
            <div>
              <dt>MQTT 即時推送</dt>
              <dd>{{ detail.survey.wants_push ? PUSH[detail.survey.wants_push] : '—' }}</dd>
            </div>
            <div>
              <dt>更新時間</dt>
              <dd>{{ time(detail.survey.updated_at) }}</dd>
            </div>
          </dl>
        </section>

        <section class="card block">
          <h2>最近紀錄</h2>
          <p v-if="!detail.events.length" class="muted">沒有紀錄。</p>
          <table v-else>
            <thead>
              <tr>
                <th>時間</th>
                <th>動作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(ev, i) in detail.events" :key="i">
                <td class="nowrap">{{ time(ev.created_at) }}</td>
                <td>
                  {{ ACTIONS[ev.action] ?? ev.action
                  }}<span v-if="ev.target" class="sub">{{ ev.target }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </section>
      </div>
    </template>
  </div>
</template>

<style scoped>
.search {
  display: flex;
  gap: 8px;
  max-width: 32rem;
  margin-bottom: 6px;
}
.search .input {
  flex: 1;
  min-width: 0;
  padding: 6px 10px;
}
.small {
  font-size: var(--font-s);
}
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px 16px;
}
.head h2 {
  flex-wrap: wrap;
  gap: 8px;
}
.actions {
  display: flex;
  gap: 8px;
}
.btn.bad {
  color: var(--danger-text);
}
.facts {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(11rem, 1fr));
  gap: 12px 24px;
  margin: 8px 0 12px;
}
.facts dt {
  color: var(--text-muted);
  font-size: var(--font-s);
}
.facts dd {
  margin: 2px 0 0;
  font-size: 0.9375rem;
}
.two {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  gap: var(--font-m);
  align-items: start;
}
.two .block {
  margin-bottom: 0;
}
.gap {
  margin-top: 1.25rem;
}
.nowrap {
  white-space: nowrap;
}
code {
  font-size: 0.8rem;
}
@media (max-width: 68.75em) {
  .two {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
