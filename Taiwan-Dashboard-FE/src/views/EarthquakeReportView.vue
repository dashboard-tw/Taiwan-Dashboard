<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { ApiError, getQuake, type QuakeDetail } from '../api/client'
import QuakeStationMap from '../components/QuakeStationMap.vue'
import { usePageTitle } from '../composables/usePageTitle'
import { formatNumber } from '../lib/format'
import { intensityLevel, QUAKE_KINDS, quakePlace, quakeTime } from '../lib/quake'

const props = defineProps<{ id: string }>()

const quake = ref<QuakeDetail | null>(null)
const error = ref('')
const imageFailed = ref(false)
const shakemapFailed = ref(false)
const openWave = ref<string | null>(null)
const showAllStations = ref(false)
const FIRST_STATIONS = 20

const stations = computed(() => quake.value?.stations ?? [])
const visibleStations = computed(() =>
  showAllStations.value ? stations.value : stations.value.slice(0, FIRST_STATIONS),
)
const hasStationMap = computed(() => stations.value.some((s) => s.lon !== null))

function toggleWave(id: string) {
  openWave.value = openWave.value === id ? null : id
}

function selectStation(id: string) {
  const i = stations.value.findIndex((s) => s.id === id)
  if (i >= FIRST_STATIONS) showAllStations.value = true
  openWave.value = null
  requestAnimationFrame(() =>
    document.getElementById(`station-${id}`)?.scrollIntoView({ block: 'nearest' }),
  )
}

watch(
  () => props.id,
  async (id) => {
    quake.value = null
    error.value = ''
    imageFailed.value = false
    shakemapFailed.value = false
    openWave.value = null
    showAllStations.value = false
    try {
      quake.value = await getQuake(id)
    } catch (e) {
      error.value =
        e instanceof ApiError && e.status === 404
          ? '找不到這份地震報告。'
          : '地震報告暫時無法取得。'
    }
  },
  { immediate: true },
)

const title = computed(() =>
  quake.value
    ? `${quakePlace(quake.value.location)} 規模 ${quake.value.magnitude ?? '—'} 地震`
    : '',
)
usePageTitle(computed(() => title.value || '地震報告'))

function num(v: number | null, d = 1) {
  return v === null ? '—' : formatNumber(v, d)
}
</script>

<template>
  <div class="page quake-page">
    <nav class="crumbs" aria-label="位置">
      <RouterLink to="/earthquake">地震</RouterLink>
      <span class="mi" aria-hidden="true">chevron_right</span>
      <span>地震報告</span>
    </nav>

    <p v-if="error" class="notice">
      {{ error }}<RouterLink to="/earthquake">回到地震頁</RouterLink>
    </p>
    <div v-else-if="!quake" class="card block">
      <span class="skeleton" style="display: block; height: 2rem; width: 60%"></span>
      <span class="skeleton" style="display: block; height: 16rem; margin-top: 1rem"></span>
    </div>

    <template v-else>
      <h1 class="page-title"><span class="mi" aria-hidden="true">vibration</span>{{ title }}</h1>

      <div class="layout">
        <div class="col">
          <section class="card block">
            <p v-if="quake.content" class="content">{{ quake.content }}</p>
            <dl class="facts">
              <div>
                <dt>發震時間</dt>
                <dd>{{ quakeTime(quake.origin_time) }}</dd>
              </div>
              <div>
                <dt>報告種類</dt>
                <dd>
                  {{ QUAKE_KINDS[quake.kind] ?? quake.kind
                  }}<template v-if="quake.no && quake.kind === 'significant'">
                    第 {{ quake.no }} 號</template
                  >
                </dd>
              </div>
              <div>
                <dt>規模</dt>
                <dd>{{ num(quake.magnitude) }}</dd>
              </div>
              <div>
                <dt>深度</dt>
                <dd>{{ num(quake.depth_km) }} 公里</dd>
              </div>
              <div class="wide">
                <dt>震央</dt>
                <dd>
                  {{ quake.location ?? '—' }}
                  <span v-if="quake.lon !== null && quake.lat !== null" class="muted coord">
                    北緯 {{ num(quake.lat, 2) }}、東經 {{ num(quake.lon, 2) }}
                  </span>
                </dd>
              </div>
            </dl>

            <h2>各縣市最大震度</h2>
            <p v-if="!quake.intensities.length" class="muted">這份報告沒有各縣市震度。</p>
            <ul v-else class="intensities">
              <li v-for="i in quake.intensities" :key="i.county_code">
                <RouterLink :to="`/earthquake/${i.county_code}`">{{ i.county }}</RouterLink>
                <span
                  class="badge"
                  :style="
                    intensityLevel(i.intensity)
                      ? {
                          background: intensityLevel(i.intensity)!.color,
                          color: intensityLevel(i.intensity)!.ink,
                        }
                      : undefined
                  "
                  >{{ i.intensity }}</span
                >
              </li>
            </ul>

            <p class="source muted">
              <template v-if="quake.remark">{{ quake.remark }}</template>
              資料來源：交通部中央氣象署。報告為速報結果，之後可能修正。
              <a v-if="quake.report_url" :href="quake.report_url" target="_blank" rel="noopener"
                >查看氣象署的原始報告</a
              >
            </p>
          </section>
          <section v-if="hasStationMap" class="card block map-card">
            <h2>各測站震度</h2>
            <QuakeStationMap
              :stations="stations"
              :lon="quake.lon"
              :lat="quake.lat"
              @select="selectStation"
            />
            <p class="muted small-note">紅色 ✕ 是震央。點圓點可以看測站的數值。</p>
          </section>
        </div>
        <div class="col">
          <section class="card block figure">
            <h2>報告圖</h2>
            <a
              v-if="quake.image_url && !imageFailed"
              :href="quake.image_url"
              target="_blank"
              rel="noopener"
              class="image-link"
            >
              <img
                :src="quake.image_url"
                :alt="`中央氣象署地震報告圖：${quake.content ?? title}`"
                loading="lazy"
                referrerpolicy="no-referrer"
                @error="imageFailed = true"
              />
            </a>
            <p v-else class="muted">
              報告圖暫時無法顯示。<a
                v-if="quake.report_url"
                :href="quake.report_url"
                target="_blank"
                rel="noopener"
                >到氣象署查看</a
              >
            </p>
          </section>
          <section v-if="quake.shakemap_url && !shakemapFailed" class="card block figure">
            <h2>震度分布圖</h2>
            <a :href="quake.shakemap_url" target="_blank" rel="noopener" class="image-link">
              <img
                :src="quake.shakemap_url"
                :alt="`中央氣象署震度分布圖：${title}`"
                loading="lazy"
                referrerpolicy="no-referrer"
                @error="shakemapFailed = true"
              />
            </a>
          </section>
        </div>
      </div>

      <section v-if="stations.length" class="card block">
        <h2>測站數值（{{ stations.length }} 站）</h2>
        <p class="muted small-note">
          加速度單位是 gal（公分/秒²），速度單位是
          kine（公分/秒），都是用來計算震度的值。點「波形」可以看該測站記錄到的震波。
        </p>
        <div class="table-wrap">
          <table class="stations">
            <thead>
              <tr>
                <th scope="col">測站</th>
                <th scope="col">縣市</th>
                <th scope="col">震度</th>
                <th scope="col" class="n">最大加速度</th>
                <th scope="col" class="n">最大速度</th>
                <th scope="col" class="n">離震央</th>
                <th scope="col"><span class="visually-hidden">波形</span></th>
              </tr>
            </thead>
            <tbody>
              <template v-for="st in visibleStations" :key="st.id">
                <tr :id="`station-${st.id}`">
                  <th scope="row">{{ st.name }}</th>
                  <td>{{ st.county ?? '—' }}</td>
                  <td>
                    <span
                      class="badge"
                      :style="
                        intensityLevel(st.intensity)
                          ? {
                              background: intensityLevel(st.intensity)!.color,
                              color: intensityLevel(st.intensity)!.ink,
                            }
                          : undefined
                      "
                      >{{ st.intensity ?? '—' }}</span
                    >
                  </td>
                  <td class="n">{{ num(st.pga_gal, 1) }}</td>
                  <td class="n">{{ num(st.pgv_kine, 2) }}</td>
                  <td class="n">{{ num(st.distance_km, 1) }} 公里</td>
                  <td>
                    <button
                      v-if="st.wave_url"
                      type="button"
                      class="wave-btn"
                      :aria-expanded="openWave === st.id"
                      @click="toggleWave(st.id)"
                    >
                      波形
                    </button>
                  </td>
                </tr>
                <tr v-if="openWave === st.id && st.wave_url" class="wave-row">
                  <td colspan="7">
                    <img
                      :src="st.wave_url"
                      :alt="`${st.name}測站的強地動波形圖`"
                      referrerpolicy="no-referrer"
                    />
                    <p class="muted small-note">
                      東西向 {{ num(st.pga_ew_gal, 1) }}、南北向 {{ num(st.pga_ns_gal, 1) }}、垂直
                      {{ num(st.pga_v_gal, 1) }} gal
                    </p>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
        <button
          v-if="stations.length > FIRST_STATIONS"
          type="button"
          class="more"
          @click="showAllStations = !showAllStations"
        >
          {{ showAllStations ? '收合' : `顯示全部 ${stations.length} 站` }}
        </button>
      </section>
    </template>
  </div>
</template>

<style scoped>
.crumbs {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 8px;
  color: var(--text-muted);
  font-size: 0.875rem;
}
.crumbs .mi {
  font-size: 1rem;
}
.layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--font-m);
  align-items: stretch;
}
/* 左右兩欄各自往下排，測站地圖填滿左欄剩下的高度 */
.col {
  display: flex;
  flex-direction: column;
  gap: var(--font-m);
  min-width: 0;
}
.map-card {
  flex: 1;
}
.map-card > .quake-map {
  flex: 1;
  min-height: 22rem;
}
.block {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: var(--font-m);
  line-height: 1.7;
}
.content {
  font-size: 1.05rem;
}
h2 {
  margin-top: 6px;
  font-size: var(--font-ms);
}
.facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 24px;
  margin: 0;
}
.facts .wide {
  grid-column: 1 / -1;
}
.facts dt {
  color: var(--text-muted);
  font-size: var(--font-s);
}
.facts dd {
  margin: 2px 0 0;
}
.coord {
  display: block;
  font-size: var(--font-s);
}
.intensities {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(9rem, 1fr));
  gap: 8px 16px;
  margin: 0;
  padding: 0;
  list-style: none;
}
.intensities li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 6px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
}
.intensities a {
  color: var(--text-primary);
  text-decoration: none;
}
.intensities a:hover {
  text-decoration: underline;
}
.badge {
  padding: 1px 8px;
  border-radius: 999px;
  background: var(--surface-muted);
  font-size: 0.8rem;
  white-space: nowrap;
}
.source {
  font-size: var(--font-s);
}
.image-link {
  display: block;
}
.figure img {
  display: block;
  width: 100%;
  height: auto;
  border-radius: var(--radius);
  background: #ffffff;
}
/* 測站表與上面的間距 */
.layout + .block {
  margin-top: var(--font-m);
}
.small-note {
  font-size: var(--font-s);
}
.table-wrap {
  overflow-x: auto;
}
.stations {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
  font-variant-numeric: tabular-nums;
}
.stations th,
.stations td {
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
  text-align: left;
  white-space: nowrap;
}
.stations thead th {
  color: var(--text-muted);
  font-size: var(--font-s);
  font-weight: 400;
}
.stations tbody th {
  font-weight: 500;
}
.stations .n {
  text-align: right;
}
.wave-btn,
.more {
  padding: 2px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: transparent;
  color: var(--text-secondary);
  font: inherit;
  font-size: 0.85rem;
  cursor: pointer;
}
.wave-btn:hover,
.more:hover {
  color: var(--text-primary);
}
.wave-btn[aria-expanded='true'] {
  background: var(--surface-muted);
}
.more {
  align-self: flex-start;
}
.wave-row td {
  white-space: normal;
}
.wave-row img {
  display: block;
  max-width: 100%;
  height: auto;
  background: #ffffff;
  border-radius: var(--radius);
}
@media (max-width: 62.4375em) {
  .layout {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
