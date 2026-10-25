import type { Diagram } from '../../api/types'
import { palettes } from '../../draw/theme'
import { createRough, measureText, render } from '../../draw/renderer'
import { boundsOf } from '../../draw/geometry'
import { FONT } from '../../draw/theme'
import type { Reg } from '../../model/diagram'
import { emptyStats } from '../../state/store'

/** The design as a PNG, cropped to the components with a margin, on a light or dark background. Null for an empty canvas. */
export async function diagramPng(diagram: Diagram, reg: Reg, theme: 'light' | 'dark'): Promise<Blob | null> {
  if (diagram.nodes.length === 0) return null
  await document.fonts?.ready
  // the picture covers everything that is drawn (boxes and notes too), as the canvas shows it, so a collapsed box is its chip
  const b = boundsOf(diagram, reg, measureText)
  if (!b) return null
  const x0 = b.x
  const y0 = b.y
  const pad = 40
  const head = diagram.name ? 34 : 0 // room for the design's name above the picture
  const w = Math.ceil(b.w + 2 * pad)
  const h = Math.ceil(b.h + 2 * pad + head)
  const dpr = 2
  const canvas = document.createElement('canvas')
  canvas.width = w * dpr
  canvas.height = h * dpr
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  render(ctx, createRough(canvas), w, h, dpr, {
    diagram, reg, stats: emptyStats(), running: false,
    viewport: { scrollX: pad - x0, scrollY: pad + head - y0, zoom: 1 },
    palette: palettes[theme], selection: new Set(), hover: null, handles: new Set(), connect: null, validTargets: new Set(), marquee: null,
    particles: [], now: 0, editingNote: null, showParticles: false,
  })
  if (diagram.name) {
    ctx.fillStyle = palettes[theme].ink
    ctx.font = `bold 20px ${FONT}`
    ctx.textAlign = 'left'
    ctx.textBaseline = 'alphabetic'
    ctx.fillText(diagram.name, pad, 34)
  }
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
