<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  createChannel,
  deleteChannel,
  getChannels,
  testChannel,
  updateChannel,
  type ChannelInput,
  type ChannelKind,
  type ChannelList,
  type NotifyChannel,
  type OutboxRow,
} from '../../api/admin'
import { ApiError } from '../../api/client'
import { usePageTitle } from '../../composables/usePageTitle'

usePageTitle('通知頻道')

const KINDS: Record<ChannelKind, string> = {
  discord: 'Discord',
  slack: 'Slack',
  telegram: 'Telegram',
  json: 'JSON webhook',
}
const STATUS: Record<OutboxRow['status'], { label: string; cls: string }> = {
  pending: { label: '排隊中', cls: 'warn' },
  sending: { label: '送出中', cls: 'warn' },
  sent: { label: '已送出', cls: 'ok' },
  failed: { label: '失敗', cls: 'bad' },
}

const data = ref<ChannelList | null>(null)
const loadError = ref('')
const editing = ref<number | 'new' | null>(null)
const form = ref<ChannelInput>(blank())
const formError = ref('')
const busy = ref(false)
const notice = ref<{ ok: boolean; text: string } | null>(null)

function blank(): ChannelInput {
  return {
    name: '',
    kind: 'discord',
    url: '',
    token: '',
    chat: '',
    secret: '',
    events: [],
    enabled: true,
  }
}

const eventLabel = computed<Record<string, string>>(() => ({
  test: '測試',
  ...Object.fromEntries((data.value?.events ?? []).map((e) => [e.key, e.label])),
}))
const current = computed(() =>
  typeof editing.value === 'number'
    ? (data.value?.channels.find((c) => c.id === editing.value) ?? null)
    : null,
)

function message(e: unknown) {
  return e instanceof ApiError ? e.message : '操作失敗，請再試一次'
}

async function load() {
  try {
    data.value = await getChannels()
    loadError.value = ''
  } catch (e) {
    loadError.value = message(e)
  }
}
onMounted(load)

function startNew() {
  form.value = blank()
  editing.value = 'new'
  formError.value = ''
}

function startEdit(c: NotifyChannel) {
  form.value = { ...blank(), name: c.name, kind: c.kind, events: [...c.events], enabled: c.enabled }
  editing.value = c.id
  formError.value = ''
}

async function save() {
  busy.value = true
  formError.value = ''
  try {
    if (editing.value === 'new') await createChannel(form.value)
    else if (typeof editing.value === 'number') await updateChannel(editing.value, form.value)
    editing.value = null
    notice.value = { ok: true, text: '已儲存' }
    await load()
  } catch (e) {
    formError.value = message(e)
  } finally {
    busy.value = false
  }
}

async function remove(c: NotifyChannel) {
  if (!window.confirm(`刪除「${c.name}」？`)) return
  try {
    await deleteChannel(c.id)
    if (editing.value === c.id) editing.value = null
    notice.value = { ok: true, text: `已刪除「${c.name}」` }
    await load()
  } catch (e) {
    notice.value = { ok: false, text: message(e) }
  }
}

async function test(c: NotifyChannel) {
  notice.value = null
  try {
    await testChannel(c.id)
    notice.value = { ok: true, text: `測試訊息已送到「${c.name}」` }
  } catch (e) {
    notice.value = { ok: false, text: message(e) }
  }
  await load()
}

function time(iso: string | null) {
  if (!iso) return ''
  return new Date(iso).toLocaleString('zh-TW', {
    timeZone: 'Asia/Taipei',
    hour12: false,
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>

<template>
  <div>
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">campaign</span>通知頻道
      <button type="button" class="btn-highlight add" @click="startNew">新增頻道</button>
    </h1>
    <p v-if="data && !data.can_save" class="notice warn">沒有設定 EMAIL_KEY，無法儲存頻道。</p>
    <p v-if="loadError" class="notice bad" role="alert">{{ loadError }}</p>
    <p v-if="notice" :class="['notice', { bad: !notice.ok }]" role="status">{{ notice.text }}</p>

    <section v-if="editing !== null && data" class="card block form" aria-labelledby="channel-form">
      <h2 id="channel-form">{{ editing === 'new' ? '新增頻道' : `編輯：${current?.name}` }}</h2>
      <div class="row">
        <label class="field">
          <span>名稱</span>
          <input v-model="form.name" type="text" maxlength="40" placeholder="例如：公開頻道" />
        </label>
        <label class="field">
          <span>種類</span>
          <select v-model="form.kind">
            <option v-for="k in data.kinds" :key="k" :value="k">{{ KINDS[k] }}</option>
          </select>
        </label>
      </div>

      <p v-if="current" class="sub">目前：{{ current.hint }}</p>
      <template v-if="form.kind === 'telegram'">
        <div class="row">
          <label class="field">
            <span>Bot token</span>
            <input
              v-model="form.token"
              type="password"
              autocomplete="off"
              :placeholder="current ? '不改請留空' : ''"
            />
          </label>
          <label class="field">
            <span>Chat ID</span>
            <input v-model="form.chat" type="text" placeholder="-100… 或 @頻道名稱" />
          </label>
        </div>
      </template>
      <label v-else class="field">
        <span>Webhook 網址</span>
        <input
          v-model="form.url"
          type="password"
          autocomplete="off"
          :placeholder="current ? '不改請留空' : 'https://'"
        />
      </label>
      <label v-if="form.kind === 'json'" class="field">
        <span>簽章密鑰（選填）</span>
        <input v-model="form.secret" type="password" autocomplete="off" />
      </label>

      <fieldset class="events">
        <legend>要收的事件</legend>
        <label v-for="e in data.events" :key="e.key" class="check">
          <input v-model="form.events" type="checkbox" :value="e.key" />{{ e.label }}
        </label>
      </fieldset>
      <label class="check"><input v-model="form.enabled" type="checkbox" />啟用</label>

      <p v-if="formError" class="notice bad" role="alert">{{ formError }}</p>
      <div class="actions">
        <button type="button" class="btn-highlight" :disabled="busy" @click="save">儲存</button>
        <button type="button" class="plain" @click="editing = null">取消</button>
      </div>
    </section>

    <section v-if="data" class="card block" aria-labelledby="channel-list">
      <h2 id="channel-list">頻道</h2>
      <table>
        <thead>
          <tr>
            <th>名稱</th>
            <th>種類</th>
            <th>事件</th>
            <th>最近一次</th>
            <th><span class="visually-hidden">操作</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in data.channels" :key="c.id">
            <td>
              {{ c.name }}
              <span v-if="!c.enabled" class="badge muted">停用</span>
              <span class="sub">{{ c.hint }}</span>
            </td>
            <td>{{ KINDS[c.kind] }}</td>
            <td>{{ c.events.map((e) => eventLabel[e] ?? e).join('、') || '沒有' }}</td>
            <td>
              <template v-if="c.last_at">
                <span :class="c.last_ok ? 'ok' : 'bad'">{{ c.last_ok ? '成功' : '失敗' }}</span>
                {{ time(c.last_at) }}
                <span v-if="!c.last_ok" class="sub error-cell">{{ c.last_error }}</span>
              </template>
              <span v-else class="muted">還沒送過</span>
            </td>
            <td>
              <div class="ops">
                <button type="button" class="plain" @click="test(c)">送測試</button>
                <button type="button" class="plain" @click="startEdit(c)">編輯</button>
                <button type="button" class="plain bad" @click="remove(c)">刪除</button>
              </div>
            </td>
          </tr>
          <tr v-if="data.channels.length === 0">
            <td colspan="5" class="muted">還沒有頻道</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="data" class="card block" aria-labelledby="channel-recent">
      <h2 id="channel-recent">最近的通知</h2>
      <table>
        <thead>
          <tr>
            <th>頻道</th>
            <th>事件</th>
            <th>內容</th>
            <th>狀態</th>
            <th>時間</th>
            <th>原因</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in data.recent" :key="r.id">
            <td>{{ r.channel }}</td>
            <td>{{ eventLabel[r.event] ?? r.event }}</td>
            <td>{{ r.title }}</td>
            <td>
              <span class="badge" :class="STATUS[r.status].cls">{{ STATUS[r.status].label }}</span>
            </td>
            <td>{{ time(r.sent_at ?? r.created_at) }}</td>
            <td class="error-cell">{{ r.last_error }}</td>
          </tr>
          <tr v-if="data.recent.length === 0">
            <td colspan="6" class="muted">還沒有通知</td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<style scoped>
.add {
  margin-left: auto;
  font-size: 0.875rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  max-width: 44rem;
}
.row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 0.875rem;
}
.field input,
.field select {
  padding: 0.45rem 0.5rem;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font: inherit;
}
.events {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem 1rem;
  margin: 0;
  padding: 0.5rem 0.75rem;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  font-size: 0.875rem;
}
.events legend {
  padding: 0 4px;
  color: var(--text-muted);
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.875rem;
}
.actions,
.ops {
  display: flex;
  gap: 0.5rem;
}
.ops {
  justify-content: flex-end;
  white-space: nowrap;
}
.plain {
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: none;
  color: var(--text-muted);
  font-size: var(--font-s);
  cursor: pointer;
}
.plain:hover {
  color: var(--text-primary);
}
.notice:not(.bad) {
  color: var(--text-primary);
}
</style>
