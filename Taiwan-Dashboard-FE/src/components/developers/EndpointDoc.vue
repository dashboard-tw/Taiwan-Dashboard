<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { Endpoint } from '../../lib/apiReference'
import CodeBlock from './CodeBlock.vue'
import UrlBar from './UrlBar.vue'

const props = defineProps<{ endpoint: Endpoint; apiBase: string; origin: string }>()

const e = computed(() => props.endpoint)
const base = computed(() => (e.value.path.startsWith('/downloads') ? props.origin : props.apiBase))
const IN_LABELS: Record<string, string> = { path: '路徑參數', query: '查詢參數', form: '表單欄位' }
const groups = computed(() =>
  (['path', 'query', 'form'] as const)
    .map((k) => ({ key: k, label: IN_LABELS[k]!, items: e.value.params.filter((p) => p.in === k) }))
    .filter((g) => g.items.length),
)
const examples = computed(() => e.value.example(props.apiBase, props.origin))
</script>

<template>
  <article class="ep">
    <h1>{{ e.title }}</h1>
    <UrlBar :method="e.method" :base="base" :path="e.path" />
    <p class="summary">{{ e.summary }}</p>

    <section class="panel">
      <h2 class="panel-title"><span class="mi" aria-hidden="true">key</span>驗證</h2>
      <p v-if="e.auth">
        在標頭加上 <code>Authorization</code>，值為 <code>Bearer</code> 加上 Access Token。
        <RouterLink to="/developers/endpoints/token">如何取得 token</RouterLink>
      </p>
      <p v-else>不需要 Access Token。請以 <code>application/x-www-form-urlencoded</code> 送出。</p>
      <code v-if="e.auth" class="sample">Authorization: Bearer Yp3kQ...</code>
    </section>

    <h2 class="sec-title">請求</h2>
    <div class="split">
      <div class="params">
        <p v-if="!groups.length" class="muted">沒有參數。</p>
        <section v-for="g in groups" :key="g.key" class="param-group">
          <h3>{{ g.label }}</h3>
          <ul>
            <li v-for="p in g.items" :key="p.name">
              <div class="param-head">
                <code class="name">{{ p.name }}</code>
                <span class="type">{{ p.type }}</span>
                <span v-if="p.required" class="req">必填</span>
              </div>
              <p>{{ p.text }}</p>
            </li>
          </ul>
        </section>
      </div>
      <CodeBlock :code="examples" label="範例" />
    </div>

    <h2 class="sec-title">回應</h2>
    <div class="split">
      <ul class="statuses">
        <li v-for="s in e.statuses" :key="`${s.code}-${s.name}`">
          <span :class="['code', `c${Math.floor(s.code / 100)}`]">{{ s.code }}</span>
          <code>{{ s.name }}</code>
          <span class="muted">{{ s.text }}</span>
        </li>
      </ul>
      <CodeBlock :code="e.response" label="200 回應" />
    </div>
  </article>
</template>

<style scoped>
.ep {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
h1 {
  font-size: clamp(1.6rem, 3vw, 2.1rem);
  font-weight: 700;
  letter-spacing: -0.01em;
}
.summary {
  color: var(--text-secondary);
  font-size: 1.02rem;
  line-height: 1.8;
}
.panel {
  padding: 16px 18px;
  border: 1px solid var(--dev-line);
  border-radius: 12px;
  line-height: 1.8;
}
.panel-title {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  font-size: 1rem;
  font-weight: 600;
}
.panel-title .mi {
  color: var(--text-muted);
  font-size: 1.125rem;
}
.panel p {
  color: var(--text-secondary);
}
.sample {
  display: block;
  margin-top: 8px;
  padding: 8px 12px;
  overflow-x: auto;
  border-radius: 8px;
  background: var(--dev-subtle);
  white-space: nowrap;
}
.sec-title {
  margin-top: 14px;
  font-size: 1.35rem;
  font-weight: 700;
}
.split {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 16px;
}
@media (min-width: 73.75em) {
  .split {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 28px;
    align-items: start;
  }
}
.param-group + .param-group {
  margin-top: 18px;
}
.param-group h3 {
  padding-bottom: 8px;
  border-bottom: 1px solid var(--dev-line);
  color: var(--text-muted);
  font-size: 0.8rem;
  font-weight: 600;
  letter-spacing: 0.06em;
}
.param-group ul,
.statuses {
  margin: 0;
  padding: 0;
  list-style: none;
}
.param-group li {
  padding: 12px 0;
  border-bottom: 1px solid var(--dev-line);
}
.param-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
.name {
  padding: 2px 8px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--accent) 12%, transparent);
  color: var(--accent);
  font-weight: 600;
}
.type {
  color: var(--text-muted);
  font-family: var(--mono);
  font-size: 0.82rem;
}
.req {
  margin-left: auto;
  color: var(--verb-post);
  font-size: 0.8rem;
}
.param-group li p {
  margin-top: 6px;
  color: var(--text-secondary);
  font-size: 0.9rem;
  line-height: 1.7;
}
.statuses li {
  display: grid;
  grid-template-columns: 44px minmax(0, auto) 1fr;
  gap: 10px;
  align-items: baseline;
  padding: 10px 0;
  border-bottom: 1px solid var(--dev-line);
  font-size: 0.9rem;
}
.code {
  font-family: var(--mono);
  font-weight: 700;
}
.code.c2 {
  color: var(--verb-get);
}
.code.c3 {
  color: var(--text-muted);
}
.code.c4 {
  color: var(--verb-post);
}
@media (max-width: 40em) {
  .statuses li {
    grid-template-columns: 44px minmax(0, 1fr);
  }
  .statuses li .muted {
    grid-column: 2;
  }
}
</style>
