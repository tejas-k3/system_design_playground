import { validate } from '../api/client'
import type { ServerMsg } from '../api/types'
import { useStore } from '../state/store'
import { pushParticles } from './particles'
import { SimSocket } from './socket'

let socket: SimSocket | null = null
let timers: Record<string, ReturnType<typeof setTimeout> | undefined> = {}

function debounce(key: string, ms: number, fn: () => void) {
  clearTimeout(timers[key])
  timers[key] = setTimeout(fn, ms)
}

function onMessage(m: ServerMsg) {
  const st = useStore.getState()
  switch (m.type) {
    case 'frame':
      st.setSim({ status: m.status, speed: m.speed })
      if (m.stats) {
        st.setStats(m.stats)
        if (m.stats.global.cost > 0) st.setCost(m.stats.global.cost)
      }
      pushParticles(m.particles)
      m.notices?.forEach((n) => st.toast(n.text, n.level === 'warn' ? 'warn' : n.level, 4000))
      break
    case 'result':
      if (st.scenario?.def.id === m.scenarioId) st.setResult(m.score)
      break
    case 'error':
      st.toast(m.message, 'error', 5000)
      break
  }
}

function sendInit() {
  const s = useStore.getState()
  socket?.send({ type: 'init', diagram: s.diagram, scenarioId: s.scenario?.def.id, seed: s.sim.seed, speed: s.sim.speed })
}

/** Connect to the simulation server and keep it in step with the store. */
export function startBridge(): () => void {
  socket = new SimSocket(onMessage, (state) => {
    const st = useStore.getState()
    st.setConnection(state)
    if (state === 'open') sendInit()
    if (state === 'closed' && st.sim.status === 'running') {
      st.setSim({ status: 'paused' })
      st.toast('Lost connection to the simulator. Reconnecting…', 'warn')
    }
  })
  socket.connect()

  const unsub = useStore.subscribe((s, p) => {
    if (s.scenario?.def.id !== p.scenario?.def.id) sendInit()
    if (s.diagram !== p.diagram) {
      debounce('diagram', 120, () => socket?.send({ type: 'diagram', diagram: useStore.getState().diagram }))
      debounce('cost', 250, () => {
        validate(useStore.getState().diagram)
          .then((r) => useStore.getState().setCost(r.costPerHour))
          .catch(() => undefined)
      })
    }
    if (s.sim.speed !== p.sim.speed) socket?.send({ type: 'speed', speed: s.sim.speed })
  })
  return () => {
    unsub()
    socket?.close()
    socket = null
    timers = {}
  }
}

// ---- commands ---------------------------------------------------------------------------

function ready(): boolean {
  if (socket?.open) return true
  useStore.getState().toast('Not connected to the simulator yet', 'warn')
  return false
}

export function play(): void {
  if (!ready()) return
  const s = useStore.getState()
  const fresh = s.sim.status === 'stopped' || !!s.scenario?.result
  if (fresh) {
    useStore.getState().setUi({ welcome: false })
    sendInit()
  }
  socket!.send({ type: 'play' })
}

export const pause = () => ready() && socket!.send({ type: 'pause' })
export function reset(): void {
  useStore.getState().clearRun()
  if (ready()) socket!.send({ type: 'reset' })
}
export function step(): void {
  if (!ready()) return
  const s = useStore.getState()
  if (s.sim.status === 'stopped') sendInit()
  socket!.send({ type: 'step' })
}
export const togglePlay = () => (useStore.getState().sim.status === 'running' ? pause() : play())
export const crashNode = (nodeId: string) => socket?.send({ type: 'crash', nodeId })
export const restartNode = (nodeId: string) => socket?.send({ type: 'restart', nodeId })
