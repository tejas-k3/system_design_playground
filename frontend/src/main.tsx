import '@fontsource-variable/fraunces/index.css'
import '@fontsource-variable/fraunces/wght-italic.css'
import '@fontsource-variable/bricolage-grotesque/index.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/600.css'
import '@fontsource/patrick-hand/400.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { fetchRegistry } from './api/client'
import { Reg } from './model/diagram'
import { startBridge } from './sim/bridge'
import { startPersistence } from './state/persist'
import { startDailyWatch } from './state/dailyWatch'
import { useStore } from './state/store'
import './styles.css'

async function boot() {
  const root = createRoot(document.getElementById('root')!)
  try {
    const reg = new Reg(await fetchRegistry())
    useStore.getState().setReg(reg)
    startPersistence(reg)
    startDailyWatch()
    startBridge()
    root.render(
      <StrictMode>
        <App />
      </StrictMode>,
    )
  } catch (e) {
    root.render(
      <div style={{ padding: 32, fontFamily: 'system-ui' }}>
        <h1>Can't reach the simulator</h1>
        <p>Start the Go server (<code>make dev</code> or <code>go run ./cmd/server</code> in <code>backend/</code>) and reload.</p>
        <pre>{String(e)}</pre>
      </div>,
    )
  }
}

boot()
