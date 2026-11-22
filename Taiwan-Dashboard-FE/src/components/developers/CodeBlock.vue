<script setup lang="ts">
import { computed, ref } from 'vue'
import { detectLang, highlight } from '../../lib/highlight'

const props = defineProps<{
  code: string | Record<string, string>
  label?: string
}>()

const tabs = computed(() => (typeof props.code === 'string' ? null : Object.keys(props.code)))
const active = ref(0)
const text = computed(() =>
  typeof props.code === 'string' ? props.code : (props.code[tabs.value![active.value]!] ?? ''),
)

const html = computed(() =>
  highlight(text.value, detectLang(text.value, tabs.value?.[active.value])),
)

const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
async function copy() {
  try {
    await navigator.clipboard.writeText(text.value)
    copied.value = true
    clearTimeout(timer)
    timer = setTimeout(() => (copied.value = false), 1500)
  } catch {}
}
</script>

<template>
  <div class="code">
    <div class="bar">
      <div v-if="tabs" class="tabs" role="tablist" :aria-label="label ?? '程式語言'">
        <button
          v-for="(t, i) in tabs"
          :key="t"
          type="button"
          role="tab"
          :aria-selected="i === active"
          :class="{ on: i === active }"
          @click="active = i"
        >
          {{ t }}
        </button>
      </div>
      <span v-else class="label">{{ label }}</span>
      <button
        type="button"
        class="copy"
        :aria-label="copied ? '已複製' : '複製程式碼'"
        @click="copy"
      >
        <span class="mi" aria-hidden="true">{{ copied ? 'check' : 'content_copy' }}</span>
        {{ copied ? '已複製' : '複製' }}
      </button>
    </div>
    <!-- eslint-disable-next-line vue/no-v-html -- highlight() 已跳脫所有內容 -->
    <pre><code v-html="html"></code></pre>
  </div>
</template>

<style scoped>
.code {
  --code-bg: #111418;
  --code-bar: #191d22;
  --code-text: #e6e9ee;
  --code-muted: #8b939e;
  overflow: hidden;
  border: 1px solid #262b31;
  border-radius: 10px;
  background: var(--code-bg);
}
.bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  min-height: 38px;
  padding: 0 6px 0 12px;
  border-bottom: 1px solid #262b31;
  background: var(--code-bar);
}
.tabs {
  display: flex;
  gap: 2px;
}
.tabs button,
.copy {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--code-muted);
  font-size: 0.8rem;
  cursor: pointer;
  transition:
    color 0.15s,
    background 0.15s;
}
.tabs button:hover,
.copy:hover {
  color: var(--code-text);
}
.tabs button.on {
  background: #262b31;
  color: var(--code-text);
}
.copy .mi {
  font-size: 0.95rem;
}
.label {
  color: var(--code-muted);
  font-size: 0.8rem;
}
pre {
  margin: 0;
  padding: 14px 16px;
  overflow-x: auto;
  color: var(--code-text);
  font-family: ui-monospace, 'SF Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.82rem;
  line-height: 1.65;
  tab-size: 2;
}
pre :deep(.tok-c) {
  color: #7d8590;
  font-style: italic;
}
pre :deep(.tok-s) {
  color: #a5d6a7;
}
pre :deep(.tok-p) {
  color: #79c0ff;
}
pre :deep(.tok-n) {
  color: #f0a875;
}
pre :deep(.tok-k) {
  color: #ff7b9c;
}
pre :deep(.tok-f) {
  color: #d2a8ff;
}
pre :deep(.tok-v) {
  color: #e3c07b;
}
pre :deep(.tok-fn) {
  color: #8ddbe0;
}
</style>
