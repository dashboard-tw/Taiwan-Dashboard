<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ApiError } from '../api/client'
import {
  APP_CATEGORIES,
  APP_STATUS,
  categoryLabel,
  MAX_APP_IMAGE_BYTES,
  appImageUrl,
  createApp,
  deleteApp,
  getMyApps,
  updateApp,
  uploadAppImage,
  type AppFields,
  type MyApp,
} from '../api/apps'
import { TOPICS } from '../lib/apiSurvey'

const props = defineProps<{ hasClient: boolean | null }>()
const emit = defineEmits<{ changed: [] }>()

const apps = ref<MyApp[]>([])
const loadError = ref('')
onMounted(async () => {
  try {
    const r = await getMyApps()
    apps.value = r.apps
  } catch {
    loadError.value = '載入失敗，請重新整理。'
  }
})

const blank = (): AppFields => ({
  category: 'web',
  name: '',
  description: '',
  web_url: '',
  ios_url: '',
  android_url: '',
  topics: [],
  show_author: true,
})
const editing = ref<number | null>(null)
const form = ref<AppFields>(blank())
const file = ref<File | null>(null)
const preview = ref<string | null>(null)
const shotFile = ref<File | null>(null)
const shotPreview = ref<string | null>(null)
const agreed = ref(false)
const picked = ref(false)
const busy = ref(false)
const error = ref('')

function start(a?: MyApp) {
  editing.value = a ? a.id : 0
  form.value = a
    ? {
        category: a.category,
        name: a.name,
        description: a.description,
        web_url: a.web_url,
        ios_url: a.ios_url,
        android_url: a.android_url,
        topics: [...a.topics],
        show_author: a.show_author,
      }
    : blank()
  setFile(null)
  setShot(null)
  preview.value = a ? appImageUrl(a.image) : null
  shotPreview.value = a ? appImageUrl(a.screenshot) : null
  picked.value = !!a
  agreed.value = false
  error.value = ''
}
function cancel() {
  editing.value = null
  setFile(null)
  setShot(null)
}

function setFile(f: File | null) {
  if (preview.value?.startsWith('blob:')) URL.revokeObjectURL(preview.value)
  file.value = f
  preview.value = f ? URL.createObjectURL(f) : null
}
function setShot(f: File | null) {
  if (shotPreview.value?.startsWith('blob:')) URL.revokeObjectURL(shotPreview.value)
  shotFile.value = f
  shotPreview.value = f ? URL.createObjectURL(f) : null
}
function pick(e: Event, which: 'logo' | 'shot' = 'logo') {
  const f = (e.target as HTMLInputElement).files?.[0] ?? null
  error.value = ''
  if (f && f.size > MAX_APP_IMAGE_BYTES) {
    error.value = '圖片最大 2 MB'
    ;(e.target as HTMLInputElement).value = ''
    return
  }
  if (f && !['image/png', 'image/jpeg', 'image/webp'].includes(f.type)) {
    error.value = '只能上傳 PNG、JPEG 或 WebP 圖片'
    ;(e.target as HTMLInputElement).value = ''
    return
  }
  if (which === 'shot') setShot(f)
  else setFile(f)
}

const FIELDS = {
  app: {
    name: 'App 名稱',
    desc: 'App 介紹',
    descHint: '這個 App 可以做什麼',
    image: 'App 圖示',
  },
  web: {
    name: '網站名稱',
    desc: '網站介紹',
    descHint: '這個網站可以做什麼',
    image: '網站 Logo',
  },
  innovation: {
    name: '作品名稱',
    desc: '作品介紹',
    descHint: '這個作品的內容',
    image: '作品 Logo',
  },
} as const
const fields = computed(() => FIELDS[form.value.category])

function choose(c: AppFields['category']) {
  form.value.category = c
  picked.value = true
}

function payload(): AppFields {
  const f = { ...form.value }
  if (f.category !== 'app') {
    f.ios_url = ''
    f.android_url = ''
  }
  return f
}

function toggle(t: string, on: boolean) {
  const list = form.value.topics.filter((x) => x !== t)
  form.value.topics = on ? [...list, t] : list
}

const linkOk = computed(() => {
  const f = form.value
  return f.category === 'app' ? !!(f.ios_url.trim() || f.android_url.trim()) : !!f.web_url.trim()
})
const ready = computed(
  () =>
    picked.value &&
    linkOk.value &&
    !!form.value.name.trim() &&
    !!form.value.description.trim() &&
    form.value.topics.length > 0 &&
    agreed.value,
)

async function submit() {
  if (!ready.value || busy.value) return
  busy.value = true
  error.value = ''
  try {
    let list: MyApp[]
    let id = editing.value ?? 0
    if (id) {
      list = await updateApp(id, payload())
    } else {
      const before = new Set(apps.value.map((a) => a.id))
      list = await createApp(payload())
      id = list.find((a) => !before.has(a.id))?.id ?? 0
    }
    apps.value = list
    const uploads: [File | null, 'image' | 'screenshot'][] = [
      [file.value, 'image'],
      [form.value.category === 'web' ? shotFile.value : null, 'screenshot'],
    ]
    for (const [f, kind] of uploads) {
      if (!f || !id) continue
      try {
        apps.value = await uploadAppImage(id, f, kind)
      } catch (e) {
        const what = kind === 'image' ? 'Logo' : '截圖'
        error.value = `${what}上傳失敗：${e instanceof ApiError ? e.message : '請再試一次'}`
        editing.value = id
        return
      }
    }
    cancel()
    emit('changed')
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '送出失敗，請再試一次'
  } finally {
    busy.value = false
  }
}

async function remove(a: MyApp) {
  const msg = a.published
    ? `確定要刪除「${a.name}」嗎？刪除後會從應用活化下架。`
    : `確定要刪除「${a.name}」嗎？`
  if (!confirm(msg)) return
  try {
    await deleteApp(a.id)
    apps.value = apps.value.filter((x) => x.id !== a.id)
    if (editing.value === a.id) cancel()
    emit('changed')
  } catch (e) {
    loadError.value = e instanceof ApiError ? e.message : '刪除失敗，請再試一次'
  }
}

function statusText(a: MyApp) {
  if (a.status === 'pending' && a.published) return '修改審核中'
  if (a.status === 'rejected' && a.published) return '修改未通過'
  return APP_STATUS[a.status]
}
</script>

<template>
  <section class="card block">
    <h2>
      我的應用
      <RouterLink to="/showcase/app" class="btn-plain"
        ><span class="mi" aria-hidden="true">apps</span>應用活化</RouterLink
      >
    </h2>
    <p v-if="!apps.length && editing === null" class="muted small">還沒有申請紀錄</p>
    <p v-if="loadError" class="warn small" role="alert">{{ loadError }}</p>

    <ul v-if="apps.length" class="apps">
      <li v-for="a in apps" :key="a.id">
        <img v-if="a.image" :src="appImageUrl(a.image)!" alt="" class="thumb" />
        <span v-else class="thumb empty mi" aria-hidden="true">apps</span>
        <div class="info">
          <div class="head">
            <strong>{{ a.name }}</strong>
            <span class="badge">{{ categoryLabel(a.category) }}</span>
            <span class="badge" :class="a.status">{{ statusText(a) }}</span>
          </div>
          <p v-if="a.review_note" class="note small">原因：{{ a.review_note }}</p>
          <div class="actions">
            <button type="button" class="btn-plain" @click="start(a)">
              <span class="mi" aria-hidden="true">edit</span>修改
            </button>
            <button type="button" class="btn-plain" @click="remove(a)">
              <span class="mi" aria-hidden="true">delete</span>刪除
            </button>
          </div>
        </div>
      </li>
    </ul>

    <form v-if="editing !== null" class="form" @submit.prevent="submit">
      <fieldset class="field">
        <legend>選擇類別</legend>
        <div class="kinds">
          <button
            v-for="c in APP_CATEGORIES"
            :key="c.value"
            type="button"
            class="kind"
            :class="{ on: picked && form.category === c.value }"
            :aria-pressed="picked && form.category === c.value"
            @click="choose(c.value)"
          >
            <span class="mi" aria-hidden="true">{{ c.icon }}</span>
            <strong>{{ c.label }}</strong>
            <span class="muted small">{{ c.hint }}</span>
          </button>
        </div>
      </fieldset>

      <template v-if="picked">
        <label class="field">
          <span>{{ fields.name }}<em>（必填）</em></span>
          <input v-model="form.name" class="input" maxlength="40" required />
        </label>

        <template v-if="form.category === 'app'">
          <fieldset class="field">
            <legend>下載連結<em>（至少填一個）</em></legend>
            <input
              v-model="form.ios_url"
              class="input"
              type="url"
              placeholder="App Store https://apps.apple.com/…"
            />
            <input
              v-model="form.android_url"
              class="input"
              type="url"
              placeholder="Google Play https://play.google.com/…"
            />
          </fieldset>
          <label class="field">
            <span>官方網站</span>
            <input v-model="form.web_url" class="input" type="url" placeholder="https://" />
          </label>
        </template>
        <label v-else class="field">
          <span>{{ form.category === 'web' ? '網站網址' : '作品連結' }}<em>（必填）</em></span>
          <input v-model="form.web_url" class="input" type="url" placeholder="https://" required />
          <span v-if="form.category === 'innovation'" class="muted small"
            >報告、文章、影片或 GitHub 都可以</span
          >
        </label>

        <label class="field">
          <span>{{ fields.desc }}<em>（必填）</em></span>
          <textarea
            v-model="form.description"
            class="input"
            rows="3"
            maxlength="200"
            :placeholder="fields.descHint"
            required
          ></textarea>
        </label>

        <div class="field">
          <span>{{ fields.image }}</span>
          <div class="image-row">
            <img v-if="preview" :src="preview" alt="" class="thumb" />
            <label class="btn-plain upload">
              <span class="mi" aria-hidden="true">image</span
              >{{ preview ? '更換圖片' : '選擇圖片' }}
              <input type="file" accept="image/png,image/jpeg,image/webp" @change="pick" />
            </label>
          </div>
          <span class="muted small">正方形，PNG、JPEG 或 WebP，2 MB 以內</span>
        </div>

        <div v-if="form.category === 'web'" class="field">
          <span>網站截圖</span>
          <div class="image-row">
            <img v-if="shotPreview" :src="shotPreview" alt="" class="thumb wide" />
            <label class="btn-plain upload">
              <span class="mi" aria-hidden="true">image</span
              >{{ shotPreview ? '更換截圖' : '選擇截圖' }}
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp"
                @change="pick($event, 'shot')"
              />
            </label>
          </div>
          <span class="muted small">一張橫向截圖，PNG、JPEG 或 WebP，2 MB 以內</span>
        </div>

        <fieldset class="field">
          <legend>用到的資料<em>（可複選）</em></legend>
          <div class="chips">
            <label
              v-for="(label, k) in TOPICS"
              :key="k"
              class="chip"
              :class="{ on: form.topics.includes(k) }"
            >
              <input
                type="checkbox"
                :checked="form.topics.includes(k)"
                @change="toggle(k, ($event.target as HTMLInputElement).checked)"
              />{{ label }}
            </label>
          </div>
        </fieldset>

        <label class="check">
          <input v-model="form.show_author" type="checkbox" />顯示我的名稱
        </label>
        <div class="rules small">
          <p>送出前請確認</p>
          <ul>
            <li>有使用本站 API，而且可以正常使用</li>
            <li>沒有廣告或違法內容，不冒用政府名義</li>
            <li>無法使用或違反規定時會下架</li>
          </ul>
          <label class="check"> <input v-model="agreed" type="checkbox" />我同意以上規定 </label>
        </div>
      </template>
      <p v-if="error" class="warn small" role="alert">{{ error }}</p>
      <div class="actions">
        <button v-if="picked" type="submit" class="btn-highlight" :disabled="!ready || busy">
          {{ busy ? '送出中' : '送出審核' }}
        </button>
        <button type="button" class="btn-plain" @click="cancel">取消</button>
      </div>
    </form>
    <p v-else-if="props.hasClient === false" class="muted small">
      建立 API 憑證後才能申請。<RouterLink to="/me/api-key">前往 API 憑證</RouterLink>
    </p>
    <button v-else-if="props.hasClient" type="button" class="btn-plain add" @click="start()">
      <span class="mi" aria-hidden="true">add</span>申請應用活化
    </button>
  </section>
</template>

<style scoped>
h2 {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: var(--font-ms);
}
p {
  margin: 0;
}
.apps {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
.apps li {
  display: flex;
  gap: 0.75rem;
  padding: 0.75rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
.thumb {
  flex-shrink: 0;
  width: 72px;
  height: 45px;
  border-radius: 4px;
  object-fit: contain;
  background: var(--surface-muted);
}
.thumb.wide {
  width: 96px;
  height: 60px;
}
.thumb.empty {
  display: grid;
  place-items: center;
  color: var(--text-secondary);
}
.info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}
.head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}
.head strong {
  overflow-wrap: anywhere;
}
.badge {
  padding: 1px 8px;
  border-radius: 999px;
  border: 1px solid var(--border);
  font-size: var(--font-s);
  color: var(--text-secondary);
}
.badge.approved {
  border-color: var(--ok-text, #3fb950);
  color: var(--ok-text, #3fb950);
}
.badge.rejected,
.badge.removed {
  border-color: var(--danger-text);
  color: var(--danger-text);
}
.note {
  margin: 0;
  color: var(--danger-text);
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  max-width: 32rem;
  margin-top: 0.5rem;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  border: 0;
  font-size: 0.875rem;
}
.field > span:first-child,
legend {
  color: var(--text-secondary);
}
legend {
  margin-bottom: 4px;
  padding: 0;
}
em {
  color: var(--danger-text);
  font-style: normal;
}
.input {
  padding: 5px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font: inherit;
}
textarea.input {
  resize: vertical;
}
.checks {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem 1.25rem;
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 0.875rem;
}
.kinds {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));
  gap: 0.5rem;
}
.kind {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  padding: 0.6rem 0.75rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.kind:hover {
  border-color: var(--accent);
}
.kind.on {
  border-color: var(--accent);
  background: var(--accent-soft);
}
.kind .mi {
  color: var(--accent);
  font-size: 1.3rem;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.chip {
  position: relative;
  padding: 2px 10px;
  border: 1px solid var(--border);
  border-radius: 999px;
  font-size: 0.8125rem;
  cursor: pointer;
}
.chip input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}
.chip:has(input:focus-visible) {
  outline: 2px solid var(--focus);
}
.chip.on {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}
.image-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.upload {
  position: relative;
  cursor: pointer;
}
.upload input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
}
.rules {
  padding-top: 0.5rem;
  border-top: 1px solid var(--border);
}
.rules p {
  margin: 0 0 4px;
}
.rules ul {
  margin: 0 0 0.5rem;
  padding-left: 1.25rem;
  color: var(--text-secondary);
}
.btn-plain {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-primary);
  font-size: 0.85rem;
  text-decoration: none;
  cursor: pointer;
}
.btn-plain:hover {
  background: var(--surface-muted);
}
.btn-plain .mi {
  font-size: 1rem;
}
h2 .btn-plain {
  margin-left: auto;
  font-weight: 400;
}
.add {
  align-self: flex-start;
}
.btn-highlight:disabled {
  opacity: 0.5;
  cursor: default;
}
.small {
  font-size: var(--font-s);
}
</style>
