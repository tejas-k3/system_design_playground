import { validate } from '../api/client'
import type { ServerMsg } from '../api/types'
import { nothingToRun } from '../model/diagram'
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
      if (m.diagram) st.adoptDiagram(m.diagram) // the answer to a seek carries the design as it was then
      pushParticles(m.particles)
      m.notices?.forEach((n) => st.toast(n.text, n.level === 'warn' ? 'warn' : n.level, 4000))
      break
    case 'traces':
      st.setTraces({ failed: m.failed ?? [], ok: m.ok ?? [] })
      break
    case 'nodeSeries':
      st.setNodeSeries({ nodeId: m.nodeId, points: m.points ?? [] })
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
    if (state === 'closed' && socket?.asleep) {
      st.setSim({ status: 'stopped' })
      st.toast('Disconnected after being idle. Press Play to reconnect.', 'info', 5000)
    } else if (state === 'closed' && st.sim.status === 'running') {
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
          .then((r) => {
            useStore.getState().setCost(r.costPerHour)
            useStore.getState().setFindings(r.findings ?? [])
          })
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

/** Run a command once connected, waking a dormant socket (or waiting for the first connect). */
function withSocket(fn: (sock: SimSocket) => void): void {
  const sock = socket
  if (!sock) return
  if (sock.open) return fn(sock)
  useStore.getState().toast('Connecting to the simulator…', 'info')
  sock.whenOpen(() => fn(sock))
}

/** A new run needs traffic to simulate; say what is missing instead of starting a run that does nothing. */
function blockedStart(): boolean {
  const s = useStore.getState()
  const why = s.sim.status === 'stopped' || !!s.scenario?.result ? nothingToRun(s.diagram) : null
  if (why) s.toast(why, 'info')
  return !!why
}

export function play(): void {
  if (blockedStart()) return
  withSocket((sock) => {
    const s = useStore.getState()
    if (s.sim.status === 'stopped' || !!s.scenario?.result) {
      if (s.scenario?.result) s.clearRun() // a new run starts from nothing: the last result and its numbers no longer describe it
      s.setUi({ welcome: false })
      sendInit()
    }
    sock.send({ type: 'play' })
  })
}

/** Ask the server for the latest request traces; the answer lands in the store. */
export const requestTraces = () => socket?.open && socket.send({ type: 'traces' })

/** Ask the server for one component's per-second history; the answer lands in the store. */
export const requestNodeSeries = (nodeId: string) => socket?.open && socket.send({ type: 'nodeSeries', nodeId })

export const pause = () => withSocket((sock) => sock.send({ type: 'pause' }))

export function reset(): void {
  useStore.getState().clearRun()
  if (socket?.open) socket.send({ type: 'reset' }) // nothing to reset on a closed connection
}

/** Rewind the run to `t` simulated seconds (paused there; Play carries on from that moment). */
export function seek(t: number): void {
  withSocket((sock) => sock.send({ type: 'seek', t }))
}

export function step(): void {
  if (blockedStart()) return
  withSocket((sock) => {
    if (useStore.getState().sim.status === 'stopped') sendInit()
    sock.send({ type: 'step' })
  })
}
export const togglePlay = () => (useStore.getState().sim.status === 'running' ? pause() : play())
export const crashNode = (nodeId: string) => socket?.send({ type: 'crash', nodeId })
export const restartNode = (nodeId: string) => socket?.send({ type: 'restart', nodeId })
