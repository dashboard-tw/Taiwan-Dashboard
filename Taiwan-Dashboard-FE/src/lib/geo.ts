import { geoArea, geoContains } from 'd3-geo'
import type { CountyCollection, CountyFeature } from '../api/client'

export function rewind(f: CountyFeature): CountyFeature {
  const g = f.geometry
  const fix = (rings: GeoJSON.Position[][]) => {
    const poly: GeoJSON.Polygon = { type: 'Polygon', coordinates: rings }
    return geoArea(poly) > 2 * Math.PI ? rings.map((r) => [...r].reverse()) : rings
  }
  if (g.type === 'Polygon') return { ...f, geometry: { ...g, coordinates: fix(g.coordinates) } }
  if (g.type === 'MultiPolygon')
    return { ...f, geometry: { ...g, coordinates: g.coordinates.map(fix) } }
  return f
}

export function countyAt(geo: CountyCollection, lon: number, lat: number): CountyFeature | null {
  for (const f of geo.features) {
    const r = rewind(f)
    if (geoContains(r, [lon, lat])) return r
  }
  return null
}
