import { useEffect } from 'react'
import { Board } from './ui/Board'
import { Charts } from './ui/Charts'
import { Inspector } from './ui/Inspector'
import { Library } from './ui/Library'
import { Help, ResultModal, ScenarioPanel, ScenarioPicker, Toasts, Welcome, loadScenario } from './ui/Scenario'
import { Toolbar } from './ui/Toolbar'
import { togglePlay } from './sim/bridge'
import { useStore } from './state/store'

export function App() {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target
      if (t instanceof HTMLElement && (['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON'].includes(t.tagName) || t.isContentEditable)) return
      if (e.code === 'Space') {
        e.preventDefault()
        togglePlay()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // deep link: /#challenge=<id>
  useEffect(() => {
    const m = /challenge=([a-z0-9-]+)/.exec(location.hash)
    if (m) loadScenario(m[1])
  }, [])

  const registryReady = useStore((s) => !!s.reg)
  return (
    <div className="app">
      <Toolbar />
      <div className="main">
        <Library />
        <div className="center-col">
          <div className="board-wrap">
            <ScenarioPanel />
            <div className="board-main">
              {registryReady && <Board />}
              <Welcome />
            </div>
          </div>
          <Charts />
        </div>
        <Inspector />
      </div>
      <ScenarioPicker />
      <ResultModal />
      <Help />
      <Toasts />
    </div>
  )
}
