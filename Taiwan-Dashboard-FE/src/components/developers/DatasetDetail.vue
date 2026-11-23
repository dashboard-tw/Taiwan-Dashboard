<script setup lang="ts">
import { computed, ref } from 'vue'
import type { CatalogDataset, CatalogField } from '../../api/catalog'
import { formatFreq, formatObserved } from '../../lib/format'
import CodeBlock from './CodeBlock.vue'

const props = defineProps<{
  dataset: CatalogDataset
  apiBase: string
  pageSize: number
  maxRangeDays: number
}>()

const d = computed(() => props.dataset)
const sameFields = computed(
  () => JSON.stringify(d.value.latest_fields) === JSON.stringify(d.value.fields),
)

const fieldTabs = computed(() => {
  const tabs: { key: string; label: string; fields: CatalogField[] }[] = []
  if (sameFields.value || !d.value.has_history) {
    tabs.push({
      key: 'latest',
      label: d.value.has_history ? '資料欄位' : '欄位',
      fields: d.value.latest_fields,
    })
  } else {
    tabs.push({ key: 'latest', label: 'latest 欄位', fields: d.value.latest_fields })
    tabs.push({ key: 'history', label: 'history 欄位', fields: d.value.fields })
  }
  if (d.value.has_sites)
    tabs.push({ key: 'sites', label: 'sites 欄位', fields: d.value.site_fields })
  return tabs
})
const tab = ref(0)
const fields = computed(() => fieldTabs.value[Math.min(tab.value, fieldTabs.value.length - 1)]!)

const TYPE_LABELS: Record<string, string> = {
  string: '文字',
  integer: '整數',
  number: '數字',
  boolean: '是/否',
  timestamp: '時間',
  date: '日期',
}

const endpoints = computed(() => {
  const base = `/datasets/${d.value.id}`
  const out = [
    {
      path: `${base}/latest`,
      text: d.value.latest_paged ? `完整清單，每頁 ${props.pageSize} 筆` : '各觀測點最新一筆',
    },
  ]
  if (d.value.has_history)
    out.push({
      path: `${base}/history`,
      text: `歷史資料，每次最多 ${props.maxRangeDays} 天`,
    })
  if (d.value.has_sites) out.push({ path: `${base}/sites`, text: '觀測點代碼與位置' })
  if (d.value.has_downloads)
    out.push({ path: `/downloads/${d.value.id}/index.json`, text: '每月 CSV 大量下載' })
  return out
})

const filters = computed(() => {
  const f: string[] = []
  if (d.value.has_county) f.push('county 縣市代碼')
  if (d.value.has_sites) f.push('site 觀測點代碼')
  f.push('format 輸出格式')
  return f
})

const examples = computed(() => {
  const url = `${props.apiBase}/datasets/${d.value.id}`
  const q = d.value.has_county ? '?county=63000' : ''
  const out: Record<string, string> = {
    latest: `curl "${url}/latest${q}" \\\n  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN"`,
  }
  if (d.value.has_history)
    out.history = `curl "${url}/history?from=2026-11-01&to=2026-11-08${q ? '&county=63000' : ''}" \\\n  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN"`
  if (d.value.has_sites)
    out.sites = `curl "${url}/sites" \\\n  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN"`
  out.CSV = `curl "${url}/latest?format=csv" \\\n  -H "Authorization: Bearer $DASHBOARD_TW_TOKEN" -o ${d.value.id}.csv`
  return out
})

const STATUS: Record<string, { label: string; cls: string }> = {
  ok: { label: '正常更新', cls: 'ok' },
  stale: { label: '來源延遲', cls: 'warn' },
  error: { label: '暫停更新', cls: 'warn' },
  pending: { label: '尚無資料', cls: 'muted' },
}
const status = computed(() => STATUS[d.value.status] ?? STATUS.pending!)
</script>

<template>
  <div class="detail">
    <p class="desc">{{ d.description }}</p>

    <dl class="facts">
      <div>
        <dt>資料來源</dt>
        <dd>
          <a :href="d.source_url" target="_blank" rel="noopener">{{ d.agency }}</a>
        </dd>
      </div>
      <div>
        <dt>授權</dt>
        <dd>{{ d.license }}</dd>
      </div>
      <div>
        <dt>更新頻率</dt>
        <dd>{{ formatFreq(d.update_freq_seconds) || '—' }}</dd>
      </div>
      <div>
        <dt>最新資料</dt>
        <dd>
          <span :class="['dot', status.cls]" aria-hidden="true"></span>{{ status.label }}
          <span v-if="d.last_observed_at" class="muted"
            >· {{ formatObserved(d.last_observed_at) }}</span
          >
        </dd>
      </div>
    </dl>

    <h2>端點</h2>
    <ul class="endpoints">
      <li v-for="e in endpoints" :key="e.path">
        <span class="method">GET</span>
        <code>{{ e.path }}</code>
        <span class="muted">{{ e.text }}</span>
      </li>
    </ul>
    <p class="muted small">可用的參數：{{ filters.join('、') }}</p>

    <h2>欄位</h2>
    <div v-if="fieldTabs.length > 1" class="seg" role="tablist" aria-label="欄位">
      <button
        v-for="(t, i) in fieldTabs"
        :key="t.key"
        type="button"
        role="tab"
        :aria-selected="i === tab"
        :class="{ on: i === tab }"
        @click="tab = i"
      >
        {{ t.label }}
      </button>
    </div>
    <div class="table-wrap">
      <table class="fields">
        <thead>
          <tr>
            <th scope="col">欄位</th>
            <th scope="col">型別</th>
            <th scope="col">說明</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="f in fields.fields" :key="f.name">
            <td>
              <code>{{ f.name }}</code>
            </td>
            <td class="type">
              {{ TYPE_LABELS[f.type] ?? f.type
              }}<span v-if="f.unit" class="unit">{{ f.unit }}</span>
            </td>
            <td>{{ f.description }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <h2>呼叫範例</h2>
    <CodeBlock :code="examples" label="端點" />
  </div>
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  gap: 14px;
}
.desc {
  color: var(--text-secondary);
  line-height: 1.75;
}
.facts {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(170px, 1fr));
  gap: 12px 20px;
  margin: 0;
  padding: 14px 16px;
  border-radius: 10px;
  background: var(--dev-subtle);
}
.facts dt {
  color: var(--text-muted);
  font-size: 0.75rem;
}
.facts dd {
  margin: 2px 0 0;
  font-size: 0.9rem;
}
.dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  margin-right: 6px;
  border-radius: 50%;
  vertical-align: 1px;
  background: var(--text-muted);
}
.dot.ok {
  background: #2fb36b;
}
.dot.warn {
  background: #e0a32b;
}
h2 {
  margin-top: 6px;
  font-size: 0.85rem;
  font-weight: 600;
  letter-spacing: 0.02em;
  color: var(--text-primary);
}
.endpoints {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.endpoints li {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px;
  font-size: 0.88rem;
}
.method {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.04em;
}
code {
  font-family: ui-monospace, 'SF Mono', 'Cascadia Code', Consolas, monospace;
  font-size: 0.85em;
}
.seg {
  display: inline-flex;
  align-self: flex-start;
  gap: 2px;
  padding: 3px;
  border-radius: 8px;
  background: var(--dev-subtle);
}
.seg button {
  padding: 4px 12px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--text-muted);
  font-size: 0.82rem;
  cursor: pointer;
}
.seg button.on {
  background: var(--surface);
  color: var(--text-primary);
  box-shadow: 0 1px 2px rgb(0 0 0 / 12%);
}
.table-wrap {
  overflow-x: auto;
  border: 1px solid var(--dev-line);
  border-radius: 10px;
}
.fields {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.86rem;
}
.fields th {
  padding: 8px 12px;
  border-bottom: 1px solid var(--dev-line);
  background: var(--dev-subtle);
  color: var(--text-muted);
  font-size: 0.75rem;
  font-weight: 500;
  text-align: left;
}
.fields td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--dev-line);
  vertical-align: top;
  line-height: 1.6;
}
.fields tr:last-child td {
  border-bottom: 0;
}
.fields td:first-child {
  white-space: nowrap;
}
.type {
  white-space: nowrap;
  color: var(--text-muted);
}
.unit {
  margin-left: 6px;
  padding: 0 5px;
  border-radius: 4px;
  background: var(--dev-subtle);
  font-size: 0.75rem;
}
.muted {
  color: var(--text-muted);
}
.small {
  font-size: 0.8rem;
}
</style>
