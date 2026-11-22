<script setup lang="ts">
import * as maplibregl from 'maplibre-gl'
import type { GeoJSONSource, Map as MapLibreMap, Marker } from 'maplibre-gl'
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { applyBasemapTheme, basemapStyle, TAIWAN } from '../../lib/basemap'
import { resolvedTheme } from '../../theme'

const props = defineProps<{
  lon: number | null
  lat: number | null
  others?: { lon: number; lat: number; name: string }[]
}>()
const emit = defineEmits<{ pick: [lon: number, lat: number] }>()

const el = ref<HTMLDivElement | null>(null)
const map = shallowRef<MapLibreMap | null>(null)
let marker: Marker | null = null
let resizeObserver: ResizeObserver | null = null

const round = (v: number) => Math.round(v * 1e5) / 1e5

function othersGeoJSON(): GeoJSON.FeatureCollection {
  return {
    type: 'FeatureCollection',
    features: (props.others ?? []).map((o) => ({
      type: 'Feature',
      geometry: { type: 'Point', coordinates: [o.lon, o.lat] },
      properties: { name: o.name },
    })),
  }
}

function place(lon: number | null, lat: number | null, fly: boolean) {
  const m = map.value
  if (!m) return
  if (lon === null || lat === null) {
    marker?.remove()
    marker = null
    return
  }
  if (!marker) {
    marker = new maplibregl.Marker({ draggable: true, color: '#5a9cf8' })
      .setLngLat([lon, lat])
      .addTo(m)
    marker.on('dragend', () => {
      const p = marker!.getLngLat()
      emit('pick', round(p.lng), round(p.lat))
    })
  } else {
    marker.setLngLat([lon, lat])
  }
  if (fly) m.jumpTo({ center: [lon, lat], zoom: Math.max(m.getZoom(), 11) })
}

onMounted(() => {
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
  m.getCanvas().style.cursor = 'crosshair'
  map.value = m
  m.on('load', () => {
    m.addSource('others', { type: 'geojson', data: othersGeoJSON() })
    m.addLayer({
      id: 'others',
      type: 'circle',
      source: 'others',
      paint: {
        'circle-radius': 4,
        'circle-color': '#888787',
        'circle-stroke-width': 1,
        'circle-stroke-color': '#ffffff',
      },
    })
    m.addLayer({
      id: 'others-label',
      type: 'symbol',
      source: 'others',
      minzoom: 10,
      layout: { 'text-field': ['get', 'name'], 'text-size': 11, 'text-offset': [0, 1.1] },
      paint: { 'text-color': '#888787', 'text-halo-color': '#000000', 'text-halo-width': 1 },
    })
  })
  m.on('click', (e) => emit('pick', round(e.lngLat.lng), round(e.lngLat.lat)))
  resizeObserver = new ResizeObserver(() => m.resize())
  resizeObserver.observe(el.value!)
  place(props.lon, props.lat, true)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  map.value?.remove()
})

watch(
  () => [props.lon, props.lat] as const,
  ([lon, lat], [oldLon, oldLat]) => {
    const moved = Math.abs((lon ?? 0) - (oldLon ?? 0)) + Math.abs((lat ?? 0) - (oldLat ?? 0)) > 0.05
    place(lon, lat, moved)
  },
)
watch(
  () => props.others,
  () => (map.value?.getSource('others') as GeoJSONSource | undefined)?.setData(othersGeoJSON()),
)
watch(resolvedTheme, () => {
  if (map.value?.isStyleLoaded()) applyBasemapTheme(map.value)
})
</script>

<template>
  <div ref="el" class="picker" role="application" aria-label="在地圖上點選座標"></div>
</template>

<style scoped>
.picker {
  width: 100%;
  height: 100%;
  min-height: 320px;
  border-radius: var(--radius);
  overflow: hidden;
}
</style>
