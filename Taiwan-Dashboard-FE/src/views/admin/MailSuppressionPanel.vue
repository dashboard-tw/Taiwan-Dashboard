<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { deleteMailSuppression, getMailSuppressions, type MailSuppression } from '../../api/admin'
import { ApiError } from '../../api/client'

const REASONS: Record<MailSuppression['reason'], string> = {
  hard_bounce: '退信',
  spam: '被檢舉為垃圾信',
  invalid_email: '無效信箱',
  blocked: '被 Brevo 封鎖',
}

const list = ref<MailSuppression[] | null>(null)
const error = ref('')
const busy = ref(0)

async function load() {
  try {
    list.value = await getMailSuppressions()
    error.value = ''
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '停寄名單暫時無法取得'
  }
}
onMounted(load)

async function remove(s: MailSuppression) {
  const who = s.user ?? '這個信箱'
  if (!window.confirm(`讓${who}恢復收信？`)) return
  busy.value = s.id
  try {
    await deleteMailSuppression(s.id)
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '操作失敗，請再試一次'
  } finally {
    busy.value = 0
  }
}

function date(iso: string) {
  return new Date(iso).toLocaleString('zh-TW', { timeZone: 'Asia/Taipei', hour12: false })
}
</script>

<template>
  <section class="card block" aria-labelledby="mail-suppression-title">
    <h2 id="mail-suppression-title">停寄名單</h2>
    <p class="sub">退信或被檢舉的信箱，不會再收到信。</p>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>
    <table v-if="list">
      <thead>
        <tr>
          <th>帳號</th>
          <th>原因</th>
          <th>說明</th>
          <th>時間</th>
          <th><span class="visually-hidden">操作</span></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="s in list" :key="s.id">
          <td>{{ s.user ?? '找不到帳號' }}</td>
          <td>{{ REASONS[s.reason] }}</td>
          <td class="error-cell">{{ s.detail }}</td>
          <td>{{ date(s.created_at) }}</td>
          <td>
            <button type="button" class="remove" :disabled="busy === s.id" @click="remove(s)">
              恢復收信
            </button>
          </td>
        </tr>
        <tr v-if="list.length === 0">
          <td colspan="5" class="muted">目前沒有停寄的信箱</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.remove {
  padding: 2px 8px;
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  background: none;
  color: var(--text-muted);
  font-size: var(--font-s);
  cursor: pointer;
}
.remove:disabled {
  opacity: 0.5;
}
</style>
