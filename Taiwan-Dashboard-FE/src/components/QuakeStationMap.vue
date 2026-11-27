<script setup lang="ts">
import * as maplibregl from 'maplibre-gl'
import type { Map as MapLibreMap } from 'maplibre-gl'
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import type { QuakeStation } from '../api/client'
import { applyBasemapTheme, basemapStyle, TAIWAN } from '../lib/basemap'
import { formatNumber } from '../lib/format'
import { intensityLevel } from '../lib/quake'
import { resolvedTheme } from '../theme'

const props = defineProps<{
  stations: QuakeStation[]
  lon: number | null
  lat: number | null
}>()
const emit = defineEmits<{ select: [id: string] }>()

const el = ref<HTMLDivElement | null>(null)
const map = shallowRef<MapLibreMap | null>(null)
let resizeObserver: ResizeObserver | null = null
let popup: maplibregl.Popup | null = null

function stationsGeoJSON(): GeoJSON.FeatureCollection {
  return {
    type: 'FeatureCollection',
    features: props.stations
      .filter((s) => s.lon !== null && s.lat !== null)
      // 震度大的畫在上面
      .sort((a, b) => (a.rank ?? 0) - (b.rank ?? 0))
      .map((s) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [s.lon!, s.lat!] },
        properties: {
          id: s.id,
          color: intensityLevel(s.intensity)?.color ?? '#a8a29e',
          rank: s.rank ?? 0,
        },
      })),
  }
}

function fit(m: MapLibreMap) {
  const pts: [number, number][] = props.stations
    .filter((s) => s.lon !== null && s.lat !== null)
    .map((s) => [s.lon!, s.lat!])
  if (props.lon !== null && props.lat !== null) pts.push([props.lon, props.lat])
  if (pts.length < 2) {
    m.fitBounds(TAIWAN, { padding: 20, animate: false })
    return
  }
  const b = new maplibregl.LngLatBounds(pts[0], pts[0])
  for (const p of pts) b.extend(p)
  m.fitBounds(b, { padding: 40, maxZoom: 9, animate: false })
}

function num(v: number | null, d: number) {
  return v === null ? '—' : formatNumber(v, d)
}

function popupHTML(s: QuakeStation): string {
  const box = document.createElement('div')
  const title = document.createElement('strong')
  title.textContent = `${s.name}${s.county ? `（${s.county}）` : ''}`
  box.append(title)
  for (const [k, v] of [
    ['震度', s.intensity ?? '—'],
    ['最大加速度', `${num(s.pga_gal, 1)} gal`],
    ['最大速度', `${num(s.pgv_kine, 2)} kine`],
    ['離震央', `${num(s.distance_km, 1)} 公里`],
  ]) {
    const line = document.createElement('div')
    line.textContent = `${k}：${v}`
    box.append(line)
  }
  return box.innerHTML
}

onMounted(() => {
  const m = new maplibregl.Map({
    container: el.value!,
    style: basemapStyle(),
    bounds: TAIWAN,
    fitBoundsOptions: { padding: 20 },
    maxZoom: 13,
    attributionControl: { compact: true },
    dragRotate: false,
    cooperativeGestures: true,
  })
  m.touchZoomRotate.disableRotation()
  m.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
  map.value = m
  m.on('load', () => {
    m.addSource('stations', { type: 'geojson', data: stationsGeoJSON() })
    m.addLayer({
      id: 'stations',
      type: 'circle',
      source: 'stations',
      paint: {
        'circle-radius': ['interpolate', ['linear'], ['get', 'rank'], 1, 5, 4, 8, 9, 12],
        'circle-color': ['get', 'color'],
        'circle-stroke-color': '#ffffff',
        'circle-stroke-width': 1,
      },
    })
    if (props.lon !== null && props.lat !== null) {
      const star = document.createElement('div')
      star.className = 'epicenter'
      star.textContent = '✕'
      star.title = '震央'
      new maplibregl.Marker({ element: star }).setLngLat([props.lon, props.lat]).addTo(m)
    }
    fit(m)
  })
  m.on('mouseenter', 'stations', () => (m.getCanvas().style.cursor = 'pointer'))
  m.on('mouseleave', 'stations', () => (m.getCanvas().style.cursor = ''))
  m.on('click', 'stations', (e) => {
    const id = e.features?.[0]?.properties?.id as string | undefined
    const s = props.stations.find((x) => x.id === id)
    if (!s || s.lon === null || s.lat === null) return
    popup?.remove()
    popup = new maplibregl.Popup({ closeButton: true, maxWidth: '16rem' })
      .setLngLat([s.lon, s.lat])
      .setHTML(popupHTML(s))
      .addTo(m)
    emit('select', s.id)
  })
  resizeObserver = new ResizeObserver(() => m.resize())
  resizeObserver.observe(el.value!)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  map.value?.remove()
})

watch(resolvedTheme, () => {
  if (map.value?.isStyleLoaded()) applyBasemapTheme(map.value)
})
</script>

<template>
  <div
    ref="el"
    class="quake-map"
    role="region"
    aria-label="各測站震度地圖，點圓點可看測站數值"
  ></div>
</template>

<style scoped>
.quake-map {
  width: 100%;
  height: 26rem;
  border-radius: var(--radius);
  overflow: hidden;
}
.quake-map :deep(.epicenter) {
  color: #d41f1f;
  font-size: 1.6rem;
  font-weight: 700;
  line-height: 1;
  text-shadow:
    0 0 3px #ffffff,
    0 0 3px #ffffff;
}
.quake-map :deep(.maplibregl-popup-content) {
  color: #1c1917;
  font-size: 0.85rem;
  line-height: 1.6;
}
</style>
