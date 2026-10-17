import { useEffect } from 'react'
import { Board } from './ui/Board'
import { Charts } from './ui/Charts'
import { Inspector } from './ui/Inspector'
import { Help } from './ui/Help'
import { Library } from './ui/Library'
import { ResultModal } from './ui/ResultModal'
import { ScenarioPanel } from './ui/ScenarioPanel'
import { ScenarioPicker } from './ui/ScenarioPicker'
import { Toasts } from './ui/Toasts'
import { Toolbar } from './ui/Toolbar'
import { Welcome } from './ui/Welcome'
import { togglePlay } from './sim/bridge'
import { parseHash } from './model/links'
import { openDesign } from './sim/designs'
import { loadScenario } from './sim/loadScenario'
import { useStore } from './state/store'

export function App() {
  const registryReady = useStore((s) => !!s.reg)
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

  // deep links: /#challenge=<id> and /#design=<id>, on load and when the hash changes in an open tab
  // (the registry must be ready before a design can be validated, hence the dependency)
  useEffect(() => {
    const open = () => {
      const link = parseHash(location.hash)
      if (link?.kind === 'challenge') loadScenario(link.id)
      if (link?.kind === 'design') {
        history.replaceState(null, '', location.pathname + location.search) // a reload must not re-apply the link
        openDesign(link.id)
      }
    }
    open()
    window.addEventListener('hashchange', open)
    return () => window.removeEventListener('hashchange', open)
  }, [registryReady])

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
