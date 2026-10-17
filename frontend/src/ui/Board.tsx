import { useCallback, useEffect, useRef, useState } from 'react'
import { type ActiveParticle, PARTICLE_MS, createRough, measureText, render } from '../draw/renderer'
import { type Pt, type Rect, hitTest, nodeRect, noteRect, rectsIntersect, screenToWorld } from '../draw/geometry'
import { palettes } from '../draw/theme'
import { drainParticles } from '../sim/particles'
import { useStore } from '../state/store'
import { NodeOutline } from './board/NodeOutline'
import { NoteEditor, type NoteEdit } from './board/NoteEditor'
import { type PinchState, pinchOf, pinchStep } from './board/pinch'
import { useBoardShortcuts } from './board/useBoardShortcuts'
import { useCanvasSize } from './board/useCanvasSize'
import { useFitView } from './board/useFitView'
import { useWheelZoom } from './board/useWheelZoom'
import { ZoomBadge } from './board/ZoomBadge'

type Mode =
  | { k: 'idle' }
  | { k: 'pan'; sx: number; sy: number; moved: boolean }
  | { k: 'drag'; last: Pt; ids: string[] }
  | { k: 'connect'; from: string; downAt: Pt; moved: boolean }
  | { k: 'marquee'; start: Pt; additive: boolean; base: string[] }

export function Board() {
  const wrap = useRef<HTMLDivElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const mode = useRef<Mode>({ k: 'idle' })
  const hover = useRef<string | null>(null)
  const pointer = useRef<Pt>({ x: 0, y: 0 })
  const connectTo = useRef<{ target: string | null; pending: string | null }>({ target: null, pending: null })
  const marquee = useRef<Rect | null>(null)
  const touches = useRef(new Map<number, Pt>()) // active pointers in screen space, for pinch
  const pinch = useRef<PinchState | null>(null)
  const particles = useRef<ActiveParticle[]>([])
  const raf = useRef(0)
  const [noteEdit, setNoteEdit] = useState<NoteEdit | null>(null)
  const [cursor, setCursor] = useState('default')

  const draw = useCallback(() => {
    raf.current = 0
    const cv = canvas.current
    const st = useStore.getState()
    if (!cv || !st.reg) return
    const ctx = cv.getContext('2d')!
    const rc = createRough(cv)
    const now = performance.now()
    const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches
    for (const p of drainParticles()) if (!reduced) particles.current.push({ edge: p.e, back: !!p.b, fail: !!p.f, t0: now })
    if (particles.current.length > 700) particles.current = particles.current.slice(-700)
    particles.current = particles.current.filter((p) => now - p.t0 < PARTICLE_MS)

    const conn = mode.current.k === 'connect' ? mode.current.from : connectTo.current.pending
    const byId = new Map(st.diagram.nodes.map((n) => [n.id, n]))
    const valid = new Set<string>()
    if (conn) {
      const src = byId.get(conn)
      if (src) for (const n of st.diagram.nodes) if (n.id !== conn && st.reg.canConnect(src.kind, n.kind)) valid.add(n.id)
    }
    render(ctx, rc, size.current.w, size.current.h, size.current.dpr, {
      diagram: st.diagram,
      reg: st.reg,
      stats: st.stats,
      running: st.sim.status !== 'stopped',
      viewport: st.viewport,
      palette: palettes[st.theme],
      selection: new Set(st.selection),
      hover: hover.current,
      connect: conn ? { from: conn, to: screenToWorld(pointer.current, st.viewport), target: connectTo.current.target } : null,
      validTargets: valid,
      marquee: marquee.current,
      particles: particles.current,
      now,
      editingNote: noteEdit?.id ?? null,
      showParticles: true,
    })
    if (particles.current.length > 0 || st.sim.status === 'running') raf.current = requestAnimationFrame(draw)
  }, [noteEdit])

  const wake = useCallback(() => {
    if (!raf.current) raf.current = requestAnimationFrame(draw)
  }, [draw])

  // keep a stable handle for subscribers
  const wakeRef = useRef(wake)
  wakeRef.current = wake

  useEffect(() => {
    const unsub = useStore.subscribe(() => wakeRef.current())
    return () => {
      unsub()
      cancelAnimationFrame(raf.current)
      raf.current = 0
    }
  }, [])
  useEffect(() => wake(), [wake])

  const size = useCanvasSize(wrap, canvas, () => wakeRef.current())
  const fitView = useFitView(size)

  // ---- pointer handling -------------------------------------------------------------

  const local = (e: { clientX: number; clientY: number }): Pt => {
    const r = canvas.current!.getBoundingClientRect()
    return { x: e.clientX - r.left, y: e.clientY - r.top }
  }
  const world = (p: Pt) => screenToWorld(p, useStore.getState().viewport)

  const hitAt = (w: Pt) => {
    const st = useStore.getState()
    const handles = new Set<string>(st.selection.filter((id) => st.diagram.nodes.some((n) => n.id === id)))
    if (hover.current && st.diagram.nodes.some((n) => n.id === hover.current)) handles.add(hover.current)
    return hitTest(w, st.diagram, st.reg!, st.viewport.zoom, measureText, handles)
  }

  const finishConnect = (from: string, to: string | null) => {
    const st = useStore.getState()
    connectTo.current = { target: null, pending: null }
    if (to && to !== from) st.addEdge(from, to)
    mode.current = { k: 'idle' }
    wake()
  }

  const onPointerDown = (e: React.PointerEvent) => {
    if (noteEdit) return
    const st = useStore.getState()
    if (!st.reg) return
    canvas.current!.setPointerCapture(e.pointerId)
    const sp = local(e)
    pointer.current = sp
    touches.current.set(e.pointerId, sp)
    if (touches.current.size === 2) {
      // a second finger turns whatever was happening into a pinch: drop any drag or connection
      if (mode.current.k === 'drag') st.endGesture()
      mode.current = { k: 'idle' }
      connectTo.current = { target: null, pending: null }
      const [a, b] = [...touches.current.values()]
      pinch.current = pinchOf(a, b)
      return
    }
    const w = world(sp)
    if (e.button === 1 || e.button === 2) {
      mode.current = { k: 'pan', sx: sp.x, sy: sp.y, moved: false }
      setCursor('grabbing')
      return
    }
    if (e.button !== 0) return
    const hit = hitAt(w)

    // click-click connect: second click completes
    if (connectTo.current.pending) {
      const from = connectTo.current.pending
      finishConnect(from, hit?.type === 'node' ? hit.id : null)
      return
    }
    if (st.tool === 'connect') {
      if (hit && (hit.type === 'node' || hit.type === 'handle')) {
        mode.current = { k: 'connect', from: hit.id, downAt: sp, moved: false }
      } else mode.current = { k: 'pan', sx: sp.x, sy: sp.y, moved: false }
      wake()
      return
    }
    if (hit?.type === 'handle') {
      mode.current = { k: 'connect', from: hit.id, downAt: sp, moved: false }
      wake()
      return
    }
    if (hit && (hit.type === 'node' || hit.type === 'note')) {
      if (e.shiftKey) {
        st.setSelection(st.selection.includes(hit.id) ? st.selection.filter((i) => i !== hit.id) : [...st.selection, hit.id])
      } else if (!st.selection.includes(hit.id)) st.setSelection([hit.id])
      const ids = useStore.getState().selection.filter((id) => st.diagram.nodes.some((n) => n.id === id) || st.diagram.notes.some((n) => n.id === id))
      st.beginGesture()
      mode.current = { k: 'drag', last: w, ids }
      setCursor('grabbing')
      return
    }
    if (hit?.type === 'edge') {
      st.setSelection(e.shiftKey ? [...st.selection, hit.id] : [hit.id])
      return
    }
    if (e.shiftKey) {
      mode.current = { k: 'marquee', start: w, additive: true, base: st.selection }
      return
    }
    mode.current = { k: 'pan', sx: sp.x, sy: sp.y, moved: false }
  }

  const onPointerMove = (e: React.PointerEvent) => {
    const st = useStore.getState()
    if (!st.reg) return
    const sp = local(e)
    pointer.current = sp
    if (touches.current.has(e.pointerId)) touches.current.set(e.pointerId, sp)
    if (pinch.current && touches.current.size === 2) {
      const [a, b] = [...touches.current.values()]
      const next = pinchOf(a, b)
      const { dx, dy, factor } = pinchStep(pinch.current, next)
      const v = st.viewport
      st.setViewport({ scrollX: v.scrollX + dx, scrollY: v.scrollY + dy })
      if (factor !== 1) st.zoomAt(factor, next.mid.x, next.mid.y)
      pinch.current = next
      return
    }
    const w = world(sp)
    const m = mode.current
    switch (m.k) {
      case 'pan': {
        const dx = sp.x - m.sx
        const dy = sp.y - m.sy
        if (Math.abs(dx) + Math.abs(dy) > 2) m.moved = true
        m.sx = sp.x
        m.sy = sp.y
        st.setViewport({ scrollX: st.viewport.scrollX + dx, scrollY: st.viewport.scrollY + dy })
        return
      }
      case 'drag': {
        st.moveNodes(m.ids, w.x - m.last.x, w.y - m.last.y)
        m.last = w
        return
      }
      case 'connect': {
        if (Math.hypot(sp.x - m.downAt.x, sp.y - m.downAt.y) > 4) m.moved = true
        const hit = hitAt(w)
        const src = st.diagram.nodes.find((n) => n.id === m.from)
        const tgt = hit?.type === 'node' ? st.diagram.nodes.find((n) => n.id === hit.id) : undefined
        connectTo.current.target = src && tgt && tgt.id !== src.id && st.reg.canConnect(src.kind, tgt.kind) ? tgt.id : null
        wake()
        return
      }
      case 'marquee': {
        const r: Rect = { x: Math.min(m.start.x, w.x), y: Math.min(m.start.y, w.y), w: Math.abs(w.x - m.start.x), h: Math.abs(w.y - m.start.y) }
        marquee.current = r
        const ids = [
          ...st.diagram.nodes.filter((n) => rectsIntersect(r, nodeRect(n, st.reg!))).map((n) => n.id),
          ...st.diagram.notes.filter((n) => rectsIntersect(r, noteRect(n, measureText))).map((n) => n.id),
        ]
        st.setSelection(Array.from(new Set([...(m.additive ? m.base : []), ...ids])))
        wake()
        return
      }
    }
    // idle: hover feedback
    const hit = hitAt(w)
    const id = hit && hit.type !== 'handle' ? hit.id : hit?.type === 'handle' ? hit.id : null
    if (id !== hover.current) {
      hover.current = id
      wake()
    }
    if (connectTo.current.pending) {
      const src = st.diagram.nodes.find((n) => n.id === connectTo.current.pending)
      const tgt = hit?.type === 'node' ? st.diagram.nodes.find((n) => n.id === hit.id) : undefined
      connectTo.current.target = src && tgt && tgt.id !== src.id && st.reg.canConnect(src.kind, tgt.kind) ? tgt.id : null
      wake()
    }
    setCursor(hit?.type === 'handle' ? 'crosshair' : hit ? 'pointer' : st.tool === 'connect' ? 'crosshair' : 'default')
  }

  const onPointerUp = (e: React.PointerEvent) => {
    const st = useStore.getState()
    const m = mode.current
    canvas.current?.releasePointerCapture(e.pointerId)
    touches.current.delete(e.pointerId)
    if (pinch.current) {
      // the pinch ends when a finger lifts; the remaining finger must not start dragging or panning
      if (touches.current.size < 2) pinch.current = null
      mode.current = { k: 'idle' }
      setCursor('default')
      return
    }
    if (m.k === 'drag') st.endGesture()
    if (m.k === 'pan' && !m.moved) st.setSelection([])
    if (m.k === 'connect') {
      if (m.moved) {
        finishConnect(m.from, connectTo.current.target)
        setCursor('default')
        return
      }
      if (st.tool === 'connect') {
        // plain click on a node: wait for the second click
        connectTo.current = { target: null, pending: m.from }
        mode.current = { k: 'idle' }
        wake()
        return
      }
      mode.current = { k: 'idle' }
      st.setSelection([m.from])
    }
    marquee.current = null
    mode.current = { k: 'idle' }
    setCursor('default')
    wake()
  }

  const beginNoteEdit = (id: string) => {
    const st = useStore.getState()
    const n = st.diagram.notes.find((x) => x.id === id)
    if (!n) return
    const r = noteRect(n, measureText)
    const v = st.viewport
    setNoteEdit({ id, left: r.x * v.zoom + v.scrollX, top: r.y * v.zoom + v.scrollY, w: Math.max(160, r.w * v.zoom), size: (n.fontSize ?? 18) * v.zoom })
  }

  const onDoubleClick = (e: React.MouseEvent) => {
    const st = useStore.getState()
    if (!st.reg) return
    const w = world(local(e))
    const hit = hitAt(w)
    if (hit?.type === 'note') beginNoteEdit(hit.id)
    else if (!hit) {
      const id = st.addNote(w.x - 20, w.y - 14)
      setTimeout(() => beginNoteEdit(id), 0)
    }
  }

  const commitNote = (text: string) => {
    const st = useStore.getState()
    if (!noteEdit) return
    const id = noteEdit.id
    setNoteEdit(null)
    if (text.trim() === '') st.deleteIds([id])
    else st.updateNote(id, { text })
  }

  const zoomBy = useCallback((f: number) => useStore.getState().zoomAt(f, size.current.w / 2, size.current.h / 2), [])
  const cancelGesture = useCallback(() => {
    connectTo.current = { target: null, pending: null }
    mode.current = { k: 'idle' }
    wake()
  }, [wake])
  useWheelZoom(canvas)
  useBoardShortcuts({ fit: fitView, zoomBy, cancel: cancelGesture })

  // ---- drag & drop from the library ----------------------------------------------------

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const kind = e.dataTransfer.getData('application/x-sdp-kind')
    const st = useStore.getState()
    const spec = st.reg?.spec(kind)
    if (!spec) return
    const w = world(local(e))
    st.addNode(kind, w.x - spec.width / 2, w.y - spec.height / 2)
  }

  return (
    <div
      ref={wrap}
      className="board"
      onDragOver={(e) => e.dataTransfer.types.includes('application/x-sdp-kind') && e.preventDefault()}
      onDrop={onDrop}
    >
      <canvas
        ref={canvas}
        style={{ cursor, touchAction: 'none' }}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onDoubleClick={onDoubleClick}
        onContextMenu={(e) => e.preventDefault()}
        aria-label="System design canvas"
      />
      {noteEdit && (
        <NoteEditor edit={noteEdit} initialText={useStore.getState().diagram.notes.find((n) => n.id === noteEdit.id)?.text ?? ''} onCommit={commitNote} />
      )}
      <NodeOutline />
      <ZoomBadge fit={fitView} by={zoomBy} />
    </div>
  )
}
