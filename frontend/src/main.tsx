import '@fontsource/kalam/400.css'
import '@fontsource/kalam/700.css'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { fetchRegistry } from './api/client'
import { Reg } from './model/diagram'
import { startBridge } from './sim/bridge'
import { startPersistence } from './state/persist'
import { useStore } from './state/store'
import './styles.css'

async function boot() {
  const root = createRoot(document.getElementById('root')!)
  try {
    const reg = new Reg(await fetchRegistry())
    useStore.getState().setReg(reg)
    startPersistence(reg)
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
