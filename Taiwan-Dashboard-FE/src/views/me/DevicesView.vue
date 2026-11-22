<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ApiError } from '../../api/client'
import { getDevices, signOutDevice, signOutOtherDevices, type Device } from '../../api/me'
import { usePageTitle } from '../../composables/usePageTitle'
import { describeUserAgent } from '../../lib/userAgent'
import { useAuth } from '../../stores/auth'

usePageTitle('登入裝置')
const auth = useAuth()
const router = useRouter()

const devices = ref<Device[] | null>(null)
const error = ref('')
const busy = ref<number | 'others' | null>(null)
const notice = ref('')

async function load() {
  try {
    devices.value = await getDevices()
    error.value = ''
  } catch {
    error.value = '無法載入登入裝置，請重新整理。'
  }
}
onMounted(load)

const others = computed(() => devices.value?.filter((d) => !d.current).length ?? 0)

const fmt = new Intl.DateTimeFormat('zh-TW', {
  timeZone: 'Asia/Taipei',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
})
function when(iso: string) {
  return fmt.format(new Date(iso))
}
function ago(iso: string | null) {
  if (!iso) return '—'
  const min = Math.floor((Date.now() - new Date(iso).getTime()) / 60000)
  if (min < 5) return '剛剛'
  if (min < 60) return `${min} 分鐘前`
  if (min < 24 * 60) return `${Math.floor(min / 60)} 小時前`
  return `${Math.floor(min / (24 * 60))} 天前`
}

async function signOut(d: Device) {
  busy.value = d.id
  notice.value = ''
  try {
    await signOutDevice(d.id)
    if (d.current) {
      auth.me = null
      void router.replace('/')
      return
    }
    notice.value = '已登出'
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '登出失敗，請再試一次。'
    await load()
  } finally {
    busy.value = null
  }
}

async function signOutOthers() {
  busy.value = 'others'
  notice.value = ''
  try {
    const n = await signOutOtherDevices()
    notice.value = n > 0 ? `已登出 ${n} 台裝置` : '沒有其他裝置'
    await load()
  } catch (e) {
    error.value = e instanceof ApiError ? e.message : '登出失敗，請再試一次。'
  } finally {
    busy.value = null
  }
}
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">devices</span>登入裝置</h1>
    <p class="muted intro">發現不認得的裝置，請立即登出。</p>
    <p v-if="error" class="warn" role="alert">{{ error }}</p>
    <p v-if="notice" class="notice" role="status">{{ notice }}</p>

    <ul class="list" aria-live="polite">
      <template v-if="devices">
        <li v-for="d in devices" :key="d.id" class="card device" :class="{ current: d.current }">
          <span class="mi icon" aria-hidden="true">{{ describeUserAgent(d.user_agent).icon }}</span>
          <div class="info">
            <p class="name">
              {{ describeUserAgent(d.user_agent).browser }} ·
              {{ describeUserAgent(d.user_agent).os }}
              <span v-if="d.current" class="badge">目前裝置</span>
            </p>
            <dl>
              <dt>最後使用</dt>
              <dd>{{ d.current ? '現在' : ago(d.last_seen_at) }}</dd>
              <dt>登入時間</dt>
              <dd>{{ when(d.created_at) }}</dd>
              <dt>登入 IP</dt>
              <dd class="mono">{{ d.ip || '—' }}</dd>
              <dt>自動登出</dt>
              <dd>{{ when(d.expires_at) }}</dd>
            </dl>
          </div>
          <button type="button" class="out" :disabled="busy !== null" @click="signOut(d)">
            <span class="mi" aria-hidden="true">logout</span>{{ busy === d.id ? '登出中' : '登出' }}
          </button>
        </li>
      </template>
      <template v-else-if="!error">
        <li v-for="i in 2" :key="i" class="card device">
          <span class="skeleton icon-box"></span>
          <div class="info">
            <span class="skeleton line wide"></span>
            <span class="skeleton line"></span>
          </div>
        </li>
      </template>
    </ul>

    <button
      v-if="others > 0"
      type="button"
      class="out others"
      :disabled="busy !== null"
      @click="signOutOthers"
    >
      <span class="mi" aria-hidden="true">phonelink_erase</span
      >{{ busy === 'others' ? '登出中' : '登出其他所有裝置' }}
    </button>

    <p class="muted small">
      登入紀錄會在登出 30 天後刪除。<RouterLink to="/privacy">隱私權政策</RouterLink>
    </p>
  </div>
</template>

<style scoped>
.intro {
  max-width: 40rem;
  margin: 0 0 var(--font-m);
}
.list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  max-width: 40rem;
  margin: 0 0 var(--font-m);
  padding: 0;
  list-style: none;
}
.device {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: start;
  gap: 1rem;
  padding: var(--font-m);
}
.device.current {
  border: 1px solid color-mix(in srgb, var(--accent) 55%, transparent);
}
.icon {
  font-size: 2rem;
  color: var(--text-muted);
}
.current .icon {
  color: var(--accent);
}
.icon-box {
  width: 2rem;
  height: 2rem;
}
.info {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  min-width: 0;
}
.name {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  margin: 0;
  font-weight: 600;
}
.badge {
  padding: 1px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--accent) 18%, transparent);
  color: var(--accent);
  font-size: var(--font-s);
  font-weight: 500;
}
dl {
  display: grid;
  grid-template-columns: 5rem 1fr;
  gap: 0.25rem 0.75rem;
  margin: 0;
  font-size: 0.875rem;
}
dt {
  color: var(--text-muted);
}
dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.mono {
  font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
}
.out {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  white-space: nowrap;
  cursor: pointer;
}
.out:hover:not(:disabled) {
  border-color: var(--danger-text);
  color: var(--danger-text);
}
.out:disabled {
  opacity: 0.45;
  cursor: default;
}
.others {
  margin-bottom: var(--font-m);
}
.notice {
  color: var(--accent);
}
.line {
  display: inline-block;
  width: 8rem;
  height: 0.9rem;
}
.line.wide {
  width: 12rem;
}
.small {
  font-size: var(--font-s);
}
.small a {
  color: var(--accent);
}
@media (max-width: 32.5em) {
  .device {
    grid-template-columns: auto 1fr;
  }
  .device .out {
    grid-column: 1 / -1;
    justify-self: start;
  }
}
</style>
