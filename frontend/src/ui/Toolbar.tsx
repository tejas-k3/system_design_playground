import { fmtCost } from '../model/goals'
import { reset, step, togglePlay } from '../sim/bridge'
import { shareCurrentDesign } from '../sim/designs'
import { useStore } from '../state/store'

export function Toolbar() {
  const tool = useStore((s) => s.tool)
  const setTool = useStore((s) => s.setTool)
  const speeds = useStore((s) => s.reg?.speeds ?? [1])
  const status = useStore((s) => s.sim.status)
  const speed = useStore((s) => s.sim.speed)
  const setSim = useStore((s) => s.setSim)
  const now = useStore((s) => s.stats.global.now)
  const cost = useStore((s) => s.cost)
  const nodeCount = useStore((s) => s.diagram.nodes.length)
  const undo = useStore((s) => s.undo)
  const redo = useStore((s) => s.redo)
  const canUndo = useStore((s) => s.past.length > 0)
  const canRedo = useStore((s) => s.future.length > 0)
  const theme = useStore((s) => s.theme)
  const setTheme = useStore((s) => s.setTheme)
  const setUi = useStore((s) => s.setUi)
  const ui = useStore((s) => s.ui)
  const connection = useStore((s) => s.connection)
  const requestFit = useStore((s) => s.requestFit)
  const running = status === 'running'

  return (
    <header className="toolbar">
      <div className="brand" title="System Design Playground">
        <svg width="26" height="26" viewBox="0 0 32 32" aria-hidden>
          <rect x="3" y="3" width="26" height="26" rx="6" fill="none" stroke="currentColor" strokeWidth="2" />
          <circle cx="10" cy="11" r="3" fill="#1971c2" />
          <circle cx="22" cy="11" r="3" fill="#e8590c" />
          <circle cx="16" cy="22" r="3" fill="#2f9e44" />
          <path d="M12 12.5 15 19M20 12.5 17 19M13 11h6" stroke="currentColor" strokeWidth="1.6" fill="none" strokeLinecap="round" />
        </svg>
        <span className="brand-name">Playground</span>
      </div>

      <div className="group" role="group" aria-label="Tools">
        <button className={tool === 'select' ? 'on' : ''} onClick={() => setTool('select')} title="Select / move (V)" aria-pressed={tool === 'select'}>
          <Icon d="M5 3l14 8-6 2-2 6z" />
        </button>
        <button className={tool === 'connect' ? 'on' : ''} onClick={() => setTool('connect')} title="Connect nodes (C)" aria-pressed={tool === 'connect'}>
          <Icon d="M5 19L19 5M19 5h-7M19 5v7" />
        </button>
        <button onClick={undo} disabled={!canUndo} title="Undo (Ctrl/⌘ Z)" aria-label="Undo">
          <Icon d="M9 14L4 9l5-5M4 9h10a6 6 0 010 12h-3" />
        </button>
        <button onClick={redo} disabled={!canRedo} title="Redo (Ctrl/⌘ Shift Z)" aria-label="Redo">
          <Icon d="M15 14l5-5-5-5M20 9H10a6 6 0 000 12h3" />
        </button>
        <button onClick={requestFit} title="Fit to content (F)" aria-label="Fit to content">
          <Icon d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5" />
        </button>
      </div>

      <div className="group sim" role="group" aria-label="Simulation">
        <button className={`play ${running ? 'on' : ''}`} onClick={togglePlay} title="Play / pause (Space)" aria-label={running ? 'Pause' : 'Play'}>
          {running ? <Icon d="M8 5v14M16 5v14" /> : <Icon d="M7 4l13 8-13 8z" fill />}
          <span>{running ? 'Pause' : status === 'paused' ? 'Resume' : 'Play'}</span>
        </button>
        <button onClick={step} disabled={running} title="Step 100 ms" aria-label="Step">
          <Icon d="M5 5l9 7-9 7zM18 5v14" />
        </button>
        <button onClick={reset} disabled={status === 'stopped'} title="Reset the run" aria-label="Reset">
          <Icon d="M4 12a8 8 0 108-8M4 4v5h5" />
        </button>
        <label className="speed">
          <span className="sr">Speed</span>
          <select value={speed} onChange={(e) => setSim({ speed: Number(e.target.value) })} aria-label="Simulation speed">
            {speeds.map((s) => (
              <option key={s} value={s}>{s}×</option>
            ))}
          </select>
        </label>
        <span className="clock" title="Simulated time">{formatClock(now)}</span>
      </div>

      <div className="spacer" />
      {nodeCount > 0 && <span className="pill" title="Estimated hourly cost of this design">{fmtCost(cost)}</span>}
      <button onClick={shareCurrentDesign} title="Copy a link to this design" aria-label="Share a link to this design">
        <Icon d="M4 12v7a1 1 0 001 1h14a1 1 0 001-1v-7M16 6l-4-4-4 4M12 2v13" />
      </button>
      <button className="wide" onClick={() => setUi({ picker: true })}>
        Challenges
      </button>
      <button onClick={() => setUi({ library: !ui.library })} title="Toggle library" aria-label="Toggle library" className={ui.library ? 'on' : ''}>
        <Icon d="M4 6h16M4 12h16M4 18h16" />
      </button>
      <button onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')} title="Toggle theme" aria-label="Toggle theme">
        <Icon d={theme === 'dark' ? 'M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6l1.4 1.4M17 17l1.4 1.4M5.6 18.4L7 17M17 7l1.4-1.4M12 8a4 4 0 100 8 4 4 0 000-8z' : 'M20 14A8 8 0 1110 4a6 6 0 0010 10z'} />
      </button>
      <button onClick={() => setUi({ help: true })} title="Help & shortcuts" aria-label="Help">
        <Icon d="M9.5 9a2.5 2.5 0 114 2c-1 .7-1.5 1.2-1.5 2.5M12 17.5v.5M12 3a9 9 0 100 18 9 9 0 000-18z" />
      </button>
      <span className={`dot ${connection}`} title={connection === 'open' ? 'Connected to the simulator' : connection === 'connecting' ? 'Connecting…' : 'Disconnected, retrying'} />
    </header>
  )
}

export function formatClock(sec: number): string {
  const m = Math.floor(sec / 60)
  const s = sec - m * 60
  return `${m}:${s.toFixed(1).padStart(4, '0')}`
}

export function Icon({ d, fill }: { d: string; fill?: boolean }) {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill={fill ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d={d} />
    </svg>
  )
}
