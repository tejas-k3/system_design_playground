import rough from 'roughjs'
import type { RoughCanvas } from 'roughjs/bin/canvas'
import type { Drawable } from 'roughjs/bin/core'
import type { Diagram, Node, Stats } from '../api/types'
import type { Reg } from '../model/diagram'
import { clientRateLabel, edgeLabel, utilBarWidth } from '../model/rates'
import { type Rect, type Pt, NOTE_FONT, boundaryPoint, center, edgeGeom, handlePos, handleRadius, legibleSize, noteRect, nodeRect, HANDLE_R } from './geometry'
import { drawIcon } from './icons'
import { FONT, type Palette, utilColor } from './theme'

export interface ActiveParticle {
  edge: string
  back: boolean
  fail: boolean
  t0: number
}

export interface Scene {
  diagram: Diagram
  reg: Reg
  stats: Stats
  running: boolean
  viewport: { scrollX: number; scrollY: number; zoom: number }
  palette: Palette
  selection: Set<string>
  hover: string | null
  handles: Set<string> // components that show their connection handle
  connect: { from: string; to: Pt; target: string | null } | null
  validTargets: Set<string>
  marquee: Rect | null
  particles: ActiveParticle[]
  now: number
  editingNote: string | null
  showParticles: boolean
}

export const PARTICLE_MS = 520

function hash(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return (h >>> 0) % 2147483647 || 1
}

const drawableCache = new Map<string, Drawable>()
function cached(key: string, make: () => Drawable): Drawable {
  let d = drawableCache.get(key)
  if (!d) {
    if (drawableCache.size > 2000) drawableCache.clear()
    d = make()
    drawableCache.set(key, d)
  }
  return d
}

let measureCtx: CanvasRenderingContext2D | null = null
export function measureText(text: string, size: number): { w: number; h: number } {
  measureCtx ??= document.createElement('canvas').getContext('2d')!
  measureCtx.font = `${size}px ${FONT}`
  const lines = (text || 'Note').split('\n')
  return { w: Math.max(...lines.map((l) => measureCtx!.measureText(l).width)), h: lines.length * size * 1.25 }
}

export function createRough(canvas: HTMLCanvasElement): RoughCanvas {
  return rough.canvas(canvas)
}

function roundedPath(r: Rect, rad: number): string {
  const { x, y, w, h } = r
  return `M ${x + rad} ${y} L ${x + w - rad} ${y} Q ${x + w} ${y} ${x + w} ${y + rad} L ${x + w} ${y + h - rad} Q ${x + w} ${y + h} ${x + w - rad} ${y + h} L ${x + rad} ${y + h} Q ${x} ${y + h} ${x} ${y + h - rad} L ${x} ${y + rad} Q ${x} ${y} ${x + rad} ${y} Z`
}

function drawGrid(ctx: CanvasRenderingContext2D, w: number, h: number, sc: Scene) {
  const v = sc.viewport
  if (v.zoom < 0.35) return
  const step = 24 * v.zoom
  ctx.fillStyle = sc.palette.grid
  const ox = ((v.scrollX % step) + step) % step
  const oy = ((v.scrollY % step) + step) % step
  for (let x = ox; x < w; x += step) for (let y = oy; y < h; y += step) ctx.fillRect(x - 0.75, y - 0.75, 1.5, 1.5)
}

function drawEdge(ctx: CanvasRenderingContext2D, rc: RoughCanvas, sc: Scene, nodes: Map<string, Node>) {
  const { palette: p, stats } = sc
  for (const e of sc.diagram.edges) {
    const g = edgeGeom(e, nodes, sc.reg)
    if (!g) continue
    const st = stats.edges[e.id]
    const selected = sc.selection.has(e.id)
    const hot = sc.hover === e.id
    let color = p.ink
    let dash: number[] | undefined
    if (st?.breaker === 'open') {
      color = p.bad
      dash = [6, 6]
    } else if (st?.breaker === 'halfOpen') {
      color = p.warn
      dash = [3, 5]
    } else if (st?.ejected) {
      color = p.warn
      dash = [6, 6]
    } else if (selected) color = p.accent
    const width = 1.6 + (st && st.rps > 0 ? Math.min(3.5, Math.log10(1 + st.rps) * 0.9) : 0) + (hot ? 0.6 : 0)
    const line = cached(`l:${e.id}:${Math.round(g.a.x)}:${Math.round(g.a.y)}:${Math.round(g.b.x)}:${Math.round(g.b.y)}`, () =>
      rc.generator.line(g.a.x, g.a.y, g.b.x, g.b.y, { roughness: 0.9, bowing: 1.2, seed: hash(e.id) }),
    )
    rc.draw({ ...line, options: { ...line.options, stroke: color, strokeWidth: width, strokeLineDash: dash } })
    // arrowhead
    const ang = Math.atan2(g.b.y - g.a.y, g.b.x - g.a.x)
    ctx.strokeStyle = color
    ctx.lineWidth = Math.max(1.6, width)
    ctx.lineCap = 'round'
    ctx.beginPath()
    ctx.moveTo(g.b.x - 11 * Math.cos(ang - 0.45), g.b.y - 11 * Math.sin(ang - 0.45))
    ctx.lineTo(g.b.x, g.b.y)
    ctx.lineTo(g.b.x - 11 * Math.cos(ang + 0.45), g.b.y - 11 * Math.sin(ang + 0.45))
    ctx.stroke()
    // label
    const label = edgeLabel(e.latencyMs, st?.rps ?? 0, selected || hot, sc.running)
    if (label) {
      const mx = (g.a.x + g.b.x) / 2
      const my = (g.a.y + g.b.y) / 2
      ctx.font = `13px ${FONT}`
      const tw = ctx.measureText(label).width
      ctx.fillStyle = p.bg
      ctx.globalAlpha = 0.9
      ctx.fillRect(mx - tw / 2 - 3, my - 9, tw + 6, 17)
      ctx.globalAlpha = 1
      ctx.fillStyle = st?.breaker === 'open' ? p.bad : p.muted
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.fillText(st?.breaker === 'open' ? `open · ${label}` : label, mx, my)
    }
  }
}

function drawParticles(ctx: CanvasRenderingContext2D, sc: Scene, nodes: Map<string, Node>) {
  const edges = new Map(sc.diagram.edges.map((e) => [e.id, e]))
  for (const pt of sc.particles) {
    const e = edges.get(pt.edge)
    if (!e) continue
    const g = edgeGeom(e, nodes, sc.reg)
    if (!g) continue
    const t = (sc.now - pt.t0) / PARTICLE_MS
    if (t < 0 || t > 1) continue
    const k = pt.back ? 1 - t : t
    const off = pt.back ? 5 : -5
    const ang = Math.atan2(g.b.y - g.a.y, g.b.x - g.a.x)
    const x = g.a.x + (g.b.x - g.a.x) * k - Math.sin(ang) * off
    const y = g.a.y + (g.b.y - g.a.y) * k + Math.cos(ang) * off
    ctx.globalAlpha = 1 - Math.pow(Math.abs(t - 0.5) * 2, 3) * 0.6
    ctx.fillStyle = pt.fail ? sc.palette.bad : pt.back ? sc.palette.ok : sc.palette.accent
    ctx.beginPath()
    ctx.arc(x, y, pt.fail ? 4 : 3.2, 0, Math.PI * 2)
    ctx.fill()
  }
  ctx.globalAlpha = 1
}

/** Requests per second leaving a node, from the live stats of its outgoing connections. */
function sentRate(id: string, sc: Scene): number {
  return sc.diagram.edges.reduce((sum, e) => (e.from === id ? sum + (sc.stats.edges[e.id]?.rps ?? 0) : sum), 0)
}

function nodeSubtitle(n: Node, sc: Scene): string {
  const spec = sc.reg.spec(n.kind)!
  const rp = sc.reg.resolve(n)
  switch (n.kind) {
    case 'client':
      return clientRateLabel(rp as { rps: number; pattern: string; peakRps: number }, sc.running, sentRate(n.id, sc))
    case 'appServer':
      return `${rp.threads} threads`
    case 'database':
    case 'readReplica':
      return `${rp.connections} conns`
    case 'cache':
    case 'cdn':
      return `${rp.capacityItems} items`
    case 'worker':
      return `×${rp.concurrency}`
    case 'queue':
      return `cap ${rp.capacity}`
    case 'rateLimiter':
      return `${rp.limitRps}/s`
    default:
      return spec.name
  }
}

/** A small status pill whose right edge is at x and whose bottom edge is at y (scale k keeps it legible when zoomed out). */
function chip(ctx: CanvasRenderingContext2D, text: string, x: number, y: number, bg: string, fg: string, k: number): number {
  ctx.font = `${12 * k}px ${FONT}`
  const w = ctx.measureText(text).width + 10 * k
  ctx.fillStyle = bg
  ctx.beginPath()
  ctx.roundRect(x - w, y - 17 * k, w, 17 * k, 8 * k)
  ctx.fill()
  ctx.fillStyle = fg
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(text, x - w / 2, y - 8.5 * k)
  return w + 4 * k
}

function drawNode(ctx: CanvasRenderingContext2D, rc: RoughCanvas, sc: Scene, n: Node) {
  const spec = sc.reg.spec(n.kind)
  if (!spec) return
  const { palette: p } = sc
  const r = nodeRect(n, sc.reg)
  const ns = sc.stats.nodes[n.id]
  const selected = sc.selection.has(n.id)
  const down = !!ns?.down
  const failing = !!ns && ns.arr > 2 && ns.err / Math.max(ns.arr, 1) > 0.02

  const body = cached(`n:${n.id}:${r.w}:${r.h}:${p.card}`, () =>
    rc.generator.path(roundedPath(r, 10), { roughness: 1.05, bowing: 0.8, seed: hash(n.id), fill: p.card, fillStyle: 'solid' }),
  )
  if (failing && sc.running) {
    ctx.save()
    ctx.shadowColor = p.bad
    ctx.shadowBlur = 14
    ctx.fillStyle = 'rgba(0,0,0,0)'
    ctx.strokeStyle = p.bad
    ctx.lineWidth = 2
    ctx.beginPath()
    ctx.roundRect(r.x - 2, r.y - 2, r.w + 4, r.h + 4, 12)
    ctx.stroke()
    ctx.restore()
  }
  rc.draw({ ...body, options: { ...body.options, stroke: down ? p.bad : p.ink, strokeWidth: selected ? 2.4 : 1.7, fill: p.card } })
  if (selected) {
    ctx.save()
    ctx.setLineDash([5, 4])
    ctx.strokeStyle = p.accent
    ctx.lineWidth = 1.6
    ctx.beginPath()
    ctx.roundRect(r.x - 6, r.y - 6, r.w + 12, r.h + 12, 14)
    ctx.stroke()
    ctx.restore()
  }
  // icon badge
  ctx.fillStyle = spec.accent
  ctx.globalAlpha = 0.14
  ctx.beginPath()
  ctx.arc(r.x + 26, r.y + 30, 19, 0, Math.PI * 2)
  ctx.fill()
  ctx.globalAlpha = 1
  drawIcon(ctx, n.kind, r.x + 26, r.y + 30, spec.accent, 0.95)
  // text
  ctx.textAlign = 'left'
  ctx.textBaseline = 'alphabetic'
  ctx.fillStyle = p.ink
  const zoom = sc.viewport.zoom
  ctx.font = `bold ${legibleSize(16, zoom, 11)}px ${FONT}`
  let label = n.label
  const maxW = r.w - 62
  while (label.length > 3 && ctx.measureText(label).width > maxW) label = label.slice(0, -2)
  if (label !== n.label) label += '…'
  ctx.fillText(label, r.x + 52, r.y + 28)
  ctx.fillStyle = p.muted
  ctx.font = `${legibleSize(13, zoom, 10)}px ${FONT}`
  ctx.fillText(nodeSubtitle(n, sc), r.x + 52, r.y + 46)
  // utilization bar
  if (ns && sc.running && n.kind !== 'client' && n.kind !== 'loadBalancer') {
    const bx = r.x + 12
    const by = r.y + r.h - 12
    const bw = r.w - 24
    ctx.fillStyle = p.grid
    ctx.beginPath()
    ctx.roundRect(bx, by, bw, 5, 3)
    ctx.fill()
    ctx.fillStyle = utilColor(ns.util, p)
    ctx.beginPath()
    ctx.roundRect(bx, by, utilBarWidth(ns.util, bw), 5, 3)
    ctx.fill()
  }
  // chips sit just above the node, right-aligned, so they never cover its title
  let cx = r.x + r.w - 4
  const cy = r.y - 4
  const ck = legibleSize(12, zoom, 10) / 12
  if (down) cx -= chip(ctx, 'DOWN', cx, cy, p.bad, '#fff', ck)
  else if (ns?.slow) cx -= chip(ctx, `${ns.slow}× slow`, cx, cy, p.warn, '#1f1f1f', ck)
  if (ns && sc.running) {
    if (ns.qLen > 0) {
      const full = ns.qLen >= (ns.cap || 1e9)
      cx -= chip(ctx, `q ${ns.qLen}`, cx, cy, full ? p.bad : p.grid, full ? '#fff' : p.ink, ck)
    }
    if (ns.hitRate >= 0) chip(ctx, `hit ${Math.round(ns.hitRate * 100)}%`, cx, cy, p.grid, p.ink, ck)
  }
  if (n.locked) {
    ctx.fillStyle = p.muted
    ctx.font = `11px ${FONT}`
    ctx.textAlign = 'right'
    ctx.fillText('fixed', r.x + r.w - 8, r.y + r.h - 18)
  }
}

function drawHandle(ctx: CanvasRenderingContext2D, sc: Scene, n: Node) {
  const zoom = sc.viewport.zoom
  const h = handlePos(nodeRect(n, sc.reg), zoom)
  const r = handleRadius(zoom)
  const k = r / HANDLE_R
  ctx.fillStyle = sc.palette.card
  ctx.strokeStyle = sc.palette.accent
  ctx.lineWidth = 2 * Math.max(1, k)
  ctx.beginPath()
  ctx.arc(h.x, h.y, r, 0, Math.PI * 2)
  ctx.fill()
  ctx.stroke()
  ctx.beginPath()
  ctx.moveTo(h.x - 3.5 * k, h.y)
  ctx.lineTo(h.x + 3.5 * k, h.y)
  ctx.moveTo(h.x + 0.5 * k, h.y - 3 * k)
  ctx.lineTo(h.x + 3.5 * k, h.y)
  ctx.lineTo(h.x + 0.5 * k, h.y + 3 * k)
  ctx.stroke()
}

function drawNotes(ctx: CanvasRenderingContext2D, sc: Scene) {
  const p = sc.palette
  for (const n of sc.diagram.notes) {
    if (n.id === sc.editingNote) continue
    const r = noteRect(n, measureText)
    ctx.save()
    ctx.fillStyle = p.noteBg
    ctx.shadowColor = 'rgba(0,0,0,0.18)'
    ctx.shadowBlur = 6
    ctx.shadowOffsetY = 2
    ctx.beginPath()
    ctx.roundRect(r.x, r.y, r.w, r.h, 4)
    ctx.fill()
    ctx.restore()
    if (sc.selection.has(n.id)) {
      ctx.strokeStyle = p.accent
      ctx.setLineDash([5, 4])
      ctx.lineWidth = 1.6
      ctx.strokeRect(r.x - 4, r.y - 4, r.w + 8, r.h + 8)
      ctx.setLineDash([])
    }
    const size = n.fontSize ?? NOTE_FONT
    ctx.fillStyle = sc.palette.ink
    ctx.font = `${size}px ${FONT}`
    ctx.textAlign = 'left'
    ctx.textBaseline = 'top'
    ;(n.text || 'Double-click to edit').split('\n').forEach((line, i) => {
      if (!n.text) ctx.globalAlpha = 0.5
      ctx.fillText(line, r.x + 8, r.y + 6 + i * size * 1.25)
      ctx.globalAlpha = 1
    })
  }
}

export function render(ctx: CanvasRenderingContext2D, rc: RoughCanvas, width: number, height: number, dpr: number, sc: Scene): void {
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.fillStyle = sc.palette.bg
  ctx.fillRect(0, 0, width, height)
  drawGrid(ctx, width, height, sc)
  const v = sc.viewport
  ctx.save()
  ctx.translate(v.scrollX, v.scrollY)
  ctx.scale(v.zoom, v.zoom)
  const nodes = new Map(sc.diagram.nodes.map((n) => [n.id, n]))
  drawEdge(ctx, rc, sc, nodes)
  if (sc.showParticles) drawParticles(ctx, sc, nodes)
  // dim nodes that cannot accept the connection being dragged
  for (const n of sc.diagram.nodes) {
    if (sc.connect && n.id !== sc.connect.from && !sc.validTargets.has(n.id)) ctx.globalAlpha = 0.35
    drawNode(ctx, rc, sc, n)
    ctx.globalAlpha = 1
  }
  for (const n of sc.diagram.nodes) {
    if (sc.handles.has(n.id)) drawHandle(ctx, sc, n)
  }
  if (sc.connect) {
    const from = nodes.get(sc.connect.from)
    if (from) {
      const fr = nodeRect(from, sc.reg)
      const target = sc.connect.target ? nodes.get(sc.connect.target) : undefined
      const end = target ? boundaryPoint(nodeRect(target, sc.reg), center(fr), 6) : sc.connect.to
      const start = boundaryPoint(fr, end, 4)
      ctx.save()
      ctx.setLineDash([7, 6])
      ctx.strokeStyle = target ? sc.palette.ok : sc.palette.accent
      ctx.lineWidth = 2.2
      ctx.beginPath()
      ctx.moveTo(start.x, start.y)
      ctx.lineTo(end.x, end.y)
      ctx.stroke()
      ctx.restore()
    }
  }
  drawNotes(ctx, sc)
  if (sc.marquee) {
    const m = sc.marquee
    ctx.fillStyle = sc.palette.accent
    ctx.globalAlpha = 0.1
    ctx.fillRect(m.x, m.y, m.w, m.h)
    ctx.globalAlpha = 1
    ctx.strokeStyle = sc.palette.accent
    ctx.lineWidth = 1
    ctx.strokeRect(m.x, m.y, m.w, m.h)
  }
  ctx.restore()
}
