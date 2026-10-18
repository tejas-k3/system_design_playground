import { nodeRect, rectsIntersect } from '../draw/geometry'
import { groupKinds } from '../model/library'
import { useStore } from '../state/store'

export function Library() {
  const reg = useStore((s) => s.reg)
  const open = useStore((s) => s.ui.library)
  const addNode = useStore((s) => s.addNode)
  const viewport = useStore((s) => s.viewport)
  const configOnly = useStore((s) => !!s.scenario?.def.configOnly)
  if (!reg || !open) return null

  const addAtCenter = (kind: string) => {
    const canvas = document.querySelector<HTMLCanvasElement>('.board canvas')
    const r = canvas?.getBoundingClientRect()
    const cx = ((r?.width ?? 800) / 2 - viewport.scrollX) / viewport.zoom
    const cy = ((r?.height ?? 500) / 2 - viewport.scrollY) / viewport.zoom
    const spec = reg.spec(kind)!
    // pick the free spot closest to the middle of the view so new components never stack
    const taken = useStore.getState().diagram.nodes.map((n) => nodeRect(n, reg))
    let best = { x: cx - spec.width / 2, y: cy - spec.height / 2 }
    let bestDist = Infinity
    for (let gx = -4; gx <= 4; gx++) {
      for (let gy = -3; gy <= 3; gy++) {
        const cand = { x: cx - spec.width / 2 + gx * (spec.width + 40), y: cy - spec.height / 2 + gy * (spec.height + 40), w: spec.width, h: spec.height }
        if (taken.some((t) => rectsIntersect(cand, { x: t.x - 12, y: t.y - 12, w: t.w + 24, h: t.h + 24 }))) continue
        const d = Math.hypot(gx, gy)
        if (d < bestDist) {
          bestDist = d
          best = { x: cand.x, y: cand.y }
        }
      }
    }
    addNode(kind, best.x, best.y)
  }

  return (
    <aside className="library" aria-label="Component library">
      <h2>Components</h2>
      {configOnly && <p className="hint-box">This challenge is configuration-only: select a node and tune its settings.</p>}
      {groupKinds(reg.nodes.map((n) => n.kind)).map((g) => (
        <section key={g.title} aria-label={g.title}>
          <h3>{g.title}</h3>
          <ul>
            {g.kinds.map((kind) => {
              const n = reg.spec(kind)!
              return (
                <li key={kind}>
                  <button
                    className="lib-item"
                    draggable={!configOnly}
                    disabled={configOnly}
                    onDragStart={(e) => {
                      e.dataTransfer.setData('application/x-sdp-kind', n.kind)
                      e.dataTransfer.effectAllowed = 'copy'
                    }}
                    onClick={() => addAtCenter(n.kind)}
                    title={n.description}
                  >
                    <span className="swatch" style={{ background: n.accent }} />
                    <span className="lib-text">
                      <b>{n.name}</b>
                      <small>{n.description}</small>
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        </section>
      ))}
      <button className="linkish" onClick={() => useStore.getState().setUi({ templates: true })} disabled={configOnly}>Starter templates…</button>
      <p className="muted small">Click or drag onto the canvas. Drag from a node's right-hand handle to connect.</p>
    </aside>
  )
}
