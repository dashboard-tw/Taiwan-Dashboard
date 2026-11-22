<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  cancelPendingLink,
  getPendingLink,
  getProviders,
  type PendingLink,
  type Provider,
} from '../api/auth'
import { ApiError } from '../api/client'
import { openAuthPopup, waitForAuthPopup } from '../lib/authPopup'
import { useAuth } from '../stores/auth'

const props = defineProps<{
  redirect: string
  error?: string
}>()
const emit = defineEmits<{ done: [] }>()

const auth = useAuth()
const router = useRouter()

const providers = ref<Provider[] | null>(null)
const message = ref('')
const busy = ref('')

const ERRORS: Record<string, string> = {
  cancelled: '已取消登入。',
  expired: '登入逾時，請重新登入。',
  failed: '登入失敗，請再試一次。',
  too_new: 'GitHub 帳號需註冊滿 30 天才能登入，可以先改用 Google 登入。',
  disabled: '此帳號已停用。',
  unavailable: '目前不提供這個登入方式。',
}

const NAMES: Record<string, string> = { github: 'GitHub', google: 'Google' }
const nameOf = (id: string) => NAMES[id] ?? id

const link = ref<PendingLink | null>(null)

onMounted(async () => {
  if (props.error === 'link') {
    try {
      link.value = await getPendingLink()
    } catch {
      link.value = null
    }
    if (!link.value) message.value = '綁定逾時，請重新登入。'
  } else if (props.error) {
    message.value = ERRORS[props.error] ?? ERRORS.failed!
  }
  try {
    providers.value = await getProviders()
  } catch {
    providers.value = []
    message.value = '暫時無法登入，請稍後再試。'
  }
})

const real = computed(() => {
  const all = (providers.value ?? []).filter((p) => !p.dev)
  return link.value ? all.filter((p) => link.value!.existing.includes(p.id)) : all
})
const dev = computed(() => (link.value ? [] : (providers.value ?? []).filter((p) => p.dev)))
const existingNames = computed(() => (link.value?.existing ?? []).map(nameOf).join('或'))

async function cancelLink() {
  try {
    await cancelPendingLink()
  } catch {}
  link.value = null
}

const waiting = ref<{ popup: Window; abort: AbortController } | null>(null)

async function loginInPopup(p: Provider): Promise<boolean> {
  if (typeof BroadcastChannel === 'undefined') return false
  const popup = openAuthPopup({ mode: 'login', provider: p.id, redirect: props.redirect })
  if (!popup) return false
  const abort = new AbortController()
  waiting.value = { popup, abort }
  busy.value = p.id
  try {
    const result = await waitForAuthPopup(abort.signal)
    if ('error' in result) {
      message.value = result.error
    } else {
      await auth.load(true)
      emit('done')
      void router.replace(result.url)
    }
  } catch {
  } finally {
    waiting.value = null
    busy.value = ''
  }
  return true
}

function cancelPopup() {
  waiting.value?.popup.close()
  waiting.value?.abort.abort()
}

async function login(p: Provider) {
  if (!p.dev && (await loginInPopup(p))) return
  busy.value = p.id
  message.value = ''
  try {
    if (await auth.login(p.id, props.redirect)) {
      emit('done')
      void router.replace(props.redirect)
    } else {
      return // 前往 GitHub、Google：保持「前往中」，避免重複按
    }
  } catch (e) {
    message.value = e instanceof ApiError ? e.message : '登入失敗，請再試一次。'
  }
  busy.value = ''
}
</script>

<template>
  <div class="panel">
    <div v-if="link" class="link-box" role="status">
      <p class="link-title"><span class="mi" aria-hidden="true">link</span>這個 Email 已註冊過</p>
      <p>
        請先用 {{ existingNames }} 登入，完成後會自動綁定
        {{ nameOf(link.provider) }}，以後兩種方式都能登入同一個帳號。
      </p>
      <p class="muted small">請在 10 分鐘內完成。</p>
    </div>
    <div v-if="providers === null" class="buttons" aria-label="載入中">
      <div class="skeleton button-skeleton"></div>
      <div class="skeleton button-skeleton"></div>
    </div>
    <div v-else class="buttons">
      <button
        v-for="p in real"
        :key="p.id"
        type="button"
        :class="['provider', p.id]"
        :disabled="!!busy"
        @click="login(p)"
      >
        <svg v-if="p.id === 'github'" viewBox="0 0 24 24" aria-hidden="true">
          <path
            fill="currentColor"
            d="M12 .5a11.5 11.5 0 0 0-3.64 22.41c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.37-3.88-1.37-.52-1.33-1.28-1.69-1.28-1.69-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.03 1.76 2.69 1.25 3.35.96.1-.75.4-1.25.73-1.54-2.55-.29-5.24-1.28-5.24-5.69 0-1.26.45-2.28 1.18-3.09-.12-.29-.51-1.46.11-3.05 0 0 .97-.31 3.17 1.18a11 11 0 0 1 5.77 0c2.2-1.49 3.17-1.18 3.17-1.18.62 1.59.23 2.76.11 3.05.74.81 1.18 1.83 1.18 3.09 0 4.42-2.7 5.39-5.26 5.68.41.36.78 1.06.78 2.14v3.17c0 .31.21.67.8.56A11.5 11.5 0 0 0 12 .5Z"
          />
        </svg>
        <svg v-else-if="p.id === 'google'" viewBox="0 0 24 24" aria-hidden="true">
          <path
            fill="#4285F4"
            d="M23.5 12.27c0-.85-.08-1.67-.22-2.45H12v4.64h6.45a5.52 5.52 0 0 1-2.39 3.62v3h3.86c2.26-2.08 3.58-5.15 3.58-8.81Z"
          />
          <path
            fill="#34A853"
            d="M12 24c3.24 0 5.95-1.07 7.93-2.91l-3.86-3c-1.07.72-2.45 1.15-4.07 1.15-3.13 0-5.78-2.11-6.73-4.95H1.28v3.1A12 12 0 0 0 12 24Z"
          />
          <path
            fill="#FBBC05"
            d="M5.27 14.29A7.2 7.2 0 0 1 4.9 12c0-.8.14-1.57.38-2.29v-3.1H1.28A12 12 0 0 0 0 12c0 1.94.46 3.77 1.28 5.39l3.99-3.1Z"
          />
          <path
            fill="#EA4335"
            d="M12 4.75c1.76 0 3.34.6 4.59 1.8l3.43-3.43A11.5 11.5 0 0 0 12 0 12 12 0 0 0 1.28 6.61l3.99 3.1C6.22 6.86 8.87 4.75 12 4.75Z"
          />
        </svg>
        {{
          busy === p.id
            ? waiting
              ? '請在跳出的視窗登入'
              : '前往中…'
            : link
              ? `使用 ${nameOf(p.id)} 登入`
              : p.label
        }}
      </button>
      <p v-if="waiting" class="muted center small">
        沒看到視窗？請允許本站開啟彈出式視窗。
        <button type="button" class="text-btn" @click="cancelPopup">取消</button>
      </p>
      <button v-if="link" type="button" class="text-btn" :disabled="!!busy" @click="cancelLink">
        取消綁定
      </button>

      <template v-if="dev.length">
        <p class="divider"><span>開發環境</span></p>
        <button
          v-for="p in dev"
          :key="p.id"
          type="button"
          class="provider dev-btn"
          :disabled="!!busy"
          @click="login(p)"
        >
          {{ p.label }}
        </button>
      </template>
      <p v-if="providers.length === 0 && !message" class="muted center">目前暫停登入。</p>
    </div>
    <p v-if="message" class="warn" role="alert">{{ message }}</p>
  </div>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.buttons {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.provider {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  height: 44px;
  border: 1px solid var(--border);
  border-radius: 999px;
  background: var(--surface);
  color: var(--text-primary);
  font-size: var(--font-ms);
  font-weight: 600;
  cursor: pointer;
  transition:
    background 0.15s,
    border-color 0.15s,
    filter 0.15s;
}
.provider:hover:not(:disabled) {
  border-color: var(--border-strong);
  background: var(--surface-muted);
}
.provider.github {
  border-color: var(--border-strong);
  background: #24292f;
  color: #ffffff;
}
.provider.github:hover:not(:disabled) {
  background: #32383f;
}
.provider.google {
  border-color: #dadce0;
  background: #ffffff;
  color: #1f1f1f;
}
.provider.google:hover:not(:disabled) {
  background: #f6f8fa;
}
.provider:disabled {
  opacity: 0.6;
  cursor: default;
}
.provider svg {
  width: 18px;
  height: 18px;
}
.dev-btn {
  border-style: dashed;
  font-weight: 400;
}
.divider {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 4px 0 0;
  color: var(--text-muted);
  font-size: var(--font-s);
}
.divider::before,
.divider::after {
  content: '';
  flex: 1;
  border-top: 1px solid var(--border);
}
.button-skeleton {
  height: 44px;
  border-radius: 999px;
}
.warn {
  color: var(--warning-text);
  font-size: 0.85rem;
  line-height: 1.6;
}
.muted {
  color: var(--text-muted);
}
.center {
  text-align: center;
}
.link-box {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 14px;
  border: 1px solid color-mix(in srgb, var(--accent) 45%, transparent);
  border-radius: 12px;
  background: color-mix(in srgb, var(--accent) 8%, transparent);
  font-size: 0.85rem;
  line-height: 1.7;
}
.link-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 700;
  font-size: 0.95rem;
}
.link-title .mi {
  color: var(--accent);
  font-size: 1.1rem;
}
.small {
  font-size: var(--font-s);
}
.text-btn {
  align-self: center;
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font: inherit;
  font-size: 0.85rem;
  text-decoration: underline;
  cursor: pointer;
}
.text-btn:hover:not(:disabled) {
  color: var(--text-primary);
}
</style>
