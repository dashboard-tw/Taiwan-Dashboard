<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'

export type CommandPart = string | { key: string; placeholder: string }

const props = defineProps<{
  parts: CommandPart[]
  mask?: (key: string, value: string) => string
}>()
const values = defineModel<Record<string, string>>({ required: true })

const editing = ref<string | null>(null)
const inputs = ref<HTMLInputElement[]>([])
const copied = ref(false)

async function edit(key: string) {
  editing.value = key
  await nextTick()
  inputs.value[0]?.focus()
}
function shown(p: { key: string; placeholder: string }) {
  const v = values.value[p.key]
  if (!v) return p.placeholder
  return props.mask ? props.mask(p.key, v) : v
}
function set(key: string, v: string) {
  values.value = { ...values.value, [key]: v.trim() }
}

const text = computed(() =>
  props.parts
    .map((p) => (typeof p === 'string' ? p : values.value[p.key] || p.placeholder))
    .join(''),
)
async function copy() {
  try {
    await navigator.clipboard.writeText(text.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    window.prompt('請複製', text.value)
  }
}
</script>

<template>
  <div class="cmd">
    <code class="example"
      ><template v-for="(p, i) in parts" :key="i"
        ><template v-if="typeof p === 'string'">{{ p }}</template
        ><input
          v-else-if="editing === p.key"
          ref="inputs"
          class="slot-input"
          :value="values[p.key] ?? ''"
          :placeholder="p.placeholder"
          :aria-label="p.placeholder"
          :size="Math.max((values[p.key] || p.placeholder).length, 8)"
          autocomplete="off"
          spellcheck="false"
          @input="set(p.key, ($event.target as HTMLInputElement).value)"
          @keydown.enter.prevent="editing = null"
          @blur="editing = null" /><button
          v-else
          type="button"
          class="slot"
          :class="{ filled: !!values[p.key] }"
          :title="`點一下填入${p.placeholder.replace('你的 ', '')}`"
          @click="edit(p.key)"
          v-text="shown(p)"
        ></button></template
    ></code>
    <button type="button" class="copy" :aria-label="copied ? '已複製' : '複製指令'" @click="copy">
      <span class="mi" aria-hidden="true">{{ copied ? 'check' : 'content_copy' }}</span>
    </button>
  </div>
</template>

<style scoped>
.cmd {
  position: relative;
}
.example {
  display: block;
  padding: 6px 2.5rem 6px 10px;
  border-radius: var(--radius);
  background: var(--bg);
  white-space: pre-wrap;
  word-break: break-all;
}
.slot,
.slot-input {
  margin: 0 1px;
  padding: 1px 5px;
  border: 0;
  border-radius: 4px;
  background: var(--accent-soft);
  color: var(--accent);
  font: inherit;
  line-height: 1.4;
  vertical-align: baseline;
  cursor: pointer;
}
.slot.filled {
  color: var(--text-primary);
}
.slot:hover,
.slot-input {
  box-shadow: inset 0 0 0 1px var(--accent);
}
.slot-input {
  max-width: 100%;
  color: var(--text-primary);
  cursor: text;
  outline: none;
}
.copy {
  position: absolute;
  top: 4px;
  right: 4px;
  display: grid;
  place-items: center;
  width: 1.9rem;
  height: 1.9rem;
  border: 0;
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
}
.copy:hover {
  background: var(--surface-muted);
  color: var(--text-primary);
}
.copy .mi {
  font-size: 1rem;
}
</style>
