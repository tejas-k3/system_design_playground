import { useEffect } from 'react'
import { Announcer } from './ui/Announcer'
import { Board } from './ui/Board'
import { Charts } from './ui/Charts'
import { InterviewBar } from './ui/interview/InterviewBar'
import { Inspector } from './ui/Inspector'
import { Library } from './ui/Library'
import { ScenarioPanel } from './ui/ScenarioPanel'
import { Toasts } from './ui/Toasts'
import { Toolbar } from './ui/Toolbar'
import { Welcome } from './ui/Welcome'
import { togglePlay } from './sim/bridge'
import { parseHash } from './model/links'
import { openDesign } from './sim/designs'
import { loadScenario } from './sim/loadScenario'
import { useStore } from './state/store'
import { Deferred, usePreloadSurfaces } from './lazySurfaces'
import { DesignName } from './ui/DesignName'

export function App() {
  const registryReady = useStore((s) => !!s.reg)
  usePreloadSurfaces()
  const ui = useStore((s) => s.ui)
  const hasResult = useStore((s) => !!s.scenario?.result)
  const hasSession = useStore((s) => !!s.interview?.session)
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

  const present = ui.present && hasSession
  const scenarioOpen = useStore((s) => !!s.scenario)
  return (
    <div className={`app ${present ? 'present-mode' : ''}`}>
      <a className="skip" href="#canvas-area" onClick={(e) => { e.preventDefault(); document.getElementById('canvas-area')?.focus() }}>Skip to the canvas</a>
      <Toolbar />
      <InterviewBar />
      <Deferred name="present" open={present} />
      <div className="main">
        <Library />
        <div className="center-col">
          <div className="board-wrap">
            <ScenarioPanel />
            <div className="board-main" id="canvas-area" tabIndex={-1}>
              {registryReady && <Board />}
              <Welcome />
            </div>
          </div>
          <Charts />
        </div>
        <Inspector />
      </div>
      <footer className="app-credit">{!scenarioOpen && <DesignName />}<span className="credit-text">Made by <b>Tejas Kothari</b> · MIT licensed</span></footer>
      <Deferred name="picker" open={ui.picker} />
      <Deferred name="result" open={ui.result && hasResult} />
      <Deferred name="help" open={ui.help} />
      <Deferred name="capacity" open={ui.capacity} />
      <Deferred name="search" open={ui.search} />
      <Deferred name="templates" open={ui.templates} />
      <Deferred name="skills" open={ui.skills} />
      <Deferred name="compare" open={ui.compare} />
      <Deferred name="question" open={ui.question !== null} />
      <Toasts />
      <Announcer />
    </div>
  )
}
