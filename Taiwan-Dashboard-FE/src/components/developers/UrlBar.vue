<script setup lang="ts">
import { ref } from 'vue'

const props = defineProps<{ method: string; base: string; path: string }>()

const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
async function copy() {
  try {
    await navigator.clipboard.writeText(props.base + props.path)
    copied.value = true
    clearTimeout(timer)
    timer = setTimeout(() => (copied.value = false), 1500)
  } catch {}
}
</script>

<template>
  <div class="url">
    <span :class="['method', method.toLowerCase()]">{{ method }}</span>
    <code class="addr"
      ><span class="base">{{ base }}</span
      ><strong>{{ path }}</strong></code
    >
    <button type="button" class="copy" :aria-label="copied ? '已複製' : '複製網址'" @click="copy">
      <span class="mi" aria-hidden="true">{{ copied ? 'check' : 'content_copy' }}</span>
    </button>
  </div>
</template>

<style scoped>
.url {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  padding: 8px 8px 8px 10px;
  border: 1px solid var(--dev-line, var(--border));
  border-radius: 10px;
  background: var(--dev-subtle, var(--surface-muted));
}
.method {
  flex: none;
  padding: 3px 8px;
  border-radius: 6px;
  color: #ffffff;
  font-family: var(--mono, monospace);
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.02em;
}
.method.get {
  background: #1f8f5f;
}
.method.post {
  background: #d9661f;
}
.addr {
  flex: 1;
  min-width: 0;
  font-family: var(--mono, monospace);
  font-size: 0.92rem;
  overflow-wrap: anywhere;
}
.base {
  color: var(--text-muted);
}
.addr strong {
  color: var(--text-primary);
  font-weight: 600;
}
.copy {
  display: grid;
  flex: none;
  place-items: center;
  width: 32px;
  height: 32px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
}
.copy:hover {
  background: var(--dev-line, var(--border));
  color: var(--text-primary);
}
.copy .mi {
  font-size: 18px;
}
</style>
