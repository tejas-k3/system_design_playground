import { exampleDiagram } from '../model/examples'
import { useStore } from '../state/store'

export function Welcome() {
  const show = useStore((s) => s.ui.welcome && !s.scenario && s.diagram.nodes.length === 0)
  const loadDiagram = useStore((s) => s.loadDiagram)
  const setUi = useStore((s) => s.setUi)
  const requestFit = useStore((s) => s.requestFit)
  const startInterview = useStore((s) => s.startInterview)
  if (!show) return null
  return (
    <div className="welcome">
      <h1>Sketch a system. Send traffic. Watch it break.</h1>
      <p>Drag components from the left onto the canvas, wire them together, then press Play.</p>
      <div className="row-btns center">
        <button className="primary" onClick={() => { loadDiagram(exampleDiagram()); setUi({ welcome: false }); requestFit() }}>Load an example</button>
        <button onClick={() => setUi({ picker: true })}>Start a challenge</button>
        <button onClick={() => setUi({ welcome: false })}>Start blank</button>
      </div>
      <div className="row-btns center">
        <button onClick={() => startInterview(45)} title="A 45-minute mock interview in seven stages, with a timer">Practise an interview</button>
      </div>
    </div>
  )
}
