<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ApiSurvey } from '../api/me'
import { IDENTITIES, PURPOSES, PUSH, TOPICS } from '../lib/apiSurvey'

const props = defineProps<{
  initial: ApiSurvey | null
  submitLabel: string
  busy: boolean
  error: string
}>()
const emit = defineEmits<{ submit: [survey: ApiSurvey] }>()

const blank = (): ApiSurvey => ({
  identity: '',
  purpose: '',
  app_name: '',
  app_url: '',
  topics: [],
  suggestion: '',
  wants_push: '',
})
const form = ref<ApiSurvey>(blank())
watch(
  () => props.initial,
  (s) => {
    form.value = s ? { ...s, topics: [...s.topics] } : blank()
  },
  { immediate: true },
)
const agreed = ref(false)

const ready = computed(
  () =>
    !!form.value.identity &&
    !!form.value.purpose &&
    form.value.topics.length > 0 &&
    !!form.value.wants_push &&
    agreed.value,
)

function toggle(t: string, on: boolean) {
  const list = form.value.topics.filter((x) => x !== t)
  form.value.topics = on ? [...list, t] : list
}
</script>

<template>
  <form class="survey" @submit.prevent="ready && emit('submit', form)">
    <label class="field">
      <span>身分類別<em>（必填）</em></span>
      <select v-model="form.identity" class="select" required>
        <option value="" disabled>請選擇</option>
        <option v-for="(label, k) in IDENTITIES" :key="k" :value="k">{{ label }}</option>
      </select>
    </label>
    <label class="field">
      <span>資料用途<em>（必填）</em></span>
      <select v-model="form.purpose" class="select" required>
        <option value="" disabled>請選擇</option>
        <option v-for="(label, k) in PURPOSES" :key="k" :value="k">{{ label }}</option>
      </select>
    </label>
    <fieldset class="field">
      <legend>關注的資料<em>（可複選，必填）</em></legend>
      <div class="checks">
        <label v-for="(label, k) in TOPICS" :key="k" class="check">
          <input
            type="checkbox"
            :checked="form.topics.includes(k)"
            @change="toggle(k, ($event.target as HTMLInputElement).checked)"
          />{{ label }}
        </label>
      </div>
    </fieldset>
    <label class="field">
      <span>應用或專案名稱</span>
      <input
        v-model="form.app_name"
        class="input"
        maxlength="50"
        placeholder="例如：空污與氣喘研究"
      />
    </label>
    <label class="field">
      <span>應用或專案連結</span>
      <input
        v-model="form.app_url"
        class="input"
        type="url"
        maxlength="200"
        placeholder="https://"
      />
    </label>
    <fieldset class="field push">
      <legend>希望未來新增 MQTT 即時推送嗎<em>（必填）</em></legend>
      <p class="hint">資料更新時主動通知你的程式。目前還沒有這個功能。</p>
      <label v-for="k in ['yes', 'maybe', 'no']" :key="k" class="check">
        <input v-model="form.wants_push" type="radio" name="wants_push" :value="k" />{{ PUSH[k] }}
      </label>
    </fieldset>
    <label class="field">
      <span>希望新增的資料或建議</span>
      <textarea
        v-model="form.suggestion"
        class="input"
        rows="3"
        maxlength="500"
        placeholder="例如：希望有雨量站資料、需要更長的歷史資料"
      ></textarea>
    </label>
    <label class="check agree">
      <input v-model="agreed" type="checkbox" />我同意 API 使用規範
    </label>
    <p v-if="error" class="warn small" role="alert">{{ error }}</p>
    <button type="submit" class="btn-highlight" :disabled="!ready || busy">
      {{ busy ? '處理中' : submitLabel }}
    </button>
  </form>
</template>

<style scoped>
.survey {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  max-width: 32rem;
}
.field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  border: 0;
  font-size: 0.875rem;
}
.field > span,
legend {
  color: var(--text-secondary);
}
legend {
  margin-bottom: 4px;
  padding: 0;
}
em {
  color: var(--danger-text);
  font-style: normal;
}
.input,
.select {
  padding: 5px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--bg);
  color: var(--text-primary);
  font: inherit;
}
textarea.input {
  resize: vertical;
}
.checks {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem 1.25rem;
}
.check {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 0.875rem;
}
.agree {
  padding-top: 0.5rem;
  border-top: 1px solid var(--border);
}
.btn-highlight {
  align-self: flex-start;
}
.btn-highlight:disabled {
  opacity: 0.5;
  cursor: default;
}
.small {
  font-size: var(--font-s);
}
</style>
