import * as maplibregl from 'maplibre-gl'
import type { LngLatBoundsLike, Map as MapLibreMap, StyleSpecification } from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import { resolvedTheme } from '../theme'

maplibregl.setWorkerUrl(workerUrl)

export const TAIWAN: LngLatBoundsLike = [
  [119.3, 21.85],
  [122.05, 25.35],
]

export function basemapPaint() {
  return resolvedTheme.value === 'dark'
    ? { 'raster-brightness-max': 0.38, 'raster-saturation': -0.7, 'raster-contrast': 0.1 }
    : { 'raster-brightness-max': 1, 'raster-saturation': -0.2, 'raster-contrast': 0 }
}

export function mapBackground() {
  return getComputedStyle(document.documentElement).getPropertyValue('--bg').trim() || '#090909'
}

export function basemapStyle(): StyleSpecification {
  return {
    version: 8,
    sources: {
      emap: {
        type: 'raster',
        tiles: [
          'https://wmts.nlsc.gov.tw/wmts/EMAP6_OPENDATA/default/GoogleMapsCompatible/{z}/{y}/{x}',
        ],
        tileSize: 256,
        maxzoom: 15,
        attribution:
          '底圖：<a href="https://maps.nlsc.gov.tw/" target="_blank" rel="noopener">內政部國土測繪中心</a>',
      },
    },
    layers: [
      { id: 'background', type: 'background', paint: { 'background-color': mapBackground() } },
      { id: 'emap', type: 'raster', source: 'emap', paint: basemapPaint() },
    ],
  }
}

export function applyBasemapTheme(m: MapLibreMap) {
  m.setPaintProperty('background', 'background-color', mapBackground())
  for (const [k, v] of Object.entries(basemapPaint()))
    m.setPaintProperty('emap', k as 'raster-opacity', v)
}
