<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  getReservoirs,
  updateReservoir,
  type ReservoirInput,
  type ReservoirRef,
} from '../../api/admin'
import { ApiError, getCounties, getCountiesGeo, type CountyCollection } from '../../api/client'
import type { County } from '../../api/types'
import PointPicker from '../../components/admin/PointPicker.vue'
import { usePageTitle } from '../../composables/usePageTitle'
import { formatNumber } from '../../lib/format'
import { countyAt } from '../../lib/geo'
import { rememberCount, rememberedCount } from '../../lib/rememberCount'

usePageTitle('水庫主檔')

const COLUMNS = [
  { label: '代碼' },
  { label: '名稱' },
  { label: '縣市' },
  { label: '有效容量（萬 m³）', num: true },
  { label: '蓄水率', num: true },
  { label: '座標' },
]
const skeletonRows = rememberedCount('admin-reservoirs', 20)

const list = ref<ReservoirRef[] | null>(null)
const counties = ref<County[]>([])
const geo = ref<CountyCollection | null>(null)
const error = ref('')
const filter = ref<'all' | 'no-county' | 'no-point' | 'odd'>('all')
const q = ref('')
const selectedId = ref<string | null>(null)
const form = ref<ReservoirInput | null>(null)
const saving = ref(false)
const message = ref<{ ok: boolean; text: string } | null>(null)

async function load() {
  try {
    list.value = await getReservoirs()
    rememberCount('admin-reservoirs', list.value.length)
    error.value = ''
  } catch {
    error.value = '水庫主檔暫時無法取得'
  }
}

onMounted(async () => {
  void load()
  counties.value = await getCounties().catch(() => [])
  geo.value = await getCountiesGeo().catch(() => null)
})

const countyName = (code: string | null) =>
  code ? (counties.value.find((c) => c.code === code)?.name ?? code) : null

const OVERFULL = 110
const isOdd = (r: ReservoirRef) =>
  (r.effective_capacity === null && r.has_storage) ||
  (r.storage_pct !== null && r.storage_pct > OVERFULL)

const counts = computed(() => {
  const l = list.value ?? []
  return {
    noCounty: l.filter((r) => !r.county_code).length,
    noPoint: l.filter((r) => r.lon === null).length,
    odd: l.filter(isOdd).length,
  }
})

const shown = computed(() =>
  (list.value ?? []).filter((r) => {
    if (filter.value === 'no-county' && r.county_code) return false
    if (filter.value === 'no-point' && r.lon !== null) return false
    if (filter.value === 'odd' && !isOdd(r)) return false
    const s = q.value.trim()
    return !s || r.name.includes(s) || r.id.includes(s)
  }),
)

const selected = computed(() => list.value?.find((r) => r.id === selectedId.value) ?? null)

function select(r: ReservoirRef) {
  selectedId.value = r.id
  message.value = null
  form.value = {
    name: r.name,
    county_code: r.county_code,
    effective_capacity: r.effective_capacity,
    capacity_locked: r.capacity_locked,
    is_major: r.is_major,
    lon: r.lon,
    lat: r.lat,
  }
}

const others = computed(() =>
  (list.value ?? [])
    .filter((r) => r.id !== selectedId.value && r.lon !== null && r.lat !== null)
    .map((r) => ({ lon: r.lon!, lat: r.lat!, name: r.name })),
)

const suggested = computed(() => {
  const f = form.value
  if (!f || f.lon === null || f.lat === null || !geo.value) return null
  return countyAt(geo.value, f.lon, f.lat)?.properties ?? null
})

function pick(lon: number, lat: number) {
  if (!form.value) return
  form.value.lon = lon
  form.value.lat = lat
}

const dirty = computed(() => {
  const r = selected.value
  const f = form.value
  if (!r || !f) return false
  return (
    f.name !== r.name ||
    f.county_code !== r.county_code ||
    f.effective_capacity !== r.effective_capacity ||
    f.capacity_locked !== r.capacity_locked ||
    f.is_major !== r.is_major ||
    f.lon !== r.lon ||
    f.lat !== r.lat
  )
})

const previewPct = computed(() => {
  const r = selected.value
  const f = form.value
  if (!r || !f || r.storage_pct === null || !r.effective_capacity || !f.effective_capacity)
    return null
  if (f.effective_capacity === r.effective_capacity) return null
  return (r.storage_pct * r.effective_capacity) / f.effective_capacity
})

async function save() {
  if (!form.value || !selectedId.value) return
  saving.value = true
  message.value = null
  try {
    const f = form.value
    await updateReservoir(selectedId.value, {
      ...f,
      capacity_locked: f.effective_capacity === null ? false : f.capacity_locked,
    })
    await load()
    if (selected.value) select(selected.value)
    message.value = { ok: true, text: '已儲存，前台的水庫資料會在幾秒內更新' }
  } catch (e) {
    message.value = { ok: false, text: e instanceof ApiError ? e.message : '儲存失敗，請稍後再試' }
  } finally {
    saving.value = false
  }
}

function setCapacity(v: string) {
  if (!form.value) return
  const n = Number(v)
  form.value.effective_capacity = v.trim() === '' || !Number.isFinite(n) ? null : n
  form.value.capacity_locked = form.value.effective_capacity !== null
}

function clearPoint() {
  if (!form.value) return
  form.value.lon = null
  form.value.lat = null
}
</script>

<template>
  <div>
    <h1 class="page-title">
      <span class="mi" aria-hidden="true">water_drop</span>水庫主檔
      <span class="tools muted">點選一座水庫來編輯</span>
    </h1>
    <p v-if="error" class="notice bad" role="alert">{{ error }}</p>

    <div class="layout">
      <section class="card block list">
        <div class="filters">
          <label class="check"><input v-model="filter" type="radio" value="all" />全部</label>
          <label class="check">
            <input v-model="filter" type="radio" value="no-county" />缺縣市（{{ counts.noCounty }}）
          </label>
          <label class="check">
            <input v-model="filter" type="radio" value="no-point" />缺座標（{{ counts.noPoint }}）
          </label>
          <label class="check">
            <input v-model="filter" type="radio" value="odd" />容量待確認（{{ counts.odd }}）
          </label>
          <input
            v-model="q"
            class="input search"
            type="search"
            placeholder="搜尋名稱或代碼"
            aria-label="搜尋水庫"
          />
        </div>
        <p v-if="filter === 'odd'" class="muted hint">
          列出有回報蓄水量卻沒有有效容量，或蓄水率超過 {{ OVERFULL }}%
          的水庫。攔河堰只回報水位，不需要有效容量。 水位略高於滿水位時，蓄水率稍微超過 100%
          是正常的。
        </p>
        <table>
          <thead>
            <tr>
              <th v-for="c in COLUMNS" :key="c.label" :class="{ num: c.num }">{{ c.label }}</th>
            </tr>
          </thead>
          <tbody v-if="list">
            <tr
              v-for="r in shown"
              :key="r.id"
              class="clickable"
              :class="{ active: r.id === selectedId }"
              tabindex="0"
              @click="select(r)"
              @keydown.enter="select(r)"
            >
              <td class="muted">{{ r.id }}</td>
              <td>{{ r.name }}<span v-if="r.is_major" class="badge major">主要</span></td>
              <td :class="{ bad: !r.county_code }">{{ countyName(r.county_code) ?? '未設定' }}</td>
              <td class="num">
                {{ r.effective_capacity === null ? '—' : formatNumber(r.effective_capacity, 0) }}
                <span
                  v-if="r.capacity_locked"
                  class="mi lock"
                  title="已鎖定，不會被資料管線覆寫"
                  aria-hidden="true"
                  >lock</span
                >
              </td>
              <td class="num" :class="{ bad: r.storage_pct !== null && r.storage_pct > OVERFULL }">
                {{ r.storage_pct === null ? '—' : `${formatNumber(r.storage_pct, 1)}%` }}
              </td>
              <td :class="r.lon === null ? 'bad' : 'muted'">
                {{ r.lon === null ? '未標註' : '已標註' }}
              </td>
            </tr>
            <tr v-if="shown.length === 0">
              <td :colspan="COLUMNS.length" class="muted">沒有符合的水庫</td>
            </tr>
          </tbody>
          <tbody v-else>
            <tr v-for="i in skeletonRows" :key="i">
              <td v-for="c in COLUMNS" :key="c.label"><span class="skeleton cell"></span></td>
            </tr>
          </tbody>
        </table>
      </section>

      <section class="card block editor">
        <template v-if="form && selected">
          <h2>
            {{ selected.name }}<span class="muted">{{ selected.id }}</span>
          </h2>
          <div class="grid">
            <label class="field">
              <span>名稱</span>
              <input v-model="form.name" class="input" maxlength="50" />
            </label>
            <label class="field">
              <span>縣市</span>
              <select v-model="form.county_code" class="select">
                <option :value="null">未設定</option>
                <option v-for="c in counties" :key="c.code" :value="c.code">{{ c.name }}</option>
              </select>
            </label>
            <label class="field">
              <span>有效容量（萬立方公尺）</span>
              <input
                class="input"
                inputmode="decimal"
                :value="form.effective_capacity ?? ''"
                @change="setCapacity(($event.target as HTMLInputElement).value)"
              />
            </label>
            <div class="field">
              <span aria-hidden="true">&nbsp;</span>
              <label class="check">
                <input
                  v-model="form.capacity_locked"
                  type="checkbox"
                  :disabled="form.effective_capacity === null"
                />鎖定，不讓每日的水庫主檔覆寫
              </label>
            </div>
          </div>
          <p v-if="previewPct !== null" class="muted small">
            以新容量計算，目前蓄水率會變成 {{ formatNumber(previewPct, 1) }}%
          </p>
          <label class="check">
            <input v-model="form.is_major" type="checkbox" />主要水庫（首頁與水情摘要會優先列出）
          </label>

          <div class="coords">
            <span class="muted">座標</span>
            <template v-if="form.lon !== null">
              <span>{{ form.lat?.toFixed(5) }}, {{ form.lon.toFixed(5) }}</span>
              <button type="button" class="btn" @click="clearPoint">清除</button>
            </template>
            <span v-else class="bad">尚未標註，在地圖上點選水庫的位置</span>
            <span v-if="suggested && suggested.code !== form.county_code" class="suggest">
              這個位置在{{ suggested.name }}
              <button type="button" class="btn" @click="form.county_code = suggested.code">
                設為縣市
              </button>
            </span>
          </div>
          <div class="map">
            <PointPicker :lon="form.lon" :lat="form.lat" :others="others" @pick="pick" />
          </div>

          <div class="actions">
            <span v-if="message" :class="message.ok ? 'ok' : 'bad'" role="status">{{
              message.text
            }}</span>
            <button
              type="button"
              class="btn"
              :disabled="!dirty || saving"
              @click="select(selected)"
            >
              還原
            </button>
            <button type="button" class="btn-highlight" :disabled="!dirty || saving" @click="save">
              <span class="mi" aria-hidden="true">save</span>{{ saving ? '儲存中' : '儲存' }}
            </button>
          </div>
        </template>
        <p v-else class="muted empty">
          從左邊的清單選一座水庫。缺縣市的水庫不會出現在縣市頁面；蓄水率超過 100%
          通常是有效容量過時，可以在這裡修正並鎖定。
        </p>
      </section>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--font-m);
  align-items: start;
}
.list {
  max-height: calc(100vh - var(--header-h) - 9rem);
  overflow-y: auto;
}
.list thead th {
  position: sticky;
  top: calc(-1 * var(--font-m));
  background: var(--surface);
}
.editor {
  position: sticky;
  top: calc(var(--header-h) + var(--font-m));
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.editor h2 {
  gap: 0.5rem;
  margin: 0;
}
.grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.75rem 1rem;
}
.search {
  margin-left: auto;
  width: 12rem;
}
.major {
  margin-left: 6px;
  color: var(--accent);
}
.lock {
  font-size: 0.9rem;
  vertical-align: -2px;
  color: var(--text-muted);
}
.coords {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  font-variant-numeric: tabular-nums;
}
.suggest {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  margin-left: auto;
  color: var(--warning-text);
}
.map {
  height: 360px;
}
.actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.5rem;
  font-size: 0.875rem;
}
.actions [role='status'] {
  margin-right: auto;
}
.small {
  font-size: var(--font-s);
}
.empty {
  padding: 2rem 0;
  font-size: 0.875rem;
  line-height: 1.7;
}
.hint {
  margin: -0.25rem 0 0.75rem;
  font-size: var(--font-s);
}
</style>
