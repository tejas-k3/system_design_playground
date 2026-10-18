import type { Diagram } from '../../api/types'
import { palettes } from '../../draw/theme'
import { createRough, render } from '../../draw/renderer'
import { nodeRect } from '../../draw/geometry'
import type { Reg } from '../../model/diagram'
import { emptyStats } from '../../state/store'

/** The design as a PNG, cropped to the components with a margin, on a light or dark background. Null for an empty canvas. */
export async function diagramPng(diagram: Diagram, reg: Reg, theme: 'light' | 'dark'): Promise<Blob | null> {
  if (diagram.nodes.length === 0) return null
  await document.fonts?.ready
  const rects = diagram.nodes.map((n) => nodeRect(n, reg))
  const x0 = Math.min(...rects.map((r) => r.x))
  const y0 = Math.min(...rects.map((r) => r.y))
  const x1 = Math.max(...rects.map((r) => r.x + r.w))
  const y1 = Math.max(...rects.map((r) => r.y + r.h))
  const pad = 40
  const w = Math.ceil(x1 - x0 + 2 * pad)
  const h = Math.ceil(y1 - y0 + 2 * pad)
  const dpr = 2
  const canvas = document.createElement('canvas')
  canvas.width = w * dpr
  canvas.height = h * dpr
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  render(ctx, createRough(canvas), w, h, dpr, {
    diagram, reg, stats: emptyStats(), running: false,
    viewport: { scrollX: pad - x0, scrollY: pad - y0, zoom: 1 },
    palette: palettes[theme], selection: new Set(), hover: null, handles: new Set(), connect: null, validTargets: new Set(), marquee: null,
    particles: [], now: 0, editingNote: null, showParticles: false,
  })
  return new Promise((resolve) => canvas.toBlob((b) => resolve(b), 'image/png'))
}

export function download(blob: Blob, name: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export const slug = (s: string): string => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40) || 'design'
