<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { API_BASE, dataPath, getComponentData } from '../../api/client'
import { useDialogs } from '../../stores/dialog'
import DialogContainer from './DialogContainer.vue'

const dialogs = useDialogs()
const name = ref('')
const format = ref<'JSON' | 'CSV'>('JSON')
const busy = ref(false)
const failed = ref(false)

const target = computed(() => dialogs.target)
const query = computed(() => (target.value?.county ? { county: target.value.county } : {}))

watch(
  () => dialogs.download,
  (open) => {
    if (open && target.value) {
      name.value = target.value.component.name
      format.value = 'JSON'
      failed.value = false
    }
  },
)

function save(href: string, filename: string) {
  const a = document.createElement('a')
  a.href = href
  a.download = filename
  a.click()
}

async function submit() {
  if (!target.value || !name.value.trim()) return
  const slug = target.value.component.slug
  const filename = name.value.trim()
  failed.value = false
  if (format.value === 'CSV') {
    save(`${API_BASE}${dataPath(slug, 'data', query.value, 'csv')}`, `${filename}.csv`)
    dialogs.download = false
    return
  }
  busy.value = true
  try {
    const data = await getComponentData(slug, 'data', query.value)
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    save(url, `${filename}.json`)
    setTimeout(() => URL.revokeObjectURL(url), 1000)
    dialogs.download = false
  } catch {
    failed.value = true
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <DialogContainer :open="dialogs.download" label="下載資料" @close="dialogs.download = false">
    <form class="download" @submit.prevent="submit">
      <h2>下載資料</h2>
      <label class="field">
        <span>請輸入檔名（必填）</span>
        <input v-model="name" type="text" required pattern=".*\S.*" title="請輸入檔名" />
      </label>
      <fieldset>
        <legend>請選擇檔案格式</legend>
        <label class="radio"><input v-model="format" type="radio" value="JSON" />JSON</label>
        <label class="radio"><input v-model="format" type="radio" value="CSV" />CSV (UTF-8)</label>
      </fieldset>
      <p v-if="failed" class="warn">下載失敗，請稍後再試</p>
      <div class="control">
        <button type="button" class="cancel" @click="dialogs.download = false">取消</button>
        <button type="submit" class="btn-highlight" :disabled="busy">下載{{ format }}</button>
      </div>
    </form>
  </DialogContainer>
</template>

<style scoped>
.download {
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
.field span,
legend {
  margin-bottom: 0.5rem;
  font-size: var(--font-s);
  color: var(--text-muted);
}
input[type='text'] {
  font: inherit;
  padding: 4px 6px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
}
input[type='text']:focus {
  border-color: var(--accent);
}
fieldset {
  border: 0;
  margin: 0;
  padding: 0;
}
.radio {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 2px;
  font-size: var(--font-s);
  color: var(--text-muted);
  cursor: pointer;
}
.radio:has(input:checked) {
  color: var(--text-primary);
}
.radio input {
  accent-color: var(--accent);
  margin: 0;
}
.control {
  display: flex;
  justify-content: flex-end;
  gap: 4px;
  margin-top: 0.5rem;
}
.cancel {
  border: 0;
  background: transparent;
  padding: 4px 6px;
  cursor: pointer;
}
.cancel:hover {
  color: var(--accent);
}
.control .btn-highlight {
  padding: 4px 10px;
  font-size: var(--font-ms);
}
.warn {
  font-size: var(--font-s);
}
</style>
