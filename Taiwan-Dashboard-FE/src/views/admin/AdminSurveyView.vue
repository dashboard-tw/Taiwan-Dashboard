<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getSurveySummary, type SurveySummary } from '../../api/admin'
import { usePageTitle } from '../../composables/usePageTitle'
import { STATUS_ICONS, STATUS_LABELS, statusColor } from '../../lib/accountStatus'
import { IDENTITIES, PURPOSES, PUSH, TOPICS, topicNames } from '../../lib/apiSurvey'

usePageTitle('API 使用調查')

const data = ref<SurveySummary | null>(null)
const error = ref('')
onMounted(async () => {
  try {
    data.value = await getSurveySummary()
  } catch {
    error.value = '調查統計暫時無法取得'
  }
})

const groups = computed(() =>
  data.value
    ? [
        { title: '身分類別', rows: data.value.identity, labels: IDENTITIES },
        { title: '資料用途', rows: data.value.purpose, labels: PURPOSES },
        { title: '關注的資料', rows: data.value.topics, labels: TOPICS },
        { title: '希望新增 MQTT 即時推送', rows: data.value.wants_push ?? [], labels: PUSH },
      ]
    : [],
)
const pct = (n: number) =>
  data.value?.respondents ? Math.round((n / data.value.respondents) * 100) : 0

function date(iso: string) {
  return new Date(iso).toLocaleDateString('zh-TW', { timeZone: 'Asia/Taipei' })
}
</script>

<template>
  <div>
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">poll</span>API 使用調查
      <span v-if="data" class="tools muted"
        >{{ data.respondents }} 人填寫（{{ data.active_respondents }} 人仍有使用中的 key）</span
      >
    </h1>
    <p class="legend small">
      <span><i :style="{ background: statusColor('active') }"></i>仍有 key</span>
      <span><i :style="{ background: statusColor('revoked') }"></i>已停用 key</span>
      <span class="muted">調查跟著帳號：停用 key 後答案仍保留，刪除帳號時才會刪除</span>
    </p>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>

    <div class="groups">
      <section v-for="g in groups" :key="g.title" class="card block">
        <h2>{{ g.title }}</h2>
        <ul class="bars">
          <li v-for="r in g.rows" :key="r.value">
            <span class="label">{{ g.labels[r.value] ?? r.value }}</span>
            <span class="bar" :title="`仍有 key ${r.active}・已停用 ${r.count - r.active}`">
              <i :style="{ width: `${pct(r.active)}%`, background: statusColor('active') }"></i>
              <i
                v-if="r.count > r.active"
                :style="{
                  width: `${pct(r.count - r.active)}%`,
                  background: statusColor('revoked'),
                }"
              ></i>
            </span>
            <span class="num"
              >{{ r.count }} 人（{{ pct(r.count) }}%）<br /><span class="muted small"
                >仍有 key {{ r.active }}</span
              ></span
            >
          </li>
          <li v-if="g.rows.length === 0" class="muted">還沒有人填寫</li>
        </ul>
      </section>
      <template v-if="!data && !error">
        <section v-for="i in 3" :key="i" class="card block">
          <span class="skeleton cell"></span>
          <span class="skeleton" style="display: block; height: 6rem"></span>
        </section>
      </template>
    </div>

    <section class="card block">
      <h2>應用與建議</h2>
      <table v-if="data">
        <thead>
          <tr>
            <th>日期</th>
            <th>key</th>
            <th>身分</th>
            <th>用途</th>
            <th>關注</th>
            <th>應用</th>
            <th>建議</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(n, i) in data.notes" :key="i">
            <td>{{ date(n.updated_at) }}</td>
            <td class="nowrap">
              <span
                class="mi status"
                :style="{ color: statusColor(n.active ? 'active' : 'revoked') }"
                aria-hidden="true"
                >{{ STATUS_ICONS[n.active ? 'active' : 'revoked'] }}</span
              >
              {{ n.active ? '仍有 key' : STATUS_LABELS.revoked }}
            </td>
            <td>{{ IDENTITIES[n.identity] ?? n.identity }}</td>
            <td>{{ PURPOSES[n.purpose] ?? n.purpose }}</td>
            <td>{{ topicNames(n.topics) }}</td>
            <td>
              {{ n.app_name || '—' }}
              <a
                v-if="n.app_url"
                class="sub"
                :href="n.app_url"
                target="_blank"
                rel="noopener nofollow"
                >{{ n.app_url }}</a
              >
            </td>
            <td class="suggestion">{{ n.suggestion || '—' }}</td>
          </tr>
          <tr v-if="data.notes.length === 0">
            <td colspan="7" class="muted">還沒有留言</td>
          </tr>
        </tbody>
      </table>
      <span v-else class="skeleton" style="display: block; height: 6rem"></span>
    </section>
  </div>
</template>

<style scoped>
.groups {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--font-m);
}
.bars {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 0.875rem;
}
.bars li {
  display: grid;
  grid-template-columns: 7.5rem 1fr 5.5rem;
  align-items: center;
  gap: 8px;
}
.bar {
  display: flex;
  gap: 2px;
  height: 0.6rem;
  border-radius: 3px;
  background: var(--surface-muted);
  overflow: hidden;
}
.bar i {
  display: block;
  height: 100%;
}
.legend {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem 1.25rem;
  margin: calc(-1 * var(--font-m) / 2) 0 var(--font-m);
}
.legend span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.legend i {
  width: 0.75rem;
  height: 0.75rem;
  border-radius: 3px;
}
.small {
  font-size: var(--font-s);
}
.nowrap {
  white-space: nowrap;
}
.status {
  font-size: 1rem;
  vertical-align: -3px;
}
.num {
  color: var(--text-muted);
  text-align: right;
  font-variant-numeric: tabular-nums;
}
.suggestion {
  min-width: 16rem;
  white-space: pre-wrap;
}
</style>
