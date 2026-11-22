<script setup lang="ts">
import { computed, ref } from 'vue'
import { useDialogs } from '../../stores/dialog'
import DialogContainer from './DialogContainer.vue'

const dialogs = useDialogs()
const copied = ref(false)

function escapeAttr(v: string): string {
  return v
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

const code = computed(() => {
  const t = dialogs.target
  if (!t) return ''
  const qs = t.county ? `?county=${t.county}` : ''
  const src = `${location.origin}/embed/${t.component.slug}${qs}`
  return [
    '<iframe',
    `  id="dashboard-tw-${escapeAttr(t.component.slug)}"`,
    `  title="${escapeAttr(t.component.name)}"`,
    `  src="${escapeAttr(src)}"`,
    '  width="450"',
    '  height="400"',
    '  style="border-radius: 5px"',
    '  frameborder="0"',
    '  loading="lazy"',
    '></iframe>',
  ].join('\n')
})

async function copy() {
  try {
    await navigator.clipboard.writeText(code.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 2000)
  } catch {
    window.prompt('請複製以下內嵌碼', code.value)
  }
}
</script>

<template>
  <DialogContainer :open="dialogs.embed" label="內嵌組件" @close="dialogs.embed = false">
    <div class="embed">
      <h2>內嵌組件</h2>
      <label class="field">
        <span>複製以下內嵌碼至您的網頁即可內嵌本組件</span>
        <textarea readonly :value="code" @focus="($event.target as HTMLTextAreaElement).select()" />
      </label>
      <div class="control">
        <button type="button" class="btn-highlight" @click="copy">
          {{ copied ? '已複製' : '複製內嵌碼' }}
        </button>
      </div>
    </div>
  </DialogContainer>
</template>

<style scoped>
.embed {
  width: 300px;
}
h2 {
  font-size: var(--font-l);
  margin-bottom: 0.5rem;
}
.field {
  display: flex;
  flex-direction: column;
  margin: 0.5rem 0;
}
.field span {
  margin-bottom: 0.5rem;
  font-size: var(--font-s);
  color: var(--text-muted);
}
textarea {
  height: 160px;
  resize: none;
  padding: 6px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font-family: ui-monospace, Consolas, monospace;
  font-size: var(--font-s);
}
.control {
  display: flex;
  justify-content: flex-end;
}
.control .btn-highlight {
  padding: 4px 10px;
}
</style>
