<script setup lang="ts">
import type { ApexOptions } from 'apexcharts'
import { computed, onMounted, ref, watch } from 'vue'
import { ApiError } from '../../api/client'
import {
  createApiClient,
  deleteApiClient,
  getApiClients,
  rotateClientSecret,
  saveApiSurvey,
  setClientEnabled,
  type ApiClient,
  type ApiClientsState,
  type ApiSurvey,
} from '../../api/me'
import ApiSurveyForm from '../../components/ApiSurveyForm.vue'
import ApiTerms from '../../components/ApiTerms.vue'
import ApexChart from '../../components/charts/ApexChart.vue'
import FillableCommand, { type CommandPart } from '../../components/FillableCommand.vue'
import { usePageTitle } from '../../composables/usePageTitle'
import { IDENTITIES, PURPOSES, PUSH, topicNames } from '../../lib/apiSurvey'
import { useDatasets } from '../../lib/datasets'

usePageTitle('API 憑證')
const { datasets } = useDatasets()
const downloadable = computed(() => datasets.value.filter((d) => d.has_downloads).length)

const state = ref<ApiClientsState | null>(null)
const error = ref('')
async function load() {
  try {
    state.value = await getApiClients()
    error.value = ''
  } catch {
    error.value = '無法載入 API 憑證，請重新整理。'
  }
}
onMounted(load)

const clients = computed(() => state.value?.clients ?? [])
const limits = computed(
  () => state.value?.limits ?? { PerDay: 600, PerMinute: 30, BytesPerDay: 300 * 1024 * 1024 },
)
const MB = 1024 * 1024
const meters = computed(() => {
  const u = state.value?.used_today ?? { requests: 0, bytes: 0 }
  const pct = (a: number, b: number) => (b > 0 ? Math.min(100, Math.round((a / b) * 100)) : 0)
  return [
    {
      label: '次數',
      used: u.requests.toString(),
      limit: limits.value.PerDay.toString(),
      unit: '次',
      pct: pct(u.requests, limits.value.PerDay),
    },
    {
      label: '流量',
      used: mb(u.bytes),
      limit: mb(limits.value.BytesPerDay),
      unit: 'MB',
      pct: pct(u.bytes, limits.value.BytesPerDay),
    },
  ]
})
function mb(bytes: number) {
  return bytes >= 100 * MB ? Math.round(bytes / MB).toString() : (bytes / MB).toFixed(1)
}
const maxClients = computed(() => state.value?.max_clients ?? 3)
const survey = computed(() => state.value?.survey ?? null)
const surveyCurrent = computed(
  () => !!survey.value && survey.value.terms_version === state.value?.terms_version,
)
const onboarding = computed(() => !!state.value && !surveyCurrent.value && !clients.value.length)

const fresh = ref<{ name: string; clientId: string; secret: string } | null>(null)
const copied = ref('')
const busy = ref(false)
const formError = ref('')

async function copy(text: string, what: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = what
  } catch {
    window.prompt('請複製', text)
  }
}

async function agree(s: ApiSurvey) {
  busy.value = true
  formError.value = ''
  try {
    await saveApiSurvey(s)
    await load()
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : '儲存失敗，請稍後再試'
  } finally {
    busy.value = false
  }
}

const newName = ref('')
async function create() {
  const name = newName.value.trim()
  if (!name) {
    error.value = '請輸入憑證名稱'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const r = await createApiClient(name)
    await load()
    fresh.value = { name: r.client.name, clientId: r.client.client_id, secret: r.client_secret }
    copied.value = ''
    newName.value = ''
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '建立失敗，請稍後再試'
  } finally {
    busy.value = false
  }
}

async function rotate(c: ApiClient) {
  if (!surveyCurrent.value) {
    editing.value = true
    return
  }
  if (!window.confirm(`「${c.name}」的舊 Secret 與已取得的 token 會立即失效，確定要重新產生嗎？`))
    return
  busy.value = true
  error.value = ''
  try {
    const secret = await rotateClientSecret(c.id)
    fresh.value = { name: c.name, clientId: c.client_id, secret }
    copied.value = ''
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '重新產生失敗，請稍後再試'
  } finally {
    busy.value = false
  }
}

async function toggle(c: ApiClient) {
  if (c.enabled && !window.confirm(`停用後「${c.name}」的 token 會立即失效，確定要停用嗎？`)) return
  busy.value = true
  error.value = ''
  try {
    await setClientEnabled(c.id, !c.enabled)
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '更新失敗，請稍後再試'
  } finally {
    busy.value = false
  }
}

async function remove(c: ApiClient) {
  if (!window.confirm(`確定要刪除「${c.name}」嗎？刪除後無法復原。`)) return
  busy.value = true
  error.value = ''
  try {
    await deleteApiClient(c.id)
    if (fresh.value?.clientId === c.client_id) fresh.value = null
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '刪除失敗，請稍後再試'
  } finally {
    busy.value = false
  }
}

const editing = ref(false)
const saved = ref(false)
async function update(s: ApiSurvey) {
  busy.value = true
  formError.value = ''
  try {
    await saveApiSurvey(s)
    await load()
    editing.value = false
    saved.value = true
  } catch (e) {
    formError.value = e instanceof ApiError ? e.message : '儲存失敗，請稍後再試'
  } finally {
    busy.value = false
  }
}

function time(iso: string | null | undefined) {
  if (!iso) return '尚未使用'
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

const usageSeries = computed(() => [
  { name: '請求次數', data: (state.value?.usage ?? []).map((u) => u.requests) },
])
const usageOptions = computed<ApexOptions>(() => ({
  chart: { type: 'bar' },
  plotOptions: { bar: { borderRadius: 2, columnWidth: '70%' } },
  colors: ['#5a9cf8'],
  dataLabels: { enabled: false },
  xaxis: {
    categories: (state.value?.usage ?? []).map((u) => u.day.slice(5).replace('-', '/')),
    labels: { rotate: 0, hideOverlappingLabels: true },
    tickAmount: 6,
  },
  yaxis: {
    min: 0,
    max: limits.value.PerDay,
    tickAmount: 4,
    labels: { formatter: (v: number) => String(Math.round(v)) },
  },
  annotations: {
    yaxis: [{ y: limits.value.PerDay, borderColor: '#ed5a5a', label: { text: '每日上限' } }],
  },
  tooltip: {
    y: {
      formatter: (v: number | null, o?: { dataPointIndex: number }) =>
        `${v} 次，${mb(state.value?.usage[o?.dataPointIndex ?? -1]?.bytes ?? 0)} MB`,
    },
  },
}))

const base = `${location.origin}/api/v1`
const ID = { key: 'client_id', placeholder: '你的 Client Id' }
const SECRET = { key: 'client_secret', placeholder: '你的 Client Secret' }
const TOKEN = { key: 'token', placeholder: '你的 Access Token' }
const fill = ref<Record<string, string>>({})
watch(
  [fresh, clients],
  () => {
    const next = { ...fill.value }
    if (fresh.value) {
      next.client_id = fresh.value.clientId
      next.client_secret = fresh.value.secret
    } else if (!next.client_id && clients.value[0]) {
      next.client_id = clients.value[0].client_id
    }
    fill.value = next
  },
  { immediate: true },
)
function maskId(id: string) {
  const parts = id.split('-')
  return parts.length > 2 ? `${parts[0]}-************-${parts[parts.length - 1]}` : id
}
function maskSecret(secret: string) {
  return secret.length > 24 ? `${secret.slice(0, 8)}-************-${secret.slice(-8)}` : secret
}
function maskFill(key: string, value: string) {
  if (fresh.value && (value === fresh.value.clientId || value === fresh.value.secret)) return value
  if (key === 'client_id') return maskId(value)
  if (key === 'client_secret') return maskSecret(value)
  return value
}
function dismissSecret() {
  fresh.value = null
  fill.value = { ...fill.value, client_secret: '' }
}
const tokenExample: CommandPart[] = [
  `curl -X POST ${base}/auth/token \\\n  -d grant_type=client_credentials \\\n  -d client_id=`,
  ID,
  ' \\\n  -d client_secret=',
  SECRET,
]
const callExample: CommandPart[] = [
  'curl -H "Authorization: Bearer ',
  TOKEN,
  `" \\\n  ${base}/datasets/aqi_obs/latest?county=63000`,
]
const lastMonth = (() => {
  const now = new Date()
  const d = new Date(now.getFullYear(), now.getMonth() - 1, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
})()
const downloadIndex: CommandPart[] = [
  'curl -H "Authorization: Bearer ',
  TOKEN,
  `" ${location.origin}/downloads/power_gen/index.json`,
]
const downloadMonth: CommandPart[] = [
  'curl -OJ -H "Authorization: Bearer ',
  TOKEN,
  `" ${location.origin}/downloads/power_gen/${lastMonth}.csv.gz`,
]
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">key</span>API 憑證</h1>
    <p v-if="error" class="warn" role="alert">{{ error }}</p>

    <div v-if="!state" class="grid">
      <div v-for="h in [14, 10]" :key="h" class="col">
        <section class="card block">
          <span class="skeleton line"></span>
          <span class="skeleton" :style="{ display: 'block', height: `${h}rem` }"></span>
        </section>
      </div>
    </div>

    <div v-else-if="onboarding" class="grid">
      <div class="col">
        <section class="card block">
          <h2><span class="step">1</span>關於開放資料 API</h2>
          <p>
            網站上的 {{ datasets.length || '60 多' }} 個資料集皆能透過 API
            取得，包含最新與歷史資料，格式為 JSON 或 CSV。欄位說明請見<RouterLink to="/developers"
              >API 文件</RouterLink
            >。
          </p>
          <p>
            身分驗證採 OAuth 2.0 Client Credentials 技術：建立憑證後，用 Client Id 與 Client Secret
            換取 Access Token，再帶著 token 呼叫正式 API。每個帳號最多可建立
            {{ maxClients }} 組憑證，額度共用。每天 {{ limits.PerDay }} 次、{{
              Math.round(limits.BytesPerDay / MB)
            }}
            MB 流量，每分鐘 {{ limits.PerMinute }} 次。
          </p>
        </section>

        <section class="card block">
          <h2><span class="step">2</span>API 使用規範</h2>
          <div class="terms-box" tabindex="0" aria-label="API 使用規範">
            <ApiTerms
              :per-day="limits.PerDay"
              :per-minute="limits.PerMinute"
              :mb-per-day="Math.round(limits.BytesPerDay / MB)"
            />
          </div>
        </section>
      </div>
      <div class="col">
        <section class="card block">
          <h2><span class="step">3</span>使用調查</h2>
          <p class="muted small">
            我們會依統計結果決定優先維護哪些資料或調整相關限制與設置，您的回答僅用於我們內部數據統計與維護。絕不會提供給第三方機構使用。
          </p>
          <ApiSurveyForm
            :initial="survey"
            submit-label="同意並繼續"
            :busy="busy"
            :error="formError"
            @submit="agree"
          />
        </section>
      </div>
    </div>

    <div v-else class="grid">
      <div class="col">
        <section class="card block">
          <h2>
            我的憑證（{{ clients.length }}/{{ maxClients }}）
            <RouterLink to="/developers" class="btn-plain"
              ><span class="mi" aria-hidden="true">description</span>API 文件</RouterLink
            >
          </h2>

          <p v-if="!surveyCurrent" class="warn small">
            使用規範已更新，請在下方重新同意後再建立憑證或重新產生 Secret。
          </p>

          <ul v-if="clients.length" class="clients">
            <li
              v-for="c in clients"
              :key="c.id"
              :class="{ off: !c.enabled, new: fresh?.clientId === c.client_id }"
            >
              <div class="client-head">
                <strong>{{ c.name }}</strong>
                <span class="badge" :class="{ off: !c.enabled }">{{
                  c.enabled ? '啟用中' : '已停用'
                }}</span>
              </div>
              <dl>
                <dt>Client Id</dt>
                <dd class="masked">
                  <code>{{
                    fresh?.clientId === c.client_id ? c.client_id : maskId(c.client_id)
                  }}</code>
                  <button
                    type="button"
                    class="icon-btn"
                    :aria-label="`複製 ${c.name} 的 Client Id`"
                    @click="copy(c.client_id, `id-${c.id}`)"
                  >
                    <span class="mi" aria-hidden="true">{{
                      copied === `id-${c.id}` ? 'check' : 'content_copy'
                    }}</span>
                  </button>
                </dd>
                <dt>Client Secret</dt>
                <dd v-if="fresh?.clientId === c.client_id" class="masked">
                  <code>{{ fresh.secret }}</code>
                  <button
                    type="button"
                    class="icon-btn"
                    :aria-label="`複製 ${c.name} 的 Client Secret`"
                    @click="copy(fresh.secret, 'secret')"
                  >
                    <span class="mi" aria-hidden="true">{{
                      copied === 'secret' ? 'check' : 'content_copy'
                    }}</span>
                  </button>
                </dd>
                <dd v-else-if="c.secret_hint" class="masked">
                  <code>{{ c.secret_hint }}</code>
                </dd>
                <dd v-else class="muted small">重新產生後顯示</dd>
                <dd v-if="fresh?.clientId === c.client_id" class="secret-note">
                  完整的 Secret 只能在現在複製，請先保存。
                  <button type="button" class="link-btn" @click="dismissSecret">我已保存</button>
                </dd>
                <dt>建立時間</dt>
                <dd>{{ time(c.created_at) }}</dd>
                <dt>最後使用</dt>
                <dd>{{ time(c.last_used_at) }}</dd>
              </dl>
              <div class="actions">
                <button type="button" class="btn-plain" :disabled="busy" @click="rotate(c)">
                  <span class="mi" aria-hidden="true">autorenew</span>重新產生 Secret
                </button>
                <button type="button" class="btn-plain" :disabled="busy" @click="toggle(c)">
                  <span class="mi" aria-hidden="true">{{
                    c.enabled ? 'block' : 'check_circle'
                  }}</span
                  >{{ c.enabled ? '停用' : '啟用' }}
                </button>
                <button type="button" class="btn-plain danger" :disabled="busy" @click="remove(c)">
                  <span class="mi" aria-hidden="true">delete</span>刪除
                </button>
              </div>
            </li>
          </ul>
          <p v-else class="muted">還沒有憑證，請先建立一組。</p>

          <form
            v-if="clients.length < maxClients && surveyCurrent"
            class="create"
            @submit.prevent="create"
          >
            <input
              v-model="newName"
              class="input"
              maxlength="30"
              placeholder="憑證名稱，例如：我的分析程式"
              aria-label="憑證名稱"
            />
            <button type="submit" class="btn-highlight" :disabled="busy">
              <span class="mi" aria-hidden="true">add</span>建立憑證
            </button>
          </form>
          <p v-else-if="clients.length >= maxClients" class="muted small">
            已達 {{ maxClients }} 組上限，請先刪除不用的憑證。
          </p>
        </section>

        <section class="card block">
          <h2>使用方式</h2>
          <p class="muted small">1. 取得 Access Token</p>
          <FillableCommand v-model="fill" :parts="tokenExample" :mask="maskFill" />
          <p class="muted small">2. 帶著 token 呼叫 API</p>
          <FillableCommand v-model="fill" :parts="callExample" />
          <p class="muted small">
            token 過期時 API 會回應 401，重新取得即可。各程式語言的範例請見
            <a href="https://github.com/dashboard-tw/SampleCode" target="_blank" rel="noopener"
              >GitHub 範例程式</a
            >。
          </p>
        </section>

        <section class="card block">
          <h2>大量下載</h2>
          <p>
            若需要長期歷史資料，可以直接下載整個月份的 <code>.csv.gz</code> 檔。每天 03:30
            更新，每下載一個檔案即算一次用量。
          </p>
          <p class="muted small">查詢可下載的月份</p>
          <FillableCommand v-model="fill" :parts="downloadIndex" />
          <p class="muted small">下載指定月份</p>
          <FillableCommand v-model="fill" :parts="downloadMonth" />
          <p class="muted small">
            共 {{ downloadable || '多' }} 個資料集提供下載，請見
            <RouterLink to="/developers/datasets">資料集目錄</RouterLink>。
          </p>
        </section>
      </div>
      <div class="col">
        <section class="card block">
          <h2>
            用量
            <RouterLink v-if="state.tier === 'partner'" to="/me/showcase" class="badge partner"
              >合作額度</RouterLink
            >
          </h2>
          <p class="muted small">所有憑證共用，每天午夜歸零。</p>
          <div class="meters">
            <div v-for="m in meters" :key="m.label" class="meter">
              <div class="meter-head">
                <span>今日{{ m.label }}</span>
                <span
                  ><strong>{{ m.used }}</strong> / {{ m.limit }} {{ m.unit }}</span
                >
              </div>
              <div
                class="bar"
                role="progressbar"
                :aria-label="`今日${m.label}`"
                :aria-valuenow="m.pct"
                aria-valuemin="0"
                aria-valuemax="100"
              >
                <span :style="{ width: `${m.pct}%` }" :class="{ high: m.pct >= 80 }"></span>
              </div>
            </div>
          </div>
          <h3>近 30 天</h3>
          <div class="chart">
            <ApexChart type="bar" :series="usageSeries" :options="usageOptions" height="100%" />
          </div>
        </section>
        <section class="card block">
          <h2>
            使用資料
            <button
              v-if="!editing && surveyCurrent"
              type="button"
              class="btn-plain"
              @click="((editing = true), (saved = false))"
            >
              <span class="mi" aria-hidden="true">edit</span>更新
            </button>
          </h2>
          <template v-if="editing || !surveyCurrent">
            <details class="terms-details" :open="!surveyCurrent">
              <summary>API 使用規範</summary>
              <ApiTerms
                :per-day="limits.PerDay"
                :per-minute="limits.PerMinute"
                :mb-per-day="Math.round(limits.BytesPerDay / MB)"
              />
            </details>
            <ApiSurveyForm
              :initial="survey"
              submit-label="儲存"
              :busy="busy"
              :error="formError"
              @submit="update"
            />
          </template>
          <dl v-else-if="survey">
            <dt>身分類別</dt>
            <dd>{{ IDENTITIES[survey.identity] ?? survey.identity }}</dd>
            <dt>資料用途</dt>
            <dd>{{ PURPOSES[survey.purpose] ?? survey.purpose }}</dd>
            <dt>關注的資料</dt>
            <dd>{{ topicNames(survey.topics) }}</dd>
            <dt>應用名稱</dt>
            <dd>{{ survey.app_name || '—' }}</dd>
            <dt>應用連結</dt>
            <dd>{{ survey.app_url || '—' }}</dd>
            <dt>MQTT 即時推送</dt>
            <dd>{{ survey.wants_push ? PUSH[survey.wants_push] : '—' }}</dd>
            <dt>建議</dt>
            <dd class="pre">{{ survey.suggestion || '—' }}</dd>
          </dl>
          <p v-if="saved" class="ok small" role="status">已儲存</p>
        </section>
      </div>
    </div>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--font-m);
  align-items: start;
}
@media (min-width: 68.75em) {
  .grid {
    grid-template-columns: minmax(0, 3fr) minmax(0, 2fr);
  }
}
.col {
  display: flex;
  flex-direction: column;
  gap: var(--font-m);
  min-width: 0;
}
.block {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: var(--font-m);
  line-height: 1.7;
}
h3 {
  margin: 0.25rem 0 0;
  color: var(--text-secondary);
  font-size: 0.875rem;
  font-weight: normal;
}
.meters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
  gap: 0.75rem 1.25rem;
}
.meter-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 4px;
  font-size: 0.875rem;
}
.meter-head strong {
  font-size: var(--font-ms);
}
.bar {
  height: 6px;
  overflow: hidden;
  border-radius: 3px;
  background: var(--border);
}
.bar span {
  display: block;
  height: 100%;
  border-radius: 3px;
  background: var(--accent);
  transition: width 0.4s ease;
}
.bar span.high {
  background: var(--danger-text);
}
h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--font-ms);
}
.step {
  display: inline-grid;
  place-items: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 50%;
  background: var(--accent);
  color: #ffffff;
  font-size: 0.8125rem;
}
.datasets {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 0.875rem;
}
.datasets li {
  display: grid;
  grid-template-columns: 11rem 7rem 1fr;
  gap: 8px;
}
.terms-box {
  max-height: 18rem;
  overflow-y: auto;
  padding: 0.75rem 1rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
}
.terms-details summary {
  color: var(--accent);
  cursor: pointer;
}
.terms-details[open] {
  padding: 0.75rem 1rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
}
dl {
  display: grid;
  grid-template-columns: 7rem 1fr;
  gap: 0.5rem 1rem;
  margin: 0;
}
dt {
  color: var(--text-muted);
}
dd {
  margin: 0;
}
.pre {
  white-space: pre-wrap;
}
code {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 0.8125rem;
}
.actions {
  display: flex;
  gap: 8px;
}
.btn-plain {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: none;
  color: var(--text-primary);
  font: inherit;
  font-size: 0.875rem;
  cursor: pointer;
}
.btn-plain:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}
.btn-plain .mi,
.btn-highlight .mi {
  font-size: 1.1em;
}
.chart {
  height: 220px;
}
.line {
  display: block;
  width: 40%;
  height: 1rem;
}
.ok {
  color: #4ade80;
}
:root[data-theme='light'] .ok {
  color: #1a7f37;
}
.small {
  font-size: var(--font-s);
}
@media (max-width: 47.4375em) {
  .datasets li {
    grid-template-columns: 1fr;
    gap: 0;
    margin-bottom: 6px;
  }
}
.clients {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
}
.clients li {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.9rem 0;
  border-top: 1px solid var(--border);
}
.masked code {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--surface-muted);
  word-break: break-all;
}
.secret-note {
  grid-column: 2;
  color: var(--warning-text, var(--text-secondary));
  font-size: var(--font-s);
}
.link-btn {
  margin-left: 6px;
  padding: 0;
  border: 0;
  background: none;
  color: var(--accent);
  font: inherit;
  text-decoration: underline;
  cursor: pointer;
}
.clients li.off dl {
  opacity: 0.6;
}
.client-head {
  display: flex;
  align-items: center;
  gap: 8px;
}
.badge {
  padding: 0 8px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: var(--font-s);
}
.badge.partner {
  font-weight: 400;
  text-decoration: none;
}
.badge.off {
  background: var(--surface-muted);
  color: var(--text-muted);
}
.icon-btn {
  margin-left: 4px;
  padding: 0 4px;
  border: 0;
  background: none;
  color: var(--text-muted);
  vertical-align: middle;
  cursor: pointer;
}
.icon-btn:hover {
  color: var(--accent);
}
.icon-btn .mi {
  font-size: 1rem;
}
.btn-plain.danger:hover:not(:disabled) {
  border-color: var(--danger-text);
  color: var(--danger-text);
}
.actions {
  flex-wrap: wrap;
}
.create {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.create .input {
  flex: 1 1 14rem;
  padding: 4px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font: inherit;
}
</style>
