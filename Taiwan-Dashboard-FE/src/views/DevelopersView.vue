<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { getCatalog, type Catalog, type CatalogDataset } from '../api/catalog'
import CodeBlock from '../components/developers/CodeBlock.vue'
import DatasetDetail from '../components/developers/DatasetDetail.vue'
import EndpointDoc from '../components/developers/EndpointDoc.vue'
import UrlBar from '../components/developers/UrlBar.vue'
import TaiwanLogo from '../components/layout/TaiwanLogo.vue'
import { usePageTitle } from '../composables/usePageTitle'
import { ENDPOINTS, endpointBySlug } from '../lib/apiReference'
import { formatFreq } from '../lib/format'
import { useAuth } from '../stores/auth'
import { cycleTheme, themeChoice } from '../theme'

const auth = useAuth()
void auth.load()
const route = useRoute()
const router = useRouter()

const catalog = ref<Catalog | null>(null)
const loadError = ref(false)
async function load() {
  try {
    catalog.value = await getCatalog()
    loadError.value = false
  } catch {
    loadError.value = true
    setTimeout(load, 5000)
  }
}
onMounted(load)

const origin = typeof window === 'undefined' ? 'https://dashboard.tw' : window.location.origin
const apiBase = `${origin}/api/v1`
const PER_DAY = 600
const PER_MINUTE = 30
const MB_PER_DAY = 300
const PARTNER_PER_DAY = 3000
const SAMPLES_URL = 'https://github.com/dashboard-tw/SampleCode'

const datasets = computed(() => catalog.value?.datasets ?? [])
const topics = computed(() => catalog.value?.topics ?? [])
const topicName = (id: string) => topics.value.find((t) => t.id === id)?.name ?? ''

const GUIDES = [
  { slug: '', label: '快速開始', icon: 'rocket_launch' },
  { slug: 'auth', label: '驗證與額度', icon: 'key' },
  { slug: 'paging', label: '分頁與減少流量', icon: 'layers' },
  { slug: 'errors', label: '錯誤', icon: 'error' },
  { slug: 'license', label: '授權與引用', icon: 'gavel' },
]
const section = computed(() => String(route.params.section ?? ''))
const item = computed(() => String(route.params.item ?? ''))
const endpoint = computed(() =>
  section.value === 'endpoints' ? endpointBySlug(item.value) : undefined,
)
const dataset = computed(() =>
  section.value === 'datasets' && item.value
    ? datasets.value.find((d) => d.id === item.value)
    : undefined,
)
const page = computed(() => {
  const s = section.value
  if (GUIDES.some((g) => g.slug === s)) return 'guide'
  if (endpoint.value) return 'endpoint'
  if (s === 'datasets' && !item.value) return 'catalog'
  if (dataset.value) return 'dataset'
  if (s === 'datasets' && !catalog.value) return 'loading'
  return 'missing'
})
const guide = computed(() => GUIDES.find((g) => g.slug === section.value))
const crumbs = computed(() => {
  if (page.value === 'guide') return ['說明']
  if (page.value === 'endpoint') return ['端點']
  if (page.value === 'dataset') return ['資料集', topicName(dataset.value!.topic)]
  return []
})
const title = computed(() => {
  if (page.value === 'guide') return guide.value!.label
  if (page.value === 'endpoint') return endpoint.value!.title
  if (page.value === 'catalog') return '資料集目錄'
  if (page.value === 'dataset') return dataset.value!.name
  return ''
})
usePageTitle(computed(() => (title.value ? `${title.value}｜開放資料 API` : '開放資料 API')))

const LEGACY: Record<string, string> = {
  start: '',
  auth: 'auth',
  endpoints: 'endpoints/latest',
  paging: 'paging',
  errors: 'errors',
  license: 'license',
  datasets: 'datasets',
}
watch(
  () => route.hash,
  (hash) => {
    if (section.value) return
    const m = /^#ds-(\w+)$/.exec(hash)
    const to = m ? `datasets/${m[1]}` : LEGACY[hash.slice(1)]
    if (to !== undefined && hash) void router.replace(`/developers${to ? `/${to}` : ''}`)
  },
  { immediate: true },
)

const navOpen = ref(false)
const navQuery = ref('')
const q = computed(() => navQuery.value.trim().toLowerCase())
const hit = (...parts: string[]) => !q.value || parts.join(' ').toLowerCase().includes(q.value)
const navGuides = computed(() => GUIDES.filter((g) => hit(g.label)))
const navEndpoints = computed(() => ENDPOINTS.filter((e) => hit(e.title, e.path, e.method)))
const navTopics = computed(() =>
  topics.value
    .map((t) => ({
      ...t,
      items: datasets.value.filter((d) => d.topic === t.id && hit(d.name, d.id, t.name)),
    }))
    .filter((t) => t.items.length),
)
const expanded = ref<Record<string, boolean>>({})
const isOpen = (id: string) => !!q.value || !!expanded.value[id]
watch(
  dataset,
  (d) => {
    if (d) expanded.value[d.topic] = true
  },
  { immediate: true },
)
watch(
  () => route.fullPath,
  () => (navOpen.value = false),
)

const query = ref('')
const topic = ref('')
const countByTopic = computed(() => {
  const m: Record<string, number> = {}
  for (const d of datasets.value) m[d.topic] = (m[d.topic] ?? 0) + 1
  return m
})
function matches(d: CatalogDataset, text: string): boolean {
  if (!text) return true
  const hay = [d.id, d.name, d.description, d.agency, ...d.latest_fields.map((f) => f.name)]
    .join(' ')
    .toLowerCase()
  return text
    .toLowerCase()
    .split(/\s+/)
    .every((w) => hay.includes(w))
}
const groups = computed(() =>
  topics.value
    .map((t) => ({
      ...t,
      items: datasets.value.filter(
        (d) =>
          d.topic === t.id &&
          (!topic.value || d.topic === topic.value) &&
          matches(d, query.value.trim()),
      ),
    }))
    .filter((g) => g.items.length),
)
function summary(text: string): string {
  const i = text.indexOf('。')
  return i > 0 ? text.slice(0, i + 1) : text
}

const themeIcon = computed(() =>
  themeChoice.value === 'light'
    ? 'light_mode'
    : themeChoice.value === 'dark'
      ? 'dark_mode'
      : 'contrast',
)

const quickstart = computed(() => ({
  curl: `# 1. 以 Client Id 與 Client Secret 換取 Access Token
curl -X POST "${apiBase}/auth/token" \\
  -d grant_type=client_credentials \\
  -d client_id="$DASHBOARD_TW_CLIENT_ID" \\
  -d client_secret="$DASHBOARD_TW_CLIENT_SECRET"
# 回應：{"access_token":"...","token_type":"Bearer","expires_in":86400}

# 2. 帶著 token 呼叫 API
curl "${apiBase}/datasets/aqi_obs/latest?county=63000" \\
  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN"`,
  JavaScript: `// 1. 以 Client Id 與 Client Secret 換取 Access Token
const tokenRes = await fetch("${apiBase}/auth/token", {
  method: "POST",
  body: new URLSearchParams({
    grant_type: "client_credentials",
    client_id: process.env.DASHBOARD_TW_CLIENT_ID,
    client_secret: process.env.DASHBOARD_TW_CLIENT_SECRET,
  }),
})
const { access_token } = await tokenRes.json()

// 2. 帶著 token 呼叫 API（token 有效 24 小時，請重複使用）
const res = await fetch("${apiBase}/datasets/aqi_obs/latest?county=63000", {
  headers: { Authorization: \`Bearer \${access_token}\` },
})
const { data, meta } = await res.json()
console.log(meta.source.agency, data.length, "個測站")`,
  Python: `import os, requests

# 1. 以 Client Id 與 Client Secret 換取 Access Token
token = requests.post(
    "${apiBase}/auth/token",
    data={
        "grant_type": "client_credentials",
        "client_id": os.environ["DASHBOARD_TW_CLIENT_ID"],
        "client_secret": os.environ["DASHBOARD_TW_CLIENT_SECRET"],
    },
).json()["access_token"]

# 2. 帶著 token 呼叫 API（token 有效 24 小時，請重複使用）
res = requests.get(
    "${apiBase}/datasets/aqi_obs/latest",
    params={"county": "63000"},
    headers={"Authorization": f"Bearer {token}"},
)
body = res.json()
print(body["meta"]["source"]["agency"], len(body["data"]), "個測站")`,
}))

const paging = `# 第一頁
curl "${apiBase}/datasets/rain_obs/history?from=2026-11-01&to=2026-11-08" \\
  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN"

# 回應的 meta.next_cursor 有值時，帶上它取得下一頁
curl "${apiBase}/datasets/rain_obs/history?from=2026-11-01&to=2026-11-08&cursor=eyJ0Ijoi..." \\
  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN"`

const conditional = `# 第一次請求，記下回應的 ETag
curl -i "${apiBase}/datasets/aqi_obs/latest" \\
  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN"

# 之後帶上 ETag，資料沒變會回應 304
curl -i "${apiBase}/datasets/aqi_obs/latest" \\
  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN" \\
  -H 'If-None-Match: W/"..."'`

const ERRORS = [
  { code: 400, name: 'invalid_*', text: '參數錯誤' },
  { code: 401, name: 'invalid_token', text: '缺少 Access Token，或 token 無效、已過期' },
  { code: 401, name: 'invalid_client', text: '換取 token 時 Client Id 或 Secret 錯誤' },
  { code: 404, name: 'dataset_not_found', text: '找不到資料集或端點' },
  { code: 429, name: 'rate_limited', text: '每分鐘的請求太多，請依 Retry-After 稍後再試' },
  { code: 429, name: 'quota_exceeded', text: '今天的次數已用完，午夜後恢復' },
  { code: 429, name: 'traffic_exceeded', text: '今天的流量已用完，午夜後恢復' },
]

const HEADERS = [
  { name: 'X-RateLimit-Limit', text: '每天可呼叫的次數' },
  { name: 'X-RateLimit-Remaining', text: '今天剩下的次數' },
  { name: 'X-Traffic-Limit', text: '每天可用的流量，單位是位元組' },
  { name: 'X-Traffic-Used', text: '今天已用的流量' },
  { name: 'Retry-After', text: '超過上限時，幾秒後可以再試' },
]
</script>

<template>
  <div class="dev">
    <a class="skip" href="#dev-main">跳到主要內容</a>
    <header class="top">
      <div class="top-inner">
        <button
          type="button"
          class="icon-btn menu"
          :aria-expanded="navOpen"
          aria-controls="dev-nav"
          aria-label="文件目錄"
          @click="navOpen = !navOpen"
        >
          <span class="mi" aria-hidden="true">{{ navOpen ? 'close' : 'menu' }}</span>
        </button>
        <div class="brand">
          <RouterLink to="/" class="brand-home" aria-label="回到 dashboard.tw 首頁">
            <TaiwanLogo class="logo" />
            <span class="brand-name">dashboard.tw</span>
          </RouterLink>
          <span class="brand-sep" aria-hidden="true">/</span>
          <RouterLink to="/developers" class="brand-page">開放資料 API</RouterLink>
        </div>
        <nav class="top-links" aria-label="頁面連結">
          <RouterLink to="/" class="link hide-sm">儀表板</RouterLink>
          <a href="/api/v1/openapi.json" class="link hide-sm" target="_blank" rel="noopener"
            >OpenAPI</a
          >
          <a :href="SAMPLES_URL" class="link hide-sm" target="_blank" rel="noopener">範例程式</a>
          <button type="button" class="icon-btn" aria-label="切換深淺色" @click="cycleTheme">
            <span class="mi" aria-hidden="true">{{ themeIcon }}</span>
          </button>
          <RouterLink :to="auth.me ? '/me/api-key' : '/login?redirect=/me/api-key'" class="cta">
            {{ auth.me ? '我的 API 憑證' : '申請 API 憑證' }}
          </RouterLink>
        </nav>
      </div>
    </header>

    <div class="layout">
      <aside id="dev-nav" class="side" :class="{ open: navOpen }" aria-label="文件目錄">
        <nav>
          <label class="nav-search">
            <span class="mi" aria-hidden="true">search</span>
            <input v-model="navQuery" type="search" placeholder="搜尋文件" aria-label="搜尋文件" />
          </label>

          <template v-if="navGuides.length">
            <p class="side-title">說明</p>
            <RouterLink
              v-for="g in navGuides"
              :key="g.slug"
              :to="g.slug ? `/developers/${g.slug}` : '/developers'"
              class="nav-link"
              :class="{ on: page === 'guide' && section === g.slug }"
              >{{ g.label }}</RouterLink
            >
          </template>

          <template v-if="navEndpoints.length">
            <p class="side-title">端點</p>
            <RouterLink
              v-for="e in navEndpoints"
              :key="e.slug"
              :to="`/developers/endpoints/${e.slug}`"
              class="nav-link"
              :class="{ on: endpoint?.slug === e.slug }"
            >
              <span>{{ e.title }}</span>
              <span :class="['verb', e.method.toLowerCase()]">{{ e.method }}</span>
            </RouterLink>
          </template>

          <p class="side-title">資料集</p>
          <RouterLink
            v-if="!q"
            to="/developers/datasets"
            class="nav-link"
            :class="{ on: page === 'catalog' }"
            >全部資料集<span class="count">{{ datasets.length || '' }}</span></RouterLink
          >
          <div v-for="t in navTopics" :key="t.id" class="nav-group">
            <button
              type="button"
              class="nav-link group-btn"
              :aria-expanded="isOpen(t.id)"
              @click="expanded[t.id] = !expanded[t.id]"
            >
              <span>{{ t.name }}</span>
              <span class="mi chev" :class="{ up: isOpen(t.id) }" aria-hidden="true"
                >expand_more</span
              >
            </button>
            <div v-if="isOpen(t.id)" class="nav-sub">
              <RouterLink
                v-for="d in t.items"
                :key="d.id"
                :to="`/developers/datasets/${d.id}`"
                class="nav-link"
                :class="{ on: dataset?.id === d.id }"
                >{{ d.name }}</RouterLink
              >
            </div>
          </div>
          <p
            v-if="q && !navGuides.length && !navEndpoints.length && !navTopics.length"
            class="muted small nav-empty"
          >
            找不到「{{ navQuery }}」。
          </p>
        </nav>
      </aside>
      <button
        v-if="navOpen"
        type="button"
        class="scrim"
        aria-label="關閉目錄"
        @click="navOpen = false"
      ></button>

      <main id="dev-main" class="main" tabindex="-1">
        <nav v-if="crumbs.length" class="crumbs" aria-label="位置">
          <RouterLink to="/developers">開放資料 API</RouterLink>
          <template v-for="c in crumbs" :key="c">
            <span class="mi" aria-hidden="true">chevron_right</span><span>{{ c }}</span>
          </template>
        </nav>

        <template v-if="page === 'guide' && section === ''">
          <section class="hero">
            <p class="eyebrow">開放資料 API</p>
            <h1>臺灣儀表板的資料，<br />都能用 API 取得</h1>
            <p class="lead">
              整合各機關的政府開放資料，統一格式並保留歷史紀錄，免費提供研究、新聞與開發使用。
            </p>
            <dl class="stats">
              <div>
                <dt>資料集</dt>
                <dd>{{ datasets.length || '—' }}</dd>
              </div>
              <div>
                <dt>端點</dt>
                <dd>{{ ENDPOINTS.length }}</dd>
              </div>
              <div>
                <dt>每天免費</dt>
                <dd>{{ PER_DAY }} 次</dd>
              </div>
              <div>
                <dt>格式</dt>
                <dd>JSON・CSV</dd>
              </div>
            </dl>
          </section>

          <section class="sec">
            <h2>三個步驟開始</h2>
            <ol class="steps">
              <li>
                <span class="num">1</span>
                <div>
                  <h3>建立 API 憑證</h3>
                  <p>
                    登入後到
                    <RouterLink :to="auth.me ? '/me/api-key' : '/login?redirect=/me/api-key'"
                      >我的 API 憑證</RouterLink
                    >
                    免費申請。Client Secret 只會顯示一次，請妥善保存。
                  </p>
                </div>
              </li>
              <li>
                <span class="num">2</span>
                <div>
                  <h3>送出第一個請求</h3>
                  <div class="split">
                    <div>
                      <p>
                        先換取 Access Token，再放在 <code>Authorization</code>
                        標頭。以查詢臺北市各測站的 AQI 為例。
                      </p>
                      <p class="more-samples">
                        Rust、Go、C、C++、Java、C#、PHP 等 15 種語言的完整範例，請見
                        <a :href="SAMPLES_URL" target="_blank" rel="noopener">GitHub 範例程式</a>。
                      </p>
                    </div>
                    <CodeBlock :code="quickstart" />
                  </div>
                </div>
              </li>
              <li>
                <span class="num">3</span>
                <div>
                  <h3>挑選資料</h3>
                  <p>
                    每個端點的參數與回應見左側目錄。資料集的欄位與呼叫範例請見
                    <RouterLink to="/developers/datasets">資料集目錄</RouterLink>。
                  </p>
                </div>
              </li>
            </ol>
          </section>
        </template>

        <template v-else-if="page === 'guide' && section === 'auth'">
          <h1 class="page-h1">驗證與額度</h1>
          <section class="sec">
            <div class="cards">
              <div class="card">
                <span class="mi" aria-hidden="true">key</span>
                <h2>OAuth 2.0</h2>
                <p>
                  以 Client Credentials 換取 Access Token，放在
                  <code>Authorization: Bearer</code> 標頭。token 只能讀取資料。
                </p>
              </div>
              <div class="card">
                <span class="mi" aria-hidden="true">speed</span>
                <h2>額度</h2>
                <p>
                  每天 {{ PER_DAY }} 次、{{ MB_PER_DAY }} MB 流量，每分鐘
                  {{ PER_MINUTE }} 次，每天午夜歸零。應用上架到<RouterLink to="/showcase/app"
                    >應用活化</RouterLink
                  >後，每天可用 {{ PARTNER_PER_DAY }} 次。
                </p>
              </div>
              <div class="card">
                <span class="mi" aria-hidden="true">public</span>
                <h2>跨網域</h2>
                <p>支援 CORS。請勿把 Client Secret 放在公開的網頁程式碼中。</p>
              </div>
            </div>
          </section>
          <section class="sec">
            <h2>取得 Access Token</h2>
            <p class="sec-lead">
              每個帳號最多 3 組憑證，額度共用。token 有效 24 小時，請重複使用，過期再重新取得。
              參數與回應見
              <RouterLink to="/developers/endpoints/token">取得 Access Token</RouterLink>。
            </p>
          </section>
          <section class="sec">
            <h2>回應標頭</h2>
            <p class="sec-lead">每個回應都會告訴你今天還剩多少額度。</p>
            <ul class="rows">
              <li v-for="h in HEADERS" :key="h.name" class="row-2">
                <code class="path">{{ h.name }}</code>
                <span class="muted">{{ h.text }}</span>
              </li>
            </ul>
          </section>
        </template>

        <template v-else-if="page === 'guide' && section === 'paging'">
          <h1 class="page-h1">分頁與減少流量</h1>
          <section class="sec">
            <h2>分頁</h2>
            <div class="split">
              <p class="sec-lead">
                每頁最多 {{ catalog?.page_size ?? 1000 }} 筆，<code>meta.next_cursor</code>
                有值表示還有下一頁。<code>history</code> 每次最多查詢
                {{ catalog?.max_range_days ?? 31 }} 天，更長的期間請改用每月的
                <RouterLink to="/developers/endpoints/download-index">大量下載</RouterLink>。
              </p>
              <CodeBlock :code="paging" label="範例" />
            </div>
          </section>
          <section class="sec">
            <h2>用 ETag 減少流量</h2>
            <div class="split">
              <p class="sec-lead">
                每個回應都有 <code>ETag</code>。下次請求時帶上
                <code>If-None-Match</code>，資料沒變就回應 <code>304</code>，不帶內容。304
                一樣算一次，但不算流量。
              </p>
              <CodeBlock :code="conditional" label="範例" />
            </div>
          </section>
        </template>

        <template v-else-if="page === 'guide' && section === 'errors'">
          <h1 class="page-h1">錯誤</h1>
          <section class="sec">
            <p class="sec-lead">
              錯誤格式為 <code>{ "error": { "code", "message" } }</code>，message 是中文說明。 取得
              token 的端點依 OAuth 2.0 規範回應 <code>error</code> 與
              <code>error_description</code>。
            </p>
            <ul class="rows">
              <li v-for="e in ERRORS" :key="e.name">
                <span class="status">{{ e.code }}</span>
                <code class="path">{{ e.name }}</code>
                <span class="muted">{{ e.text }}</span>
              </li>
            </ul>
          </section>
        </template>

        <template v-else-if="page === 'guide' && section === 'license'">
          <h1 class="page-h1">授權與引用</h1>
          <section class="sec">
            <p class="sec-lead">
              資料授權依各機關規定，大多為政府資料開放授權條款第 1 版，詳見
              <code>meta.source</code>。使用時請標示來源機關，例如「資料來源：環境部，經
              dashboard.tw 整理」。使用前請閱讀 <RouterLink to="/terms">使用條款</RouterLink>。
            </p>
            <p class="sec-lead">
              網站本身使用的資料介面，例如 <code>/api/v1/components</code> 與
              <code>/api/v1/dashboards</code>，只供 dashboard.tw
              的網頁使用。它們不開放其他網站呼叫，只有最近一年的資料，格式也可能隨時調整。要在自己的服務使用資料，請用本文件介紹的開放資料
              API。
            </p>
          </section>
        </template>

        <EndpointDoc
          v-else-if="page === 'endpoint'"
          :endpoint="endpoint!"
          :api-base="apiBase"
          :origin="origin"
        />

        <section v-else-if="page === 'catalog'" class="catalog">
          <div class="cat-head">
            <h1 class="page-h1">資料集目錄</h1>
            <label class="search">
              <span class="mi" aria-hidden="true">search</span>
              <input
                v-model="query"
                type="search"
                placeholder="搜尋名稱、代碼、機關或欄位"
                aria-label="搜尋資料集"
              />
            </label>
          </div>
          <div class="chips" role="group" aria-label="分類">
            <button type="button" :class="{ on: !topic }" @click="topic = ''">
              全部 <span>{{ datasets.length }}</span>
            </button>
            <button
              v-for="t in topics"
              :key="t.id"
              type="button"
              :class="{ on: topic === t.id }"
              @click="topic = t.id"
            >
              {{ t.name }} <span>{{ countByTopic[t.id] ?? 0 }}</span>
            </button>
          </div>

          <p v-if="loadError && !catalog" class="muted">目錄載入失敗，稍後會自動重試。</p>
          <div v-else-if="!catalog" class="loading" aria-label="載入中">
            <div v-for="i in 6" :key="i" class="skeleton-row"></div>
          </div>
          <p v-else-if="!groups.length" class="muted">找不到符合「{{ query }}」的資料集。</p>

          <div v-for="g in groups" :key="g.id" class="group">
            <h2 class="group-title">
              {{ g.name }}<span>{{ g.items.length }}</span>
            </h2>
            <ul class="datasets">
              <li v-for="d in g.items" :key="d.id">
                <RouterLink :to="`/developers/datasets/${d.id}`" class="ds-head">
                  <span class="ds-main">
                    <span class="ds-name">{{ d.name }}</span>
                    <code class="ds-id">{{ d.id }}</code>
                    <span class="ds-sum">{{ summary(d.description) }}</span>
                  </span>
                  <span class="ds-meta">
                    <span class="ds-agency">{{ d.agency }}</span>
                    <span class="ds-tags">
                      <span v-if="d.update_freq_seconds" class="tag">{{
                        formatFreq(d.update_freq_seconds)
                      }}</span>
                      <span v-if="d.has_history" class="tag">歷史</span>
                      <span v-if="d.has_downloads" class="tag">大量下載</span>
                      <span v-if="d.latest_paged && !d.has_history" class="tag">名冊</span>
                    </span>
                  </span>
                  <span class="mi chevron" aria-hidden="true">chevron_right</span>
                </RouterLink>
              </li>
            </ul>
          </div>
        </section>

        <article v-else-if="page === 'dataset'" class="ds-page">
          <h1 class="page-h1">
            {{ dataset!.name }} <code class="ds-id big">{{ dataset!.id }}</code>
          </h1>
          <UrlBar method="GET" :base="apiBase" :path="`/datasets/${dataset!.id}/latest`" />
          <DatasetDetail
            :dataset="dataset!"
            :api-base="apiBase"
            :page-size="catalog?.page_size ?? 1000"
            :max-range-days="catalog?.max_range_days ?? 31"
          />
        </article>

        <div v-else-if="page === 'loading'" class="loading" aria-label="載入中">
          <div v-for="i in 4" :key="i" class="skeleton-row"></div>
        </div>

        <section v-else class="sec">
          <h1 class="page-h1">找不到這一頁</h1>
          <p class="sec-lead">
            請從左側目錄選擇，或回到 <RouterLink to="/developers">快速開始</RouterLink>。
          </p>
        </section>

        <footer class="foot">
          <span>dashboard.tw 是雲際科技工作室的公益專案，與政府機關無關。</span>
          <span>
            <RouterLink to="/terms">使用條款</RouterLink>
            <RouterLink to="/privacy">隱私權政策</RouterLink>
            <a href="/api/v1/openapi.json" target="_blank" rel="noopener">OpenAPI 規格</a>
          </span>
        </footer>
      </main>
    </div>
  </div>
</template>

<style scoped>
.dev {
  --dev-bg: var(--bg);
  --dev-line: color-mix(in srgb, var(--border) 70%, transparent);
  --dev-subtle: color-mix(in srgb, var(--surface-muted) 55%, transparent);
  --mono: ui-monospace, 'SF Mono', 'Cascadia Code', Consolas, monospace;
  min-height: 100vh;
  background: var(--dev-bg);
  color: var(--text-primary);
  scroll-behavior: smooth;
}
code {
  font-family: var(--mono);
  font-size: 0.85em;
  padding: 1px 5px;
  border-radius: 5px;
  background: var(--dev-subtle);
}

.top {
  position: sticky;
  top: 0;
  z-index: 20;
  border-bottom: 1px solid var(--dev-line);
  background: color-mix(in srgb, var(--dev-bg) 82%, transparent);
  backdrop-filter: saturate(160%) blur(12px);
}
.top-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  max-width: 1440px;
  height: 58px;
  margin: 0 auto;
  padding: 0 24px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--text-primary);
  text-decoration: none;
  white-space: nowrap;
}
.logo {
  width: 13px;
  height: 24px;
  color: var(--text-primary);
}
.brand-name {
  font-weight: 700;
  letter-spacing: 0.01em;
}
.brand-sep {
  color: var(--text-muted);
}
.brand-page {
  overflow: hidden;
  color: var(--text-secondary);
  text-overflow: ellipsis;
}
.top-links {
  display: flex;
  align-items: center;
  gap: 4px;
}
.link {
  padding: 6px 10px;
  border-radius: 8px;
  color: var(--text-secondary);
  font-size: 0.9rem;
  text-decoration: none;
}
.link:hover {
  color: var(--text-primary);
  background: var(--dev-subtle);
}
.icon-btn {
  display: grid;
  place-items: center;
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
}
.icon-btn:hover {
  background: var(--dev-subtle);
  color: var(--text-primary);
}
.cta,
.ghost {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 7px 14px;
  border-radius: 8px;
  font-size: 0.9rem;
  font-weight: 600;
  text-decoration: none;
  white-space: nowrap;
  transition:
    filter 0.15s,
    background 0.15s;
}
.cta {
  background: var(--accent);
  color: var(--accent-text);
}
.cta:hover {
  filter: brightness(1.08);
}
.ghost {
  border: 1px solid var(--dev-line);
  color: var(--text-primary);
}
.ghost:hover {
  background: var(--dev-subtle);
}
.cta.lg,
.ghost.lg {
  padding: 11px 20px;
  font-size: 0.95rem;
}

.hero {
  margin: 0 0 8px;
  padding: 28px 0 32px;
  border-bottom: 1px solid var(--dev-line);
}
.eyebrow {
  color: var(--accent);
  font-size: 0.85rem;
  font-weight: 600;
  letter-spacing: 0.08em;
}
h1 {
  max-width: 820px;
  margin-top: 12px;
  font-size: clamp(1.9rem, 4vw, 2.9rem);
  font-weight: 700;
  line-height: 1.25;
  letter-spacing: -0.01em;
}
.lead {
  max-width: 680px;
  margin-top: 18px;
  color: var(--text-secondary);
  font-size: 1.05rem;
  line-height: 1.8;
}
.hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 28px;
}
.stats {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 44px;
  margin: 44px 0 0;
}
.stats dt {
  color: var(--text-muted);
  font-size: 0.8rem;
}
.stats dd {
  margin: 4px 0 0;
  font-size: 1.5rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.layout {
  display: grid;
  grid-template-columns: 264px minmax(0, 1fr);
  max-width: 1440px;
  margin: 0 auto;
}
.side {
  border-right: 1px solid var(--dev-line);
}
.side nav {
  position: sticky;
  top: 58px;
  display: flex;
  flex-direction: column;
  gap: 1px;
  max-height: calc(100vh - 58px);
  padding: 20px 14px 32px 24px;
  overflow-y: auto;
  scrollbar-width: none;
}
.side nav::-webkit-scrollbar {
  display: none;
}
.nav-search {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  padding: 0 10px;
  border: 1px solid var(--dev-line);
  border-radius: 8px;
  background: var(--dev-subtle);
}
.nav-search .mi {
  color: var(--text-muted);
  font-size: 1.125rem;
}
.nav-search input {
  flex: 1;
  min-width: 0;
  height: 34px;
  border: 0;
  outline: none;
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  font-size: 0.88rem;
}
.nav-link {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  padding: 6px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.88rem;
  text-align: left;
  text-decoration: none;
  cursor: pointer;
}
.nav-link:hover {
  background: var(--dev-subtle);
  color: var(--text-primary);
}
.nav-link.on {
  background: color-mix(in srgb, var(--accent) 12%, transparent);
  color: var(--text-primary);
  font-weight: 600;
}
.verb {
  flex: none;
  font-family: var(--mono);
  font-size: 0.7rem;
  font-weight: 700;
}
.verb.get {
  color: #1f8f5f;
}
.verb.post {
  color: #d9661f;
}
.count {
  color: var(--text-muted);
  font-size: 0.78rem;
  font-variant-numeric: tabular-nums;
}
.chev {
  color: var(--text-muted);
  font-size: 1.125rem;
  transition: transform 0.2s ease;
}
.chev.up {
  transform: rotate(180deg);
}
.nav-sub {
  display: flex;
  flex-direction: column;
  gap: 1px;
  margin: 2px 0 6px 10px;
  padding-left: 8px;
  border-left: 1px solid var(--dev-line);
}
.nav-sub .nav-link {
  font-size: 0.84rem;
}
.nav-empty {
  padding: 6px 10px;
}
.scrim {
  display: none;
}
.side-title {
  margin: 18px 0 6px;
  color: var(--text-muted);
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.08em;
}
.side-title:first-child {
  margin-top: 0;
}
.main {
  min-width: 0;
  padding: 0 24px 48px 48px;
}
.sec {
  padding: 28px 0 36px;
  border-bottom: 1px solid var(--dev-line);
  scroll-margin-top: 70px;
}
.sec h2 {
  font-size: 1.4rem;
  font-weight: 700;
  letter-spacing: -0.005em;
}
.sec-lead {
  margin-top: 10px;
  color: var(--text-secondary);
  line-height: 1.8;
}

.split {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  margin-top: 10px;
}
.split > :first-child {
  margin-top: 0;
}
@media (min-width: 80em) {
  .split {
    grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
    gap: 32px;
    align-items: start;
  }
}

.steps {
  display: flex;
  flex-direction: column;
  gap: 28px;
  margin: 22px 0 0;
  padding: 0;
  list-style: none;
}
.steps li {
  display: grid;
  grid-template-columns: 30px minmax(0, 1fr);
  gap: 16px;
}
.num {
  display: grid;
  place-items: center;
  width: 28px;
  height: 28px;
  border: 1px solid var(--dev-line);
  border-radius: 50%;
  color: var(--text-secondary);
  font-size: 0.85rem;
  font-weight: 600;
}
.steps h3 {
  font-size: 1rem;
  font-weight: 600;
}
.steps p {
  margin: 6px 0 12px;
  color: var(--text-secondary);
  line-height: 1.75;
}
.steps p.more-samples {
  margin-top: 10px;
  font-size: 0.9rem;
}
.more-samples a {
  color: var(--accent);
}

.cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin-top: 18px;
}
.card {
  padding: 18px;
  border: 1px solid var(--dev-line);
  border-radius: 12px;
  background: var(--surface);
}
.card .mi {
  color: var(--accent);
  font-size: 1.3rem;
}
.card h2 {
  margin-top: 10px;
  font-size: 0.95rem;
  font-weight: 600;
}
.card p {
  margin-top: 6px;
  color: var(--text-secondary);
  font-size: 0.88rem;
  line-height: 1.7;
}

.rows {
  margin: 18px 0 12px;
  padding: 0;
  border: 1px solid var(--dev-line);
  border-radius: 12px;
  background: var(--surface);
  list-style: none;
}
.rows li {
  display: grid;
  grid-template-columns: 46px minmax(180px, auto) 1fr;
  align-items: baseline;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--dev-line);
  font-size: 0.9rem;
}
.rows li:last-child {
  border-bottom: 0;
}
.method,
.status {
  justify-self: start;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.04em;
}
.status {
  background: var(--warning-bg);
  color: var(--warning-text);
}
.path {
  padding: 0;
  background: none;
  font-size: 0.85rem;
  white-space: nowrap;
}
.sec > :deep(.code) {
  margin-top: 16px;
}

.cat-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.search {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 0 1 320px;
  padding: 0 12px;
  border: 1px solid var(--dev-line);
  border-radius: 10px;
  background: var(--surface);
  color: var(--text-muted);
}
.search:focus-within {
  border-color: var(--accent);
  outline: 2px solid var(--focus);
  outline-offset: 1px;
}
.nav-search:focus-within {
  outline: 2px solid var(--focus);
  outline-offset: 1px;
}
.search input {
  flex: 1;
  min-width: 0;
  height: 38px;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  font-size: 0.9rem;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 16px;
}
.chips button {
  padding: 5px 12px;
  border: 1px solid var(--dev-line);
  border-radius: 999px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.85rem;
  cursor: pointer;
}
.chips button span {
  margin-left: 2px;
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}
.chips button.on {
  border-color: var(--text-primary);
  background: var(--text-primary);
  color: var(--dev-bg);
}
.chips button.on span {
  color: inherit;
  opacity: 0.7;
}
.group {
  margin-top: 26px;
}
.group-title {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-bottom: 10px;
  font-size: 0.8rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  color: var(--text-muted);
}
.datasets {
  margin: 0;
  padding: 0;
  border: 1px solid var(--dev-line);
  border-radius: 12px;
  background: var(--surface);
  list-style: none;
}
.datasets > li {
  border-bottom: 1px solid var(--dev-line);
  scroll-margin-top: 70px;
}
.datasets > li:last-child {
  border-bottom: 0;
}
.ds-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto 20px;
  align-items: center;
  gap: 16px;
  width: 100%;
  padding: 14px 16px;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  text-decoration: none;
  cursor: pointer;
}
.ds-head:hover {
  background: var(--dev-subtle);
}
.datasets > li:first-child .ds-head {
  border-radius: 12px 12px 0 0;
}
.ds-main {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 10px;
  min-width: 0;
}
.ds-name {
  font-weight: 600;
}
.ds-id {
  color: var(--text-muted);
  font-size: 0.78rem;
}
.ds-sum {
  flex-basis: 100%;
  overflow: hidden;
  color: var(--text-muted);
  font-size: 0.84rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ds-meta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}
.ds-agency {
  max-width: 220px;
  overflow: hidden;
  color: var(--text-secondary);
  font-size: 0.8rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ds-tags {
  display: flex;
  gap: 4px;
}
.tag {
  padding: 1px 7px;
  border-radius: 999px;
  background: var(--dev-subtle);
  color: var(--text-muted);
  font-size: 0.72rem;
  white-space: nowrap;
}
.chevron {
  color: var(--text-muted);
  transition: transform 0.2s;
}
.loading {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 22px;
}
.skeleton-row {
  height: 62px;
  border-radius: 10px;
  background: var(--dev-subtle);
}
.foot {
  display: flex;
  flex-wrap: wrap;
  justify-content: space-between;
  gap: 8px 16px;
  padding-top: 28px;
  color: var(--text-muted);
  font-size: 0.8rem;
}
.foot span:last-child {
  display: flex;
  gap: 14px;
}
.foot a {
  color: var(--text-muted);
}
.muted {
  color: var(--text-muted);
}
.small {
  font-size: 0.82rem;
  line-height: 1.75;
}

.menu {
  display: none;
}
@media (max-width: 56.25em) {
  .layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .menu {
    display: grid;
  }
  .side {
    position: fixed;
    top: 58px;
    bottom: 0;
    left: 0;
    z-index: 30;
    width: min(300px, 86vw);
    border-right: 1px solid var(--dev-line);
    background: var(--dev-bg);
    transform: translateX(-100%);
    transition: transform 0.2s ease;
  }
  .side.open {
    transform: none;
  }
  .side nav {
    position: static;
    max-height: 100%;
    padding-left: 16px;
  }
  .scrim {
    position: fixed;
    inset: 58px 0 0;
    z-index: 29;
    display: block;
    border: 0;
    background: rgb(0 0 0 / 35%);
  }
  .main {
    padding-left: 24px;
  }
}
@media (max-width: 40em) {
  .top-inner,
  .main {
    padding-left: 16px;
    padding-right: 16px;
  }
  .hide-sm {
    display: none;
  }
  .brand-name {
    display: none;
  }
  .brand-sep {
    display: none;
  }
  .rows li {
    grid-template-columns: 46px minmax(0, 1fr);
  }
  .rows li .muted {
    grid-column: 1 / -1;
  }
  .path {
    white-space: normal;
    word-break: break-all;
  }
  .ds-head {
    grid-template-columns: minmax(0, 1fr) 20px;
  }
  .ds-meta {
    display: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  .dev {
    scroll-behavior: auto;
  }
  .side,
  .chev {
    transition: none;
  }
  .chevron {
    transition: none;
  }
}
.crumbs {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  padding-top: 28px;
  color: var(--text-muted);
  font-size: 0.85rem;
}
.crumbs a {
  color: var(--text-secondary);
  text-decoration: none;
}
.crumbs a:hover {
  color: var(--accent);
}
.crumbs .mi {
  font-size: 1rem;
}
.crumbs + * {
  margin-top: 10px;
}
.main > :first-child:not(.crumbs) {
  margin-top: 28px;
}
.page-h1 {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 10px;
  margin-bottom: 6px;
  font-size: clamp(1.6rem, 3vw, 2.1rem);
  font-weight: 700;
  letter-spacing: -0.01em;
}
.ds-id.big {
  font-size: 0.95rem;
  font-weight: 500;
}
.ds-page {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.catalog {
  padding-bottom: 36px;
}
.rows li.row-2 {
  grid-template-columns: minmax(200px, auto) 1fr;
}
.brand-home {
  display: flex;
  align-items: center;
  gap: 8px;
  color: inherit;
  text-decoration: none;
}
.brand-page {
  text-decoration: none;
}
.foot {
  margin-top: 24px;
  padding-bottom: 32px;
  border-top: 1px solid var(--dev-line);
}
</style>
