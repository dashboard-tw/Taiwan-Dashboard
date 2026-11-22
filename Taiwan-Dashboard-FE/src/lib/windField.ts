import type { Map as MapLibreMap } from 'maplibre-gl'

export const WIND_MAX = 50

export interface WindGrid {
  w: number
  h: number
  west: number
  south: number
  east: number
  north: number
  u: Float32Array
  v: Float32Array
  valid: Uint8Array
}

export async function loadWindGrid(
  url: string,
  bounds: { west: number; south: number; east: number; north: number },
): Promise<WindGrid> {
  const img = new Image()
  img.src = url
  await img.decode()
  const canvas = document.createElement('canvas')
  canvas.width = img.naturalWidth
  canvas.height = img.naturalHeight
  const ctx = canvas.getContext('2d', { willReadFrequently: true })!
  ctx.drawImage(img, 0, 0)
  const px = ctx.getImageData(0, 0, canvas.width, canvas.height).data
  const n = canvas.width * canvas.height
  const g: WindGrid = {
    w: canvas.width,
    h: canvas.height,
    ...bounds,
    u: new Float32Array(n),
    v: new Float32Array(n),
    valid: new Uint8Array(n),
  }
  for (let i = 0; i < n; i++) {
    if (px[i * 4 + 3]! < 128) continue
    g.u[i] = (px[i * 4]! / 254) * 2 * WIND_MAX - WIND_MAX
    g.v[i] = (px[i * 4 + 1]! / 254) * 2 * WIND_MAX - WIND_MAX
    g.valid[i] = 1
  }
  return g
}

export function sampleWind(g: WindGrid, lon: number, lat: number): [number, number] | null {
  const fx = ((lon - g.west) / (g.east - g.west)) * (g.w - 1)
  const fy = ((g.north - lat) / (g.north - g.south)) * (g.h - 1)
  if (!(fx >= 0 && fy >= 0 && fx <= g.w - 1 && fy <= g.h - 1)) return null
  const x0 = Math.min(Math.floor(fx), g.w - 2)
  const y0 = Math.min(Math.floor(fy), g.h - 2)
  const dx = fx - x0
  const dy = fy - y0
  const i00 = y0 * g.w + x0
  const i10 = i00 + 1
  const i01 = i00 + g.w
  const i11 = i01 + 1
  if (!g.valid[i00] || !g.valid[i10] || !g.valid[i01] || !g.valid[i11]) return null
  const lerp = (a: Float32Array) =>
    (a[i00]! * (1 - dx) + a[i10]! * dx) * (1 - dy) + (a[i01]! * (1 - dx) + a[i11]! * dx) * dy
  return [lerp(g.u), lerp(g.v)]
}

interface Particle {
  x: number
  y: number
  age: number
}

const BANDS = [3, 8, 14, 20]
const MAX_AGE = 90
const STEP = 0.09

export class WindAnimation {
  private map: MapLibreMap
  private canvas: HTMLCanvasElement
  private ctx: CanvasRenderingContext2D
  private grid: WindGrid | null = null
  private particles: Particle[] = []
  private raf = 0
  private moving = false
  private dark = false
  private reduced = window.matchMedia('(prefers-reduced-motion: reduce)')

  constructor(map: MapLibreMap) {
    this.map = map
    this.canvas = document.createElement('canvas')
    this.canvas.className = 'wind-canvas'
    this.canvas.setAttribute('aria-hidden', 'true')
    Object.assign(this.canvas.style, {
      position: 'absolute',
      inset: '0',
      pointerEvents: 'none',
    })
    const container = map.getContainer()
    container.insertBefore(this.canvas, container.querySelector('.maplibregl-control-container'))
    this.ctx = this.canvas.getContext('2d')!
    map.on('movestart', this.onMoveStart)
    map.on('moveend', this.onMoveEnd)
    map.on('resize', this.onMoveEnd)
    this.resize()
  }

  setTheme(dark: boolean) {
    this.dark = dark
  }

  setGrid(g: WindGrid | null) {
    this.grid = g
    this.reset()
  }

  destroy() {
    cancelAnimationFrame(this.raf)
    this.map.off('movestart', this.onMoveStart)
    this.map.off('moveend', this.onMoveEnd)
    this.map.off('resize', this.onMoveEnd)
    this.canvas.remove()
  }

  private onMoveStart = () => {
    this.moving = true
    this.clear()
  }

  private onMoveEnd = () => {
    this.moving = false
    this.resize()
    this.reset()
  }

  private resize() {
    const r = window.devicePixelRatio || 1
    const { clientWidth: w, clientHeight: h } = this.map.getContainer()
    this.canvas.width = Math.round(w * r)
    this.canvas.height = Math.round(h * r)
    this.canvas.style.width = `${w}px`
    this.canvas.style.height = `${h}px`
    this.ctx.setTransform(r, 0, 0, r, 0, 0)
  }

  private clear() {
    this.ctx.clearRect(0, 0, this.canvas.width, this.canvas.height)
  }

  private reset() {
    cancelAnimationFrame(this.raf)
    this.clear()
    if (!this.grid || this.moving) return
    const { clientWidth: w, clientHeight: h } = this.map.getContainer()
    const n = Math.min(5000, Math.round((w * h) / 220))
    this.particles = Array.from({ length: n }, () => this.spawn(Math.random() * MAX_AGE))
    if (this.reduced.matches) {
      for (let i = 0; i < 40; i++) this.step(false)
      return
    }
    const loop = () => {
      this.step(true)
      this.raf = requestAnimationFrame(loop)
    }
    this.raf = requestAnimationFrame(loop)
  }

  private spawn(age = 0): Particle {
    const { clientWidth: w, clientHeight: h } = this.map.getContainer()
    return { x: Math.random() * w, y: Math.random() * h, age }
  }

  private step(fade: boolean) {
    const g = this.grid
    if (!g || this.moving) return
    const ctx = this.ctx
    if (fade) {
      ctx.globalCompositeOperation = 'destination-in'
      ctx.fillStyle = 'rgba(0, 0, 0, 0.93)'
      ctx.fillRect(0, 0, this.canvas.width, this.canvas.height)
      ctx.globalCompositeOperation = 'source-over'
    }
    const paths = BANDS.map(() => new Path2D())
    for (let i = 0; i < this.particles.length; i++) {
      let p = this.particles[i]!
      if (p.age++ > MAX_AGE) p = this.particles[i] = this.spawn()
      const ll = this.map.unproject([p.x, p.y])
      const wind = sampleWind(g, ll.lng, ll.lat)
      if (!wind) {
        this.particles[i] = this.spawn(Math.random() * MAX_AGE)
        continue
      }
      const [u, v] = wind
      const speed = Math.hypot(u, v)
      const nx = p.x + u * STEP
      const ny = p.y - v * STEP
      let band = 0
      while (band < BANDS.length - 1 && speed >= BANDS[band + 1]!) band++
      paths[band]!.moveTo(p.x, p.y)
      paths[band]!.lineTo(nx, ny)
      p.x = nx
      p.y = ny
    }
    ctx.lineWidth = 1.2
    ctx.lineCap = 'round'
    paths.forEach((path, i) => {
      ctx.strokeStyle = this.color(i)
      ctx.stroke(path)
    })
  }

  private color(band: number): string {
    const alpha = [0.45, 0.65, 0.8, 0.95][band]!
    return this.dark ? `rgba(240, 246, 252, ${alpha})` : `rgba(23, 52, 89, ${alpha})`
  }
}
