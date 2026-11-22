<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getTyphoons, type TyphoonEvent } from '../api/client'
import { usePageTitle } from '../composables/usePageTitle'
import { typhoonName, typhoonPeriod } from '../lib/typhoon'

usePageTitle('歷次颱風')

const list = ref<TyphoonEvent[] | null>(null)
const error = ref(false)
onMounted(async () => {
  try {
    list.value = await getTyphoons()
  } catch {
    error.value = true
  }
})
</script>

<template>
  <div class="page">
    <h1 class="page-title"><span class="mi" aria-hidden="true">history</span>歷次颱風</h1>
    <p class="lead muted">
      每次颱風警報都會建立專區，記錄路徑、警報、停班停課、雨量與停電。 本站自 2026 年 10
      月開始記錄。
    </p>
    <p v-if="error" class="notice">颱風清單載入失敗</p>
    <p v-else-if="list && list.length === 0" class="muted">尚無颱風紀錄。</p>
    <ul v-else class="typhoons">
      <template v-if="list">
        <li v-for="t in list" :key="t.id">
          <RouterLink :to="`/typhoon/${t.id}`" class="card">
            <span class="mi" aria-hidden="true">cyclone</span>
            <span class="body">
              <strong>
                {{ typhoonName(t) }}
                <small v-if="t.name_en" class="muted">{{ t.name_en }}</small>
                <em v-if="t.active" class="active">警報中</em>
              </strong>
              <span class="muted"
                >{{ typhoonPeriod(t) }}・{{ t.last_alert }}・共 {{ t.warnings }} 報</span
              >
            </span>
            <span class="mi" aria-hidden="true">chevron_right</span>
          </RouterLink>
        </li>
      </template>
      <li v-for="i in list ? 0 : 3" :key="`s${i}`" class="skeleton row-skeleton"></li>
    </ul>
  </div>
</template>

<style scoped>
.lead {
  max-width: 60rem;
  margin: 0 0 16px;
  line-height: 1.7;
}
.typhoons {
  display: grid;
  gap: 8px;
  max-width: 48rem;
  margin: 0;
  padding: 0;
  list-style: none;
}
.card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  color: inherit;
  text-decoration: none;
}
.card:hover {
  border-color: var(--border-strong);
}
.body {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 2px;
}
.body small {
  margin-left: 4px;
  font-weight: 400;
}
.active {
  margin-left: 8px;
  padding: 1px 8px;
  border-radius: 999px;
  background: #d8001d;
  color: #ffffff;
  font-size: var(--font-s);
  font-style: normal;
  font-weight: 500;
}
.row-skeleton {
  height: 64px;
  border-radius: 8px;
}
</style>
