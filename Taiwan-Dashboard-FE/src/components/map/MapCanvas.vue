<script setup lang="ts">
import * as maplibregl from 'maplibre-gl'
import type { GeoJSONSource, LngLatBoundsLike, Map as MapLibreMap } from 'maplibre-gl'
import { geoBounds } from 'd3-geo'
import { computed, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import {
  API_BASE,
  getComponentData,
  getCountiesGeo,
  layerUrl,
  type CountyCollection,
} from '../../api/client'
import type { ByCountyItem, ComponentInfo, MapLayerInfo } from '../../api/types'
import { applyBasemapTheme, basemapStyle, TAIWAN } from '../../lib/basemap'
import { formatNumber } from '../../lib/format'
import { rewind } from '../../lib/geo'
import { levelIn, levelsFor } from '../../lib/levels'
import { resolvedTheme } from '../../theme'
import { loadWindGrid, WindAnimation, type WindGrid } from '../../lib/windField'
import { onScheduledRefresh } from '../../composables/useRefreshClock'

export interface ActiveLayer {
  component: ComponentInfo
  layer: MapLayerInfo
}

const props = defineProps<{
  layers: ActiveLayer[]
  county: string | null
  basemap: 'emap' | 'none'
  insetBottom?: number
}>()

const emit = defineEmits<{
  select: [info: { title: string; rows: [string, string][] } | null]
}>()

const el = ref<HTMLDivElement | null>(null)
const map = shallowRef<MapLibreMap | null>(null)
const ready = ref(false)
const counties = shallowRef<CountyCollection | null>(null)
const mobile = window.matchMedia('(max-width: 47.4375em)')

function color(v: string): string {
  if (!v.startsWith('var(')) return v
  return (
    getComputedStyle(document.documentElement).getPropertyValue(v.slice(4, -1)).trim() || '#3b82f6'
  )
}

onMounted(async () => {
  const m = new maplibregl.Map({
    container: el.value!,
    style: basemapStyle(),
    bounds: TAIWAN,
    fitBoundsOptions: { padding: 20 },
    maxZoom: 17,
    attributionControl: { compact: true },
    dragRotate: false,
  })
  m.touchZoomRotate.disableRotation()
  m.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
  m.addControl(new maplibregl.GeolocateControl({ trackUserLocation: false }), 'top-right')
  map.value = m
  if (import.meta.env.DEV) (window as unknown as { __map: MapLibreMap }).__map = m
  m.on('load', () => {
    ready.value = true
  })
  m.on('click', onMapClick)
  resizeObserver = new ResizeObserver(() => {
    m.resize()
    if (!userMoved) focus()
  })
  resizeObserver.observe(el.value!)
  m.on('dragstart', () => (userMoved = true))
  m.on('zoomstart', (e) => {
    if ((e as { originalEvent?: unknown }).originalEvent) userMoved = true
  })
  try {
    counties.value = await getCountiesGeo()
  } catch {}
})

let resizeObserver: ResizeObserver | null = null
let userMoved = false
onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  wind?.destroy()
  map.value?.remove()
})

watch(resolvedTheme, () => {
  wind?.setTheme(resolvedTheme.value === 'dark')
  const m = map.value
  if (!m || !ready.value) return
  applyBasemapTheme(m)
  for (const id of [...added]) if (m.getLayer(id)?.type === 'fill') removeLayer(id)
  void sync()
})

watch(
  () => props.basemap,
  (b) => {
    if (map.value && ready.value)
      map.value.setLayoutProperty('emap', 'visibility', b === 'emap' ? 'visible' : 'none')
  },
)

const added = new Set<string>()
const handlers: Record<string, ActiveLayer> = {}
let popup: maplibregl.Popup | null = null

function layerId(a: ActiveLayer) {
  return `l-${a.component.slug}-${a.layer.id}`
}

async function countyCollection(
  a: ActiveLayer,
  force = false,
): Promise<GeoJSON.FeatureCollection | null> {
  if (!counties.value) return null
  const res = await getComponentData(a.component.slug, 'data', {}, force)
  if (res.meta.data_shape !== 'by_county') return null
  const values = new Map((res.data as ByCountyItem[]).map((d) => [d.county_code, d.data]))
  const levels = levelsFor(a.component.slug, a.component.topic, a.component.unit)
  const empty = color('var(--map-empty)')
  return {
    type: 'FeatureCollection',
    features: counties.value.features.map((f) => {
      const v = values.get(f.properties.code) ?? null
      return {
        type: 'Feature',
        geometry: f.geometry,
        properties: {
          ...f.properties,
          value: v,
          label:
            v === null ? '沒有資料' : `${formatNumber(v, a.component.decimals)}${a.component.unit}`,
          color: v !== null && levels ? color(levelIn(levels, v).color) : empty,
        },
      }
    }),
  }
}

async function addLayer(a: ActiveLayer) {
  const m = map.value!
  const id = layerId(a)
  if (a.layer.source_type === 'county_boundary') {
    const data = await countyCollection(a)
    if (!data || !props.layers.some((x) => layerId(x) === id)) return
    if (m.getSource(id)) (m.getSource(id) as GeoJSONSource).setData(data)
    else m.addSource(id, { type: 'geojson', data })
    if (!m.getLayer(id)) {
      m.addLayer({
        id,
        type: 'fill',
        source: id,
        paint: {
          'fill-color': ['get', 'color'],
          'fill-opacity': countyOpacity(),
          'fill-outline-color': resolvedTheme.value === 'dark' ? '#282a2c' : '#ffffff',
        },
      })
    }
  } else {
    const url = layerUrl(a.component.slug, a.layer.id, props.county)
    const style = (a.layer.style ?? {}) as {
      paint?: Record<string, unknown>
      layout?: Record<string, unknown>
      fit?: boolean
    }
    if (a.layer.layer_type === 'raster') {
      await addFrames(id, url, style.paint ?? {})
      return
    }
    if (a.layer.layer_type === 'wind') {
      await addWind(id, url)
      return
    }
    let data: string | GeoJSON.FeatureCollection = url
    const fc = await fetchLayer(url)
    if (!props.layers.some((x) => layerId(x) === id)) return
    if (fc) {
      data = fc
      if (style.fit) {
        const b = fc.features.length ? geoBounds(fc) : null
        if (b && b.flat().every(Number.isFinite)) extents.set(id, b as Extent)
      }
    }
    if (m.getSource(id)) (m.getSource(id) as GeoJSONSource).setData(data)
    else m.addSource(id, { type: 'geojson', data })
    if (extents.has(id) && !userMoved) focus()
    if (!m.getLayer(id)) {
      m.addLayer({
        id,
        type: a.layer.layer_type,
        source: id,
        paint: style.paint ?? {},
        layout: style.layout ?? {},
      } as maplibregl.LayerSpecification)
    }
  }
  if (!handlers[id]) {
    handlers[id] = a
    m.on('mouseenter', id, () => (m.getCanvas().style.cursor = 'pointer'))
    m.on('mouseleave', id, () => (m.getCanvas().style.cursor = ''))
  }
  added.add(id)
}

function removeLayer(id: string) {
  const m = map.value!
  if (m.getLayer(id)) m.removeLayer(id)
  if (m.getSource(id)) m.removeSource(id)
  delete handlers[id]
  added.delete(id)
  if (extents.delete(id) && !userMoved) focus()
  if (frames.value.has(id)) {
    const next = new Map(frames.value)
    next.delete(id)
    frames.value = next
    if (!next.size) stopFrames()
  }
  if (winds.value.has(id)) {
    const next = new Map(winds.value)
    next.delete(id)
    winds.value = next
  }
}

type Corners = [[number, number], [number, number], [number, number], [number, number]]
interface MapFrame {
  time: string
  url: string
  coordinates: Corners
}
const frames = shallowRef(new Map<string, MapFrame[]>())
const frameTimes = computed(() =>
  [...new Set([...frames.value.values()].flat().map((f) => f.time))].sort(),
)
const frameIndex = ref(0)
const framePlaying = ref(false)
let frameTimer: number | undefined

async function addFrames(id: string, url: string, paint: Record<string, unknown>) {
  const m = map.value!
  const fc = await fetchLayer(url)
  if (!props.layers.some((x) => layerId(x) === id)) return
  const list = framesOf(fc)
  added.add(id)
  const last = list.at(-1)
  if (!last) return
  if (!m.getSource(id))
    m.addSource(id, { type: 'image', url: last.url, coordinates: last.coordinates })
  if (!m.getLayer(id))
    m.addLayer({
      id,
      type: 'raster',
      source: id,
      paint: { 'raster-fade-duration': 0, ...paint },
    } as maplibregl.LayerSpecification)
  for (const f of list) new Image().src = f.url
  frames.value = new Map(frames.value).set(id, list)
  if (!framePlaying.value) frameIndex.value = frameTimes.value.length - 1
}

const winds = shallowRef(new Map<string, MapFrame[]>())
const windShown = ref<MapFrame | null>(null)
const windGrids = new Map<string, Promise<WindGrid>>()
let wind: WindAnimation | null = null

async function addWind(id: string, url: string) {
  const fc = await fetchLayer(url)
  if (!props.layers.some((x) => layerId(x) === id)) return
  added.add(id)
  winds.value = new Map(winds.value).set(id, framesOf(fc))
}

function framesOf(fc: GeoJSON.FeatureCollection | null): MapFrame[] {
  return (fc?.features ?? [])
    .map((f) => {
      const p = (f.properties ?? {}) as Record<string, unknown>
      const [w, s, e, n] = [p.west, p.south, p.east, p.north].map(Number) as [
        number,
        number,
        number,
        number,
      ]
      return {
        time: new Date(String(p.time)).toISOString(),
        url: String(p.url),
        coordinates: [
          [w, n],
          [e, n],
          [e, s],
          [w, s],
        ] as Corners,
      }
    })
    .filter(
      (f) => f.url.startsWith(`${API_BASE}/images/`) && f.coordinates.flat().every(Number.isFinite),
    )
    .sort((x, y) => (x.time < y.time ? -1 : 1))
}

async function showWind() {
  const m = map.value
  const list = [...winds.value.values()][0] ?? []
  const at = frameTimes.value[frameIndex.value] ?? new Date().toISOString()
  const t = Date.parse(at)
  let best: MapFrame | null = null
  for (const f of list)
    if (!best || Math.abs(Date.parse(f.time) - t) < Math.abs(Date.parse(best.time) - t)) best = f
  if (!m || !best) {
    windShown.value = null
    wind?.setGrid(null)
    return
  }
  if (windShown.value?.url === best.url) return
  windShown.value = best
  const f = best
  let grid = windGrids.get(f.url)
  if (!grid) {
    const [[w, n], [e], , [, s]] = f.coordinates
    grid = loadWindGrid(f.url, { west: w, south: s, east: e, north: n })
    windGrids.set(f.url, grid)
  }
  try {
    const g = await grid
    if (windShown.value?.url !== f.url) return
    if (!wind) {
      wind = new WindAnimation(m)
      wind.setTheme(resolvedTheme.value === 'dark')
    }
    wind.setGrid(g)
  } catch {
    windGrids.delete(f.url)
    windShown.value = null
  }
}
watch([winds, frameIndex, frameTimes], () => void showWind())

const windLabel = computed(() => {
  const f = windShown.value
  if (!f) return ''
  const t = new Date(f.time).toLocaleString('zh-TW', {
    timeZone: 'Asia/Taipei',
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
  return `風場：${t}${Date.parse(f.time) > Date.now() ? '（預報）' : ''}`
})

function showFrame() {
  const m = map.value
  const t = frameTimes.value[frameIndex.value]
  if (!m || !t) return
  for (const [id, list] of frames.value) {
    const f = [...list].reverse().find((x) => x.time <= t)
    const src = m.getSource(id) as maplibregl.ImageSource | undefined
    if (!src || !m.getLayer(id)) continue
    m.setLayoutProperty(id, 'visibility', f ? 'visible' : 'none')
    if (f) src.updateImage({ url: f.url, coordinates: f.coordinates })
  }
}
watch([frameIndex, frames], showFrame)

const frameLabel = computed(() => {
  const t = frameTimes.value[frameIndex.value]
  return t
    ? new Date(t).toLocaleString('zh-TW', {
        timeZone: 'Asia/Taipei',
        month: 'numeric',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })
    : ''
})

function toggleFrames() {
  if (framePlaying.value) return stopFrames()
  framePlaying.value = true
  if (frameIndex.value >= frameTimes.value.length - 1) frameIndex.value = 0
  frameTimer = window.setInterval(() => {
    frameIndex.value = frameIndex.value >= frameTimes.value.length - 1 ? 0 : frameIndex.value + 1
  }, 500)
}

function stopFrames() {
  framePlaying.value = false
  window.clearInterval(frameTimer)
}
onBeforeUnmount(stopFrames)

type Extent = [[number, number], [number, number]]
const extents = new Map<string, Extent>()

function nationalBounds(): LngLatBoundsLike {
  const [[w, s], [e, n]] = TAIWAN as Extent
  let b: Extent = [
    [w, s],
    [e, n],
  ]
  for (const [[w2, s2], [e2, n2]] of extents.values())
    b = [
      [Math.min(b[0][0], w2), Math.min(b[0][1], s2)],
      [Math.max(b[1][0], e2), Math.max(b[1][1], n2)],
    ]
  return b
}

function countyOpacity(): maplibregl.ExpressionSpecification | number {
  return props.county ? ['case', ['==', ['get', 'code'], props.county], 0.8, 0.18] : 0.72
}

async function sync() {
  const m = map.value
  if (!m || !ready.value) return
  const want = new Set(props.layers.map(layerId))
  for (const id of [...added]) if (!want.has(id)) removeLayer(id)
  const ordered = [...props.layers].sort(
    (a, b) =>
      Number(a.layer.source_type !== 'county_boundary') -
      Number(b.layer.source_type !== 'county_boundary'),
  )
  for (const a of ordered) {
    if (added.has(layerId(a)) && a.layer.source_type === 'county_boundary') {
      m.setPaintProperty(layerId(a), 'fill-opacity', countyOpacity())
      continue
    }
    if (added.has(layerId(a))) continue
    await addLayer(a)
  }
  const raster = (a: ActiveLayer) => a.layer.layer_type === 'raster'
  for (const a of [...ordered.filter(raster), ...ordered.filter((x) => !raster(x))])
    if (m.getLayer(layerId(a)) && a.layer.source_type !== 'county_boundary') m.moveLayer(layerId(a))
}

watch([() => props.layers.map(layerId).join(','), ready, counties], () => void sync())

function countyBounds(code: string): LngLatBoundsLike | null {
  const f = counties.value?.features.find((x) => x.properties.code === code)
  if (!f) return null
  const g = rewind(f).geometry
  let poly: GeoJSON.Polygon | null = null
  if (g.type === 'Polygon') poly = g
  else if (g.type === 'MultiPolygon') {
    let best = 0
    for (const p of g.coordinates) {
      const b = geoBounds({ type: 'Polygon', coordinates: p })
      const area = (b[1][0] - b[0][0]) * (b[1][1] - b[0][1])
      if (area > best) {
        best = area
        poly = { type: 'Polygon', coordinates: p }
      }
    }
  }
  if (!poly) return null
  const [[w, s], [e, n]] = geoBounds(poly)
  return [
    [w, s],
    [e, n],
  ]
}

function padding() {
  return { top: 40, left: 40, right: 40, bottom: 40 + (props.insetBottom ?? 0) }
}

watch(
  () => props.insetBottom,
  () => {
    if (!userMoved) focus()
  },
)

function focus() {
  const m = map.value
  if (!m || !ready.value) return
  const b = props.county ? countyBounds(props.county) : nationalBounds()
  if (b) m.fitBounds(b, { padding: padding(), duration: userMoved ? 600 : 0 })
  for (const id of added)
    if (m.getLayer(id) && m.getLayer(id)!.type === 'fill')
      m.setPaintProperty(id, 'fill-opacity', countyOpacity())
  for (const a of props.layers) {
    if (
      a.layer.source_type === 'query' &&
      a.layer.layer_type !== 'raster' &&
      a.layer.layer_type !== 'wind' &&
      added.has(layerId(a))
    ) {
      ;(m.getSource(layerId(a)) as GeoJSONSource | undefined)?.setData(
        layerUrl(a.component.slug, a.layer.id, props.county),
      )
    }
  }
}
function fetchLayer(url: string): Promise<GeoJSON.FeatureCollection | null> {
  return fetch(url, { cache: 'no-cache' })
    .then((r) => (r.ok ? (r.json() as Promise<GeoJSON.FeatureCollection>) : null))
    .catch(() => null)
}

async function refreshLayers() {
  const m = map.value
  if (!m || !ready.value) return
  for (const a of [...props.layers]) {
    const id = layerId(a)
    if (!added.has(id)) continue
    if (a.layer.source_type === 'county_boundary') {
      const data = await countyCollection(a, true)
      if (data) (m.getSource(id) as GeoJSONSource | undefined)?.setData(data)
      continue
    }
    const url = layerUrl(a.component.slug, a.layer.id, props.county)
    if (a.layer.layer_type === 'raster') {
      const style = (a.layer.style ?? {}) as { paint?: Record<string, unknown> }
      await addFrames(id, url, style.paint ?? {})
    } else if (a.layer.layer_type === 'wind') {
      await addWind(id, url)
    } else {
      const fc = await fetchLayer(url)
      if (fc) (m.getSource(id) as GeoJSONSource | undefined)?.setData(fc)
    }
  }
}
onScheduledRefresh(refreshLayers)

watch(
  () => props.county,
  () => (userMoved = false),
)
watch([() => props.county, ready, counties], focus)

function flyToCounty(code: string) {
  const b = countyBounds(code)
  if (b) map.value?.fitBounds(b, { padding: padding(), duration: 600 })
}
defineExpose({ flyToCounty })

function onMapClick(e: maplibregl.MapMouseEvent) {
  const m = map.value!
  const ids = [...added].filter((id) => m.getLayer(id))
  const f = ids.length ? m.queryRenderedFeatures(e.point, { layers: ids })[0] : undefined
  if (!f) {
    popup?.remove()
    emit('select', null)
    return
  }
  const a = handlers[f.layer.id]
  if (a) showFeature(a, f, e.lngLat)
}

function showFeature(a: ActiveLayer, f: maplibregl.MapGeoJSONFeature, lngLat: maplibregl.LngLat) {
  const p = f.properties as Record<string, unknown>
  let title: string
  let rows: [string, string][]
  if (a.layer.source_type === 'county_boundary') {
    title = String(p.name ?? '')
    rows = [[a.component.name, String(p.label ?? '')]]
  } else {
    const fields = (a.layer.popup_fields ?? []) as { key: string; name: string }[]
    title = a.layer.title
    rows = fields.map((x) => [
      x.name,
      p[x.key] === null || p[x.key] === undefined ? '—' : String(p[x.key]),
    ])
  }
  if (mobile.matches) {
    emit('select', { title, rows })
    return
  }
  const box = document.createElement('div')
  box.className = 'map-popup'
  const h = document.createElement('h4')
  h.textContent = title
  box.appendChild(h)
  for (const [k, v] of rows) {
    const line = document.createElement('p')
    const key = document.createElement('span')
    key.textContent = k
    const val = document.createElement('strong')
    val.textContent = v
    line.append(key, val)
    box.appendChild(line)
  }
  popup?.remove()
  popup = new maplibregl.Popup({ closeButton: true, maxWidth: '260px' })
    .setLngLat(lngLat)
    .setDOMContent(box)
    .addTo(map.value!)
}
</script>

<template>
  <div class="map-root">
    <div ref="el" class="map-canvas" role="region" aria-label="地圖"></div>
    <div
      v-if="frameTimes.length > 1"
      class="frame-bar"
      :style="{ bottom: `${(insetBottom ?? 0) + 28}px` }"
    >
      <button
        type="button"
        class="btn-plain"
        :aria-label="framePlaying ? '暫停' : '播放最近的變化'"
        @click="toggleFrames"
      >
        <span class="mi" aria-hidden="true">{{ framePlaying ? 'pause' : 'play_arrow' }}</span>
      </button>
      <input
        v-model.number="frameIndex"
        type="range"
        min="0"
        :max="frameTimes.length - 1"
        aria-label="影像圖層的時間"
        :aria-valuetext="frameLabel"
        @input="stopFrames"
      />
      <span class="frame-time">{{ frameLabel }}</span>
    </div>
    <p v-if="windLabel" class="wind-note">{{ windLabel }}</p>
  </div>
</template>

<style scoped>
.map-root {
  position: relative;
  width: 100%;
  height: 100%;
}
.map-canvas {
  width: 100%;
  height: 100%;
}
.frame-bar {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  width: min(420px, calc(100% - 24px));
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text-secondary);
  font-size: var(--font-s);
  box-shadow: 0 0 5px rgb(0 0 0 / 40%);
}
.wind-note {
  position: absolute;
  top: 10px;
  left: 50%;
  transform: translateX(-50%);
  margin: 0;
  white-space: nowrap;
  padding: 2px 8px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text-secondary);
  font-size: var(--font-s);
  pointer-events: none;
}
.frame-bar input {
  flex: 1;
}
.frame-time {
  min-width: 7.5em;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>

<style>
.maplibregl-popup-content {
  padding: 8px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  color: var(--text-primary);
  box-shadow: 0 0 5px rgb(0 0 0 / 60%);
}
.maplibregl-popup-tip {
  display: none;
}
.maplibregl-popup-close-button {
  color: var(--text-muted);
}
.map-popup h4 {
  margin: 0 0 4px;
  font-size: var(--font-ms);
}
.map-popup p {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
  font-size: 0.85rem;
}
.map-popup span {
  color: var(--text-muted);
}
.maplibregl-ctrl-group {
  border-radius: var(--radius) !important;
}
</style>
