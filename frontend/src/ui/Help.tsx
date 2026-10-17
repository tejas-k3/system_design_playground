import { useStore } from '../state/store'
import { useEscapeKey } from './useEscapeKey'

export function Help() {
  const open = useStore((s) => s.ui.help)
  const setUi = useStore((s) => s.setUi)
  useEscapeKey(open, () => setUi({ help: false }))
  if (!open) return null
  const keys: [string, string][] = [
    ['Space', 'Play / pause'],
    ['V · C', 'Select tool · Connect tool'],
    ['Delete', 'Delete selection'],
    ['Arrow keys', 'Move the selection (hold Shift for bigger steps)'],
    ['Tab, then Enter', 'Pick a component without a mouse; connect it from the inspector'],
    ['Pinch', 'Zoom and pan on a touch screen'],
    ['Ctrl/⌘ Z · Shift+Z', 'Undo · Redo'],
    ['F', 'Fit the design on screen'],
    ['+ / −  ·  scroll', 'Zoom'],
    ['Drag empty space', 'Pan the canvas'],
    ['Shift + drag', 'Box-select'],
    ['Double-click empty', 'Add a note'],
    ['Esc', 'Clear selection / cancel connecting'],
  ]
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Help" onClick={() => setUi({ help: false })}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>How it works</h2><button className="x" autoFocus onClick={() => setUi({ help: false })} aria-label="Close">×</button></header>
        <ol className="how">
          <li><b>Build.</b> Add users, servers, caches, databases, queues… and draw connections (drag from the round handle on a component's right edge).</li>
          <li><b>Run.</b> Press Play. Requests flow along the connections; bars show how busy each part is and why requests fail.</li>
          <li><b>Break it.</b> Select a component and press "Crash it", or raise the traffic, and watch what gives way first.</li>
          <li><b>Fix it.</b> Change settings or structure live. The simulation keeps running while you edit.</li>
        </ol>
        <h3>Shortcuts</h3>
        <dl className="kv keys">{keys.map(([k, v]) => (<><dt key={k}><kbd>{k}</kbd></dt><dd key={v}>{v}</dd></>))}</dl>
        <p className="muted small">Runs are deterministic: the same design and seed always produce the same result.</p>
      </div>
    </div>
  )
}
