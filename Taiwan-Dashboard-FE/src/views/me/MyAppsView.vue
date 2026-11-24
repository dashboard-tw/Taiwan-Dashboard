<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getApiClients, type ApiClientsState } from '../../api/me'
import MyApps from '../../components/MyApps.vue'
import { usePageTitle } from '../../composables/usePageTitle'

usePageTitle('申請應用活化')

const state = ref<ApiClientsState | null>(null)
async function load() {
  try {
    state.value = await getApiClients()
  } catch {}
}
onMounted(load)

const MB = 1024 * 1024
const rows = computed(() => {
  const f =
    (state.value?.tier === 'free' ? state.value.limits : null) ??
    ({ PerDay: 600, PerMinute: 30, BytesPerDay: 300 * MB } as const)
  const p = state.value?.partner_limits ?? { PerDay: 3000, PerMinute: 60, BytesPerDay: 1536 * MB }
  const mb = (b: number) =>
    b >= 1024 * MB ? `${+(b / 1024 / MB).toFixed(1)} GB` : `${Math.round(b / MB)} MB`
  return [
    {
      label: '每天次數',
      free: `${f.PerDay.toLocaleString()} 次`,
      partner: `${p.PerDay.toLocaleString()} 次`,
    },
    { label: '每分鐘', free: `${f.PerMinute} 次`, partner: `${p.PerMinute} 次` },
    { label: '每天流量', free: mb(f.BytesPerDay), partner: mb(p.BytesPerDay) },
  ]
})
const isPartner = computed(() => state.value?.tier === 'partner')
const hasClient = computed(() => (state.value ? state.value.clients.length > 0 : null))
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">apps</span>申請應用活化</h1>

    <div class="grid">
      <div class="col">
        <section class="card block">
          <h2><span class="step">1</span>關於應用活化</h2>
          <p>
            用本站 API 做的網站、App 或作品，都可以申請放上<RouterLink to="/showcase/app"
              >應用活化</RouterLink
            >。
          </p>
          <p class="muted small">
            還沒有憑證？請先到<RouterLink to="/me/api-key">API 憑證</RouterLink>頁申請。
          </p>
        </section>

        <section class="card block">
          <h2>
            <span class="step">2</span>合作額度
            <span class="badge">{{ isPartner ? '目前：合作額度' : '目前：一般額度' }}</span>
          </h2>
          <p>應用上架後，額度會自動提高。名下沒有上架中的應用時，會自動恢復為一般額度。</p>
          <table>
            <thead>
              <tr>
                <th></th>
                <th>一般</th>
                <th>合作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in rows" :key="r.label">
                <th scope="row">{{ r.label }}</th>
                <td>{{ r.free }}</td>
                <td class="strong">{{ r.partner }}</td>
              </tr>
            </tbody>
          </table>
          <p class="muted small">應用下架後恢復一般額度。</p>
        </section>

        <section class="card block">
          <h2><span class="step">3</span>申請流程</h2>
          <ol class="steps">
            <li>填寫應用資料</li>
            <li>等待審核</li>
            <li>通過後上架</li>
          </ol>
          <p class="muted small">上架後修改內容，需要重新審核。</p>
        </section>
      </div>

      <div class="col">
        <MyApps :has-client="hasClient" @changed="load" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--font-m);
  align-items: start;
}
@media (min-width: 68.75em) {
  .grid {
    grid-template-columns: minmax(0, 2fr) minmax(0, 3fr);
  }
}
.col {
  display: flex;
  flex-direction: column;
  gap: var(--font-m);
  min-width: 0;
}
.block {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin: 0;
  padding: var(--font-m);
  line-height: 1.7;
}
h2 {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: var(--font-ms);
}
p {
  margin: 0;
}
.step {
  display: inline-grid;
  place-items: center;
  width: 1.5rem;
  height: 1.5rem;
  border-radius: 50%;
  background: var(--accent);
  color: #ffffff;
  font-size: 0.8125rem;
}
.badge {
  margin-left: auto;
  padding: 0 8px;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: var(--font-s);
  font-weight: 400;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.875rem;
}
th,
td {
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
  text-align: right;
  font-variant-numeric: tabular-nums;
}
th:first-child {
  text-align: left;
}
thead th,
tbody th {
  color: var(--text-secondary);
  font-weight: 400;
}
.strong {
  color: var(--accent);
  font-weight: 600;
}
.steps {
  margin: 0;
  padding-left: 1.25rem;
}
.steps li + li {
  margin-top: 4px;
}
.small {
  font-size: var(--font-s);
}
</style>
