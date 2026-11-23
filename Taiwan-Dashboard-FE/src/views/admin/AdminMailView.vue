<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import {
  getMailTemplates,
  previewMailTemplate,
  saveMailTemplate,
  testMailTemplate,
  type MailTemplate,
} from '../../api/admin'
import { ApiError } from '../../api/client'
import { usePageTitle } from '../../composables/usePageTitle'
import MailQueuePanel from './MailQueuePanel.vue'
import MailSuppressionPanel from './MailSuppressionPanel.vue'

usePageTitle('信件模板')

const templates = ref<MailTemplate[]>([])
const sending = ref(true)
const loadError = ref('')
const currentKey = ref('')
const subject = ref('')
const body = ref('')
const enabled = ref(true)
const preview = ref<{ subject: string; body: string } | null>(null)
const previewError = ref('')
const status = ref('')
const error = ref('')
const busy = ref(false)
const bodyBox = ref<HTMLTextAreaElement | null>(null)

const current = computed(() => templates.value.find((t) => t.key === currentKey.value) ?? null)
const isFooter = computed(() => currentKey.value === 'footer')
const dirty = computed(
  () =>
    !!current.value &&
    (current.value.subject !== subject.value ||
      current.value.body !== body.value ||
      current.value.enabled !== enabled.value),
)

function message(e: unknown) {
  return e instanceof ApiError ? e.message : '操作失敗，請再試一次'
}

async function load() {
  try {
    const r = await getMailTemplates()
    templates.value = r.templates
    sending.value = r.sending
    if (!currentKey.value && r.templates.length) pick(r.templates[0]!.key)
  } catch (e) {
    loadError.value = message(e)
  }
}
onMounted(load)

function pick(key: string) {
  if (dirty.value && !window.confirm('這封信還沒儲存，要換到別封嗎？')) return
  const t = templates.value.find((x) => x.key === key)
  if (!t) return
  currentKey.value = key
  subject.value = t.subject
  body.value = t.body
  enabled.value = t.enabled
  status.value = ''
  error.value = ''
}

let timer: ReturnType<typeof setTimeout> | undefined
let seq = 0
watch([currentKey, subject, body], () => {
  clearTimeout(timer)
  timer = setTimeout(async () => {
    if (!currentKey.value) return
    const n = ++seq
    try {
      const p = await previewMailTemplate(currentKey.value, {
        subject: subject.value,
        body: body.value,
      })
      if (n !== seq) return
      preview.value = p
      previewError.value = ''
    } catch (e) {
      if (n !== seq) return
      previewError.value = message(e)
    }
  }, 400)
})

const token = (key: string) => '{{.' + key + '}}'

async function insertVar(key: string) {
  const el = bodyBox.value
  const t = token(key)
  if (!el) {
    body.value += t
    return
  }
  const start = el.selectionStart
  const end = el.selectionEnd
  body.value = body.value.slice(0, start) + t + body.value.slice(end)
  await nextTick()
  el.focus()
  el.setSelectionRange(start + t.length, start + t.length)
}

async function save() {
  if (!current.value) return
  busy.value = true
  error.value = ''
  status.value = ''
  try {
    await saveMailTemplate(current.value.key, {
      subject: subject.value,
      body: body.value,
      enabled: enabled.value,
    })
    const key = current.value.key
    await load()
    const t = templates.value.find((x) => x.key === key)
    if (t) {
      subject.value = t.subject
      body.value = t.body
      enabled.value = t.enabled
    }
    status.value = '已儲存'
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}

async function sendTest() {
  if (!current.value) return
  if (dirty.value) {
    error.value = '請先儲存'
    return
  }
  busy.value = true
  error.value = ''
  status.value = ''
  try {
    const to = await testMailTemplate(current.value.key)
    status.value = `測試信已寄到 ${to}`
  } catch (e) {
    error.value = message(e)
  } finally {
    busy.value = false
  }
}

function date(iso: string) {
  return new Date(iso).toLocaleString('zh-TW', { timeZone: 'Asia/Taipei', hour12: false })
}
</script>

<template>
  <div>
    <h1 class="page-title"><span class="mi" aria-hidden="true">mail</span>信件模板</h1>
    <p v-if="!sending" class="notice warn">還沒設定寄信，信不會寄出。</p>
    <p v-if="loadError" class="notice bad" role="alert">{{ loadError }}</p>

    <div v-if="templates.length" class="mail-grid">
      <nav class="card block list" aria-label="信件種類">
        <button
          v-for="t in templates"
          :key="t.key"
          type="button"
          :class="{ on: t.key === currentKey }"
          :aria-current="t.key === currentKey ? 'true' : undefined"
          @click="pick(t.key)"
        >
          <span>{{ t.name }}</span>
          <span v-if="!t.enabled" class="badge muted">停用</span>
        </button>
      </nav>

      <section v-if="current" class="card block editor" :aria-label="`編輯：${current.name}`">
        <h2>{{ current.name }}</h2>
        <p class="sub">
          最後修改：{{ date(current.updated_at)
          }}<template v-if="current.updated_by">（{{ current.updated_by }}）</template>
        </p>

        <label v-if="!isFooter" class="check">
          <input v-model="enabled" type="checkbox" />寄出這封信
        </label>

        <label v-if="!isFooter" class="field">
          <span>主旨</span>
          <input v-model="subject" type="text" maxlength="200" />
        </label>

        <label class="field">
          <span>內容</span>
          <textarea ref="bodyBox" v-model="body" rows="14" maxlength="5000"></textarea>
        </label>

        <div class="vars">
          <span class="sub">點一下插入變數：</span>
          <button
            v-for="v in current.vars"
            :key="v.key"
            type="button"
            class="var"
            :title="`範例：${v.sample}`"
            @click="insertVar(v.key)"
          >
            {{ v.label }} <code>{{ token(v.key) }}</code>
          </button>
        </div>

        <div class="actions">
          <button type="button" class="btn-highlight" :disabled="busy || !dirty" @click="save">
            儲存
          </button>
          <button v-if="!isFooter" type="button" :disabled="busy || !sending" @click="sendTest">
            寄測試信給我
          </button>
          <span role="status" class="muted">{{ status }}</span>
        </div>
        <p v-if="error" class="notice bad" role="alert">{{ error }}</p>
      </section>

      <section class="card block preview" aria-label="預覽" aria-live="polite">
        <h2>預覽</h2>
        <p v-if="previewError" class="notice bad">{{ previewError }}</p>
        <template v-else-if="preview">
          <p v-if="!isFooter" class="subject">{{ preview.subject }}</p>
          <pre class="mail-body">{{ preview.body }}</pre>
        </template>
      </section>
    </div>

    <MailQueuePanel />
    <MailSuppressionPanel />
  </div>
</template>

<style scoped>
.mail-grid {
  display: grid;
  grid-template-columns: 12rem minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--font-m);
  align-items: start;
}
.list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.list button {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
  padding: 0.4rem 0.6rem;
  border: 0;
  border-radius: var(--radius);
  background: none;
  color: var(--text-muted);
  font-size: 0.875rem;
  text-align: left;
  cursor: pointer;
}
.list button:hover,
.list button.on {
  background: var(--accent-soft);
  color: var(--text-primary);
}
.editor {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.editor h2 {
  margin-bottom: 0;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 0.875rem;
}
.field input,
.field textarea {
  padding: 0.5rem;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font: inherit;
  line-height: 1.6;
}
.field textarea {
  resize: vertical;
}
.check {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
}
.vars {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}
.var {
  padding: 2px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: none;
  color: var(--text-muted);
  font-size: var(--font-s);
  cursor: pointer;
}
.var:hover {
  color: var(--text-primary);
}
.var code {
  font-size: 0.75rem;
}
.actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.actions button:not(.btn-highlight) {
  padding: 0.4rem 0.8rem;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: none;
  color: var(--text-primary);
  cursor: pointer;
}
.actions button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.subject {
  margin-bottom: 0.5rem;
  font-weight: 700;
}
.mail-body {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: inherit;
  font-size: 0.875rem;
  line-height: 1.7;
}
</style>
