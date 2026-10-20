import { useEffect, useState } from 'react'
import { fetchScenarios } from '../api/client'
import type { ScenarioSummary } from '../api/types'
import { dailyChallenge, loadStreak, localDate, shownStreak } from '../model/daily'
import { exampleDiagram } from '../model/examples'
import { loadScenario } from '../sim/loadScenario'
import { useStore } from '../state/store'

export function Welcome() {
  const show = useStore((s) => s.ui.welcome && !s.scenario && s.diagram.nodes.length === 0)
  const loadDiagram = useStore((s) => s.loadDiagram)
  const setUi = useStore((s) => s.setUi)
  const requestFit = useStore((s) => s.requestFit)
  const startInterview = useStore((s) => s.startInterview)
  const [daily, setDaily] = useState<{ s: ScenarioSummary; streak: number } | null>(null)
  useEffect(() => {
    if (!show) return
    const today = localDate()
    fetchScenarios()
      .then((list) => {
        const id = dailyChallenge(today, list.map((x) => x.id))
        const s = list.find((x) => x.id === id)
        setDaily(s ? { s, streak: shownStreak(loadStreak(), today) } : null)
      })
      .catch(() => setDaily(null))
  }, [show])
  if (!show) return null
  return (
    <div className="welcome">
      <h1>Sketch a system. Send traffic. Watch it break.</h1>
      <p>Drag components from the left onto the canvas, wire them together, then press Play.</p>
      <div className="row-btns center">
        <button className="primary" onClick={() => { loadDiagram(exampleDiagram()); setUi({ welcome: false }); requestFit() }}>Load an example</button>
        <button onClick={() => setUi({ templates: true })}>Browse templates</button>
        <button onClick={() => setUi({ picker: true })}>Start a challenge</button>
        <button onClick={() => setUi({ welcome: false })}>Start blank</button>
      </div>
      {daily && (
        <div className="daily" role="group" aria-label="Daily challenge">
          <span><b>Today's challenge:</b> {daily.s.title}</span>
          <span className="muted small">{daily.streak > 0 ? `${daily.streak}-day streak` : 'Solve it to start a streak'}</span>
          <button onClick={() => { setUi({ welcome: false }); loadScenario(daily.s.id) }}>Start today's challenge</button>
        </div>
      )}
      <div className="row-btns center">
        <button onClick={() => startInterview(45)} title="A 45-minute mock interview in seven stages, with a timer">Practise an interview</button>
      </div>
    </div>
  )
}
