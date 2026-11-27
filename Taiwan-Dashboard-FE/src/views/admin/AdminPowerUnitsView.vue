<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getPowerUnits, updatePowerUnit, type PowerUnitRef } from '../../api/admin'
import { ApiError, getCounties } from '../../api/client'
import type { County } from '../../api/types'
import { usePageTitle } from '../../composables/usePageTitle'
import { formatNumber } from '../../lib/format'
import { rememberCount, rememberedCount } from '../../lib/rememberCount'

usePageTitle('發電機組')

const FUEL: Record<string, string> = {
  nuclear: '核能',
  coal: '燃煤',
  cogen: '汽電共生',
  gas: '燃氣',
  oil: '燃油',
  hydro: '水力',
  wind: '風力',
  solar: '太陽能',
  other_renewable: '其他再生能源',
  storage: '儲能',
  storage_load: '儲能負載',
}
const fuelName = (f: string) => FUEL[f] ?? f

const COLUMNS = [
  { label: '電廠' },
  { label: '燃料' },
  { label: '機組', num: true },
  { label: '目前發電（MW）', num: true },
  { label: '縣市' },
]
const skeletonRows = rememberedCount('admin-power-plants', 20)

const units = ref<PowerUnitRef[] | null>(null)
const counties = ref<County[]>([])
const error = ref('')
const onlyMissing = ref(true)
const fuel = ref('')
const q = ref('')
const expanded = ref<Record<string, boolean>>({})
const busy = ref<Record<string, boolean>>({})
const messages = ref<Record<string, { ok: boolean; text: string }>>({})

interface Plant {
  key: string
  name: string
  fuels: string[]
  units: PowerUnitRef[]
  county: string | null | undefined
  mw: number
}

async function load() {
  try {
    units.value = await getPowerUnits()
    error.value = ''
  } catch {
    error.value = '發電機組資料暫時無法取得'
  }
}

onMounted(() => {
  void load().then(() => rememberCount('admin-power-plants', Math.min(plants.value.length, 60)))
  void getCounties()
    .then((c) => (counties.value = c))
    .catch(() => {})
})

const plants = computed<Plant[]>(() => {
  const by = new Map<string, Plant>()
  for (const u of units.value ?? []) {
    const name = u.plant_name ?? u.unit_name
    let p = by.get(name)
    if (!p) {
      p = { key: name, name, fuels: [], units: [], county: u.county_code, mw: 0 }
      by.set(name, p)
    }
    p.units.push(u)
    if (!p.fuels.includes(u.fuel_type)) p.fuels.push(u.fuel_type)
    if (p.county !== u.county_code) p.county = undefined
    p.mw += u.net_gen_mw ?? 0
  }
  return [...by.values()]
})

const fuels = computed(() => [...new Set(plants.value.flatMap((p) => p.fuels))])
const missing = computed(() => plants.value.filter((p) => !p.county).length)

const shown = computed(() =>
  plants.value.filter((p) => {
    if (onlyMissing.value && p.county) return false
    if (fuel.value && !p.fuels.includes(fuel.value)) return false
    const s = q.value.trim()
    return !s || p.name.includes(s) || p.units.some((u) => u.unit_name.includes(s))
  }),
)

async function setPlant(p: Plant, county: string | null) {
  busy.value[p.key] = true
  delete messages.value[p.key]
  let failed = 0
  let lastError = ''
  for (const u of p.units) {
    if (u.county_code === county) continue
    try {
      await updatePowerUnit(u.id, county)
    } catch (e) {
      failed++
      lastError = e instanceof ApiError ? e.message : '儲存失敗'
    }
  }
  await load()
  busy.value[p.key] = false
  messages.value[p.key] = failed
    ? { ok: false, text: `${failed} 部機組沒有儲存：${lastError}` }
    : { ok: true, text: '已儲存' }
}

async function setUnit(p: Plant, u: PowerUnitRef, county: string | null) {
  busy.value[p.key] = true
  delete messages.value[p.key]
  try {
    await updatePowerUnit(u.id, county)
    await load()
    messages.value[p.key] = { ok: true, text: '已儲存' }
  } catch (e) {
    messages.value[p.key] = { ok: false, text: e instanceof ApiError ? e.message : '儲存失敗' }
  } finally {
    busy.value[p.key] = false
  }
}

const value = (e: Event) => (e.target as HTMLSelectElement).value || null
</script>

<template>
  <div>
    <div class="page-title">
      <h1><span class="mi" aria-hidden="true">bolt</span>發電機組</h1>
      <span v-if="units" class="tools" :class="missing ? 'bad' : 'ok'">
        {{ missing ? `${missing} 座電廠還沒有縣市` : '每座電廠都有縣市了' }}
      </span>
    </div>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>

    <section class="card block">
      <div class="filters">
        <label class="check"><input v-model="onlyMissing" type="checkbox" />只顯示缺縣市</label>
        <select v-model="fuel" class="select" aria-label="篩選燃料">
          <option value="">全部燃料</option>
          <option v-for="f in fuels" :key="f" :value="f">{{ fuelName(f) }}</option>
        </select>
        <input
          v-model="q"
          class="input search"
          type="search"
          placeholder="搜尋電廠或機組"
          aria-label="搜尋電廠或機組"
        />
      </div>
      <table>
        <thead>
          <tr>
            <th v-for="c in COLUMNS" :key="c.label" :class="{ num: c.num }">{{ c.label }}</th>
          </tr>
        </thead>
        <tbody v-if="units">
          <template v-for="p in shown" :key="p.key">
            <tr>
              <td>
                <button
                  type="button"
                  class="toggle"
                  :aria-expanded="!!expanded[p.key]"
                  @click="expanded[p.key] = !expanded[p.key]"
                >
                  <span class="mi" aria-hidden="true">{{
                    expanded[p.key] ? 'expand_more' : 'chevron_right'
                  }}</span>
                  {{ p.name }}
                </button>
              </td>
              <td>{{ p.fuels.map(fuelName).join('、') }}</td>
              <td class="num">{{ p.units.length }}</td>
              <td class="num">{{ formatNumber(p.mw, 1) }}</td>
              <td>
                <select
                  class="select"
                  :class="{ bad: p.county === null }"
                  :value="p.county ?? ''"
                  :disabled="busy[p.key]"
                  :aria-label="`${p.name}的縣市`"
                  @change="setPlant(p, value($event))"
                >
                  <option v-if="p.county === undefined" value="" disabled>各機組不同</option>
                  <option v-else value="">未設定</option>
                  <option v-for="c in counties" :key="c.code" :value="c.code">{{ c.name }}</option>
                </select>
                <span
                  v-if="messages[p.key]"
                  class="msg"
                  :class="messages[p.key]!.ok ? 'ok' : 'bad'"
                  role="status"
                  >{{ messages[p.key]!.text }}</span
                >
              </td>
            </tr>
            <template v-if="expanded[p.key]">
              <tr v-for="u in p.units" :key="u.id" class="unit">
                <td>{{ u.unit_name }}<span v-if="u.is_private" class="muted">（民營）</span></td>
                <td class="muted">{{ fuelName(u.fuel_type) }}</td>
                <td></td>
                <td class="num">{{ formatNumber(u.net_gen_mw, 1) }}</td>
                <td>
                  <select
                    class="select"
                    :value="u.county_code ?? ''"
                    :disabled="busy[p.key]"
                    :aria-label="`${u.unit_name}的縣市`"
                    @change="setUnit(p, u, value($event))"
                  >
                    <option value="">未設定</option>
                    <option v-for="c in counties" :key="c.code" :value="c.code">
                      {{ c.name }}
                    </option>
                  </select>
                </td>
              </tr>
            </template>
          </template>
          <tr v-if="shown.length === 0">
            <td :colspan="COLUMNS.length" class="muted">沒有符合的電廠</td>
          </tr>
        </tbody>
        <tbody v-else>
          <tr v-for="i in skeletonRows" :key="i">
            <td v-for="c in COLUMNS" :key="c.label"><span class="skeleton cell"></span></td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<style scoped>
.search {
  margin-left: auto;
  width: 12rem;
}
.toggle {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 0;
  border: 0;
  background: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
}
.toggle .mi {
  font-size: 1.1rem;
  color: var(--text-muted);
}
.unit td:first-child {
  padding-left: 2rem;
  color: var(--text-secondary);
}
.msg {
  margin-left: 0.5rem;
  font-size: var(--font-s);
}
.select.bad {
  border-color: var(--danger-text);
}
</style>
