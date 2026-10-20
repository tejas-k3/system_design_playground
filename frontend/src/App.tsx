import { useEffect } from 'react'
import { Announcer } from './ui/Announcer'
import { Board } from './ui/Board'
import { Charts } from './ui/Charts'
import { QuestionDialog } from './ui/QuestionDialog'
import { PresentView } from './ui/interview/PresentView'
import { InterviewBar } from './ui/interview/InterviewBar'
import { CompareDialog } from './ui/CompareDialog'
import { CapacityDialog } from './ui/CapacityDialog'
import { SearchDialog } from './ui/SearchDialog'
import { TemplatesDialog } from './ui/TemplatesDialog'
import { SkillMap } from './ui/SkillMap'
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

  const present = useStore((s) => s.ui.present && !!s.interview?.session)
  return (
    <div className={`app ${present ? 'present-mode' : ''}`}>
      <a className="skip" href="#canvas-area" onClick={(e) => { e.preventDefault(); document.getElementById('canvas-area')?.focus() }}>Skip to the canvas</a>
      <Toolbar />
      <InterviewBar />
      <PresentView />
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
      <ScenarioPicker />
      <ResultModal />
      <Help />
      <CapacityDialog />
      <SearchDialog />
      <TemplatesDialog />
      <SkillMap />
      <CompareDialog />
      <QuestionDialog />
      <Toasts />
      <Announcer />
    </div>
  )
}
