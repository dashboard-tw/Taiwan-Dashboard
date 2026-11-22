import { geoArea, geoMercator, geoPath, type GeoStream } from 'd3-geo'
import type { CountyCollection, CountyFeature } from '../api/client'
import { rewind } from './geo'

export const WIDTH = 440
export const HEIGHT = 580
export const INSET = { w: 132, h: 120, gap: 8 }
export const INSET_HEADER = 50
export const OUTLYING = ['09020', '09007'] // 金門縣、連江縣

export interface Shape {
  code: string
  name: string
  d: string
  cx: number
  cy: number
  small: boolean
}

const SMALL_AREA = 450

function extent(west: number, south: number, east: number, north: number): GeoJSON.Feature {
  return {
    type: 'Feature',
    properties: {},
    geometry: {
      type: 'Polygon',
      coordinates: [
        [
          [west, south],
          [west, north],
          [east, north],
          [east, south],
          [west, south],
        ],
      ],
    },
  }
}

const EXTENTS: Record<string, GeoJSON.Feature> = {
  main: extent(119.3, 21.85, 122.05, 25.35), // 臺灣本島與澎湖
  '09020': extent(118.2, 24.37, 118.5, 24.53), // 金門本島與烈嶼
  '09007': extent(119.88, 25.93, 120.53, 26.4), // 馬祖列島
}

function largestPart(f: CountyFeature): GeoJSON.Feature {
  const g = f.geometry
  if (g.type !== 'MultiPolygon') return f
  let best = g.coordinates[0]!
  for (const p of g.coordinates) {
    if (
      geoArea({ type: 'Polygon', coordinates: p }) > geoArea({ type: 'Polygon', coordinates: best })
    )
      best = p
  }
  return { type: 'Feature', properties: {}, geometry: { type: 'Polygon', coordinates: best } }
}

const MIN_STEP = 1

function simplify(out: GeoStream): GeoStream {
  let lx = 0
  let ly = 0
  let px = 0
  let py = 0
  let pending = false
  let first = true
  return {
    point(x, y) {
      if (first || Math.hypot(x - lx, y - ly) >= MIN_STEP) {
        out.point(x, y)
        lx = x
        ly = y
        first = false
        pending = false
      } else {
        px = x
        py = y
        pending = true
      }
    },
    lineStart() {
      first = true
      pending = false
      out.lineStart()
    },
    lineEnd() {
      if (pending) out.point(px, py)
      out.lineEnd()
    },
    polygonStart: () => out.polygonStart(),
    polygonEnd: () => out.polygonEnd(),
    sphere: () => out.sphere?.(),
  }
}

function project(
  raw: CountyFeature[],
  box: [[number, number], [number, number]],
  fit: GeoJSON.Feature,
): Shape[] {
  const features = raw.map(rewind)
  const projection = geoMercator().fitExtent(box, fit)
  const [[x0, y0], [x1, y1]] = box
  projection.clipExtent([
    [x0 - 6, y0 - 6],
    [x1 + 6, y1 + 6],
  ])
  const path = geoPath({ stream: (s: GeoStream) => projection.stream(simplify(s)) }).digits(1)
  return features.map((f) => {
    const part = largestPart(f)
    const [cx, cy] = path.centroid(part)
    const small = path.area(part) < SMALL_AREA
    return { code: f.properties.code, name: f.properties.name, d: path(f) ?? '', cx, cy, small }
  })
}

const shapeCache = new WeakMap<CountyCollection, Shape[]>()

export function countyShapes(geo: CountyCollection): Shape[] {
  const cached = shapeCache.get(geo)
  if (cached) return cached
  const main = geo.features.filter((f) => !OUTLYING.includes(f.properties.code))
  const out: Shape[] = project(
    main,
    [
      [4, 4],
      [WIDTH - 4, HEIGHT - 4],
    ],
    EXTENTS.main!,
  )
  OUTLYING.forEach((code, i) => {
    const f = geo.features.find((x) => x.properties.code === code)
    if (!f) return
    const y = 4 + i * (INSET.h + INSET.gap)
    out.push(
      ...project(
        [f],
        [
          [4 + 8, y + INSET_HEADER],
          [4 + INSET.w - 8, y + INSET.h - 8],
        ],
        EXTENTS[code]!,
      ),
    )
  })
  shapeCache.set(geo, out)
  return out
}
