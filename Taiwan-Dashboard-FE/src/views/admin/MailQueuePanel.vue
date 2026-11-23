<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getMailQueue, retryMail, type MailQueue, type QueuedMail } from '../../api/admin'
import { ApiError } from '../../api/client'

const STATUS: Record<QueuedMail['status'], { label: string; cls: string }> = {
  pending: { label: '排隊中', cls: 'warn' },
  sending: { label: '寄送中', cls: 'warn' },
  sent: { label: '已寄出', cls: 'ok' },
  failed: { label: '失敗', cls: 'bad' },
  skipped: { label: '略過', cls: 'muted' },
}

const queue = ref<MailQueue | null>(null)
const error = ref('')
const busy = ref(0)

async function load() {
  try {
    queue.value = await getMailQueue()
    error.value = ''
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '佇列狀態暫時無法取得'
  }
}
onMounted(load)

async function retry(id: number) {
  busy.value = id
  try {
    await retryMail(id)
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '操作失敗，請再試一次'
  } finally {
    busy.value = 0
  }
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
  <section class="card block" aria-labelledby="mail-queue-title">
    <h2 id="mail-queue-title">
      寄送佇列
      <button type="button" class="refresh" @click="load">
        <span class="mi" aria-hidden="true">refresh</span>重新整理
      </button>
    </h2>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>
    <template v-if="queue">
      <p v-if="queue.sandbox" class="notice warn">沙盒模式：信不會真的寄出。</p>
      <p class="sub">
        近七天：排隊 {{ queue.counts.pending + queue.counts.sending }}・已寄出
        {{ queue.counts.sent }}・失敗 {{ queue.counts.failed }}・略過 {{ queue.counts.skipped
        }}<template v-if="queue.daily_limit"
          >・今天 {{ queue.sent_today }}／{{ queue.daily_limit }} 封</template
        >
      </p>
      <table>
        <thead>
          <tr>
            <th>信件</th>
            <th>狀態</th>
            <th class="num">嘗試</th>
            <th>建立</th>
            <th>寄出／下次嘗試</th>
            <th>原因</th>
            <th><span class="visually-hidden">操作</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="m in queue.recent" :key="m.id">
            <td>{{ m.name }}</td>
            <td>
              <span class="badge" :class="STATUS[m.status].cls">{{ STATUS[m.status].label }}</span>
            </td>
            <td class="num">{{ m.attempts }}</td>
            <td>{{ time(m.created_at) }}</td>
            <td>
              {{
                m.sent_at ? time(m.sent_at) : m.status === 'pending' ? time(m.next_attempt_at) : ''
              }}
            </td>
            <td class="error-cell">{{ m.last_error }}</td>
            <td>
              <button
                v-if="m.status === 'failed'"
                type="button"
                class="retry"
                :disabled="busy === m.id"
                @click="retry(m.id)"
              >
                重寄
              </button>
            </td>
          </tr>
          <tr v-if="queue.recent.length === 0">
            <td colspan="7" class="muted">還沒有寄過信</td>
          </tr>
        </tbody>
      </table>
    </template>
  </section>
</template>

<style scoped>
.refresh,
.retry {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: none;
  color: var(--text-muted);
  font-size: var(--font-s);
  font-weight: 400;
  cursor: pointer;
}
.refresh .mi {
  font-size: 1rem;
}
.retry:disabled {
  opacity: 0.5;
}
</style>
