import { create } from 'zustand'
import { emptyInterview, type InterviewData } from '../model/interview'
import { newSession, pause, resume, switchStage, type StageId } from '../model/timer'
import type { Diagram, Edge, Finding, Node, Note, NodePoint, Prediction, Scenario, Score, SimStatus, Stats, TraceSet } from '../api/types'
import { MAX_ZOOM, MIN_ZOOM, Reg, emptyDiagram, newId, uniqueLabel } from '../model/diagram'
import { isCompact, isNarrow } from '../model/responsive'

export type Tool = 'select' | 'connect'
export interface Viewport {
  scrollX: number
  scrollY: number
  zoom: number
}
export interface ScenarioState {
  def: Scenario
  hintLevel: number
  result: Score | null
  predictions: Record<string, number>
  solved: boolean
  compare: { mine: Diagram; reference: Diagram } | null // your design next to the reference, once you ask or reveal it
}
export interface Toast {
  id: number
  kind: 'info' | 'warn' | 'error'
  text: string
}
export type Connection = 'connecting' | 'open' | 'closed'

export const emptyStats = (): Stats => ({ global: { now: 0, inFlight: 0, cost: 0, avgCost: 0, run: emptyRun() }, nodes: {}, edges: {}, series: [], events: [] })
function emptyRun() {
  return { resolved: 0, pending: 0, ok: 0, failed: 0, p50: 0, p95: 0, p99: 0, failP50: 0, latencyBuckets: [], mean: 0, errors: {}, conflicts: 0, freshReads: 0, staleReads: 0, duplicates: 0, lostWrites: 0 }
}

const HISTORY = 100

export interface State {
  reg: Reg | null
  diagram: Diagram
  selection: string[]
  pinned: Diagram | null // design A for the side-by-side comparison
  interview: InterviewData | null // the interview session and its pads, kept in this browser only
  findings: Finding[] // advisory design lint for the current diagram
  tool: Tool
  viewport: Viewport
  theme: 'light' | 'dark'
  progress: Record<string, number>
  sim: { status: SimStatus; speed: number; seed: number }
  stats: Stats
  nodeSeries: { nodeId: string; points: NodePoint[] } | null // latest per-second history of the selected component
  traces: TraceSet | null // latest request traces from the server; null before the first answer
  cost: number | null // null until the server has priced the current diagram
  scenario: ScenarioState | null
  sandbox: Diagram | null // sandbox saved while a scenario is open
  ui: { library: boolean; help: boolean; welcome: boolean; picker: boolean; inspector: boolean; result: boolean; capacity: boolean; compare: boolean; question: string | null; present: boolean }
  toasts: Toast[]
  connection: Connection
  past: Diagram[]
  future: Diagram[]
  gesture: Diagram | null
  fitRequest: number

  setReg(reg: Reg): void
  requestFit(): void
  clearRun(): void
  addNode(kind: string, x: number, y: number): string | null
  moveNodes(ids: string[], dx: number, dy: number): void
  updateNode(id: string, patch: Partial<Pick<Node, 'label'>>): void
  setParam(id: string, key: string, value: number | string | boolean): void
  addEdge(from: string, to: string): string | null
  updateEdge(id: string, patch: Partial<Pick<Edge, 'latencyMs' | 'bandwidthMbps'>>): void
  addNote(x: number, y: number): string
  updateNote(id: string, patch: Partial<Note>): void
  deleteIds(ids: string[]): void
  deleteSelection(): void
  beginGesture(): void
  endGesture(): void
  undo(): void
  redo(): void
  newDiagram(): void
  loadDiagram(d: Diagram): void
  importDiagram(d: Diagram): void
  setSelection(ids: string[]): void
  setTool(t: Tool): void
  setViewport(v: Partial<Viewport>): void
  zoomAt(factor: number, cx: number, cy: number): void
  setTheme(t: 'light' | 'dark'): void
  setUi(p: Partial<State['ui']>): void
  setSim(p: Partial<State['sim']>): void
  setStats(s: Stats): void
  setNodeSeries(s: { nodeId: string; points: NodePoint[] } | null): void
  setTraces(t: TraceSet | null): void
  setCost(c: number): void
  setFindings(f: Finding[]): void
  setPinned(d: Diagram | null): void
  setInterview(i: InterviewData | null): void
  startInterview(minutes: number): void
  interviewStage(stage: StageId): void
  interviewPause(paused: boolean): void
  endInterview(): void
  recordStars(id: string, stars: number): void
  setConnection(c: Connection): void
  toast(text: string, kind?: Toast['kind'], ms?: number): void
  dismissToast(id: number): void
  openScenario(def: Scenario): void
  exitScenario(): void
  answerPrediction(p: Prediction, choice: number): void
  revealHint(): void
  showSolution(d: Diagram): void
  compareWithReference(reference: Diagram): void
  adoptDiagram(d: Diagram): void
  setResult(score: Score): void
  hydrate(p: { sandbox?: Diagram; theme?: 'light' | 'dark'; progress?: Record<string, number>; viewport?: Viewport; interview?: InterviewData }): void
}

let toastSeq = 1

export const useStore = create<State>()((set, get) => {
  /** Apply a diagram change and record undo history. */
  const mutate = (fn: (d: Diagram) => Diagram) => {
    const cur = get().diagram
    const next = fn(cur)
    if (next === cur) return
    if (get().gesture) {
      set({ diagram: next, future: [] })
      return
    }
    set((s) => ({ diagram: next, past: [...s.past, cur].slice(-HISTORY), future: [] }))
  }
  const configOnly = () => !!get().scenario?.def.configOnly
  const guardTopology = (): boolean => {
    if (configOnly()) {
      get().toast('Incident mode: fix it with configuration, the architecture is fixed', 'warn')
      return true
    }
    return false
  }
  const locked = (id: string) => get().diagram.nodes.some((n) => n.id === id && n.locked)

  return {
    reg: null,
    diagram: emptyDiagram(),
    selection: [],
    findings: [],
    interview: null,
    pinned: null,
    tool: 'select',
    viewport: { scrollX: 0, scrollY: 0, zoom: 1 },
    theme: typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light',
    progress: {},
    sim: { status: 'stopped', speed: 1, seed: 42 },
    stats: emptyStats(),
    nodeSeries: null,
    traces: null,
    cost: null,
    scenario: null,
    sandbox: null,
    ui: { library: !isNarrow(), help: false, welcome: true, picker: false, inspector: !isCompact(), result: false, capacity: false, compare: false, question: null, present: false },
    toasts: [],
    connection: 'connecting',
    past: [],
    future: [],
    gesture: null,
    fitRequest: 0,

    setReg: (reg) => set({ reg }),
    requestFit: () => set((s) => ({ fitRequest: s.fitRequest + 1 })),
    clearRun: () =>
      set((s) => ({
        stats: emptyStats(),
        traces: null,
        scenario: s.scenario ? { ...s.scenario, result: null } : null,
        ui: { ...s.ui, result: false },
      })),

    addNode: (kind, x, y) => {
      const reg = get().reg
      const spec = reg?.spec(kind)
      if (!spec || guardTopology()) return null
      const id = newId('n')
      mutate((d) => ({
        ...d,
        nodes: [...d.nodes, { id, kind, x, y, label: uniqueLabel(d, spec.name), params: {} }],
      }))
      set({ selection: [id], ui: { ...get().ui, welcome: false } })
      return id
    },

    moveNodes: (ids, dx, dy) => {
      const set_ = new Set(ids)
      mutate((d) => ({
        ...d,
        nodes: d.nodes.map((n) => (set_.has(n.id) ? { ...n, x: n.x + dx, y: n.y + dy } : n)),
        notes: d.notes.map((n) => (set_.has(n.id) ? { ...n, x: n.x + dx, y: n.y + dy } : n)),
      }))
    },

    updateNode: (id, patch) => mutate((d) => ({ ...d, nodes: d.nodes.map((n) => (n.id === id ? { ...n, ...patch } : n)) })),

    setParam: (id, key, value) =>
      mutate((d) => ({
        ...d,
        nodes: d.nodes.map((n) => (n.id === id ? { ...n, params: { ...n.params, [key]: value } } : n)),
      })),

    addEdge: (from, to) => {
      const { reg, diagram } = get()
      const f = diagram.nodes.find((n) => n.id === from)
      const t = diagram.nodes.find((n) => n.id === to)
      if (!reg || !f || !t || from === to) return null
      if (guardTopology()) return null
      if (!reg.canConnect(f.kind, t.kind)) {
        get().toast(`${reg.spec(f.kind)?.name} can't connect to ${reg.spec(t.kind)?.name}`, 'warn')
        return null
      }
      if (diagram.edges.some((e) => e.from === from && e.to === to)) return null
      const id = newId('e')
      mutate((d) => ({ ...d, edges: [...d.edges, { id, from, to, latencyMs: reg.defaultLatency(f.kind, t.kind) }] }))
      set({ selection: [id] })
      return id
    },

    updateEdge: (id, patch) => mutate((d) => ({ ...d, edges: d.edges.map((e) => (e.id === id ? { ...e, ...patch } : e)) })),

    addNote: (x, y) => {
      const id = newId('t')
      mutate((d) => ({ ...d, notes: [...d.notes, { id, x, y, text: '' }] }))
      set({ selection: [id] })
      return id
    },

    updateNote: (id, patch) => mutate((d) => ({ ...d, notes: d.notes.map((n) => (n.id === id ? { ...n, ...patch } : n)) })),

    deleteIds: (ids) => {
      const kill = new Set(ids.filter((id) => !locked(id)))
      if (kill.size === 0) return
      const structural = get().diagram.nodes.some((n) => kill.has(n.id)) || get().diagram.edges.some((e) => kill.has(e.id))
      if (structural && guardTopology()) return
      mutate((d) => ({
        nodes: d.nodes.filter((n) => !kill.has(n.id)),
        edges: d.edges.filter((e) => !kill.has(e.id) && !kill.has(e.from) && !kill.has(e.to)),
        notes: d.notes.filter((n) => !kill.has(n.id)),
      }))
      set((s) => ({ selection: s.selection.filter((id) => !kill.has(id)) }))
    },
    deleteSelection: () => get().deleteIds(get().selection),

    beginGesture: () => {
      if (!get().gesture) set({ gesture: get().diagram })
    },
    endGesture: () => {
      const g = get().gesture
      if (!g) return
      set((s) => (s.diagram === g ? { gesture: null } : { gesture: null, past: [...s.past, g].slice(-HISTORY), future: [] }))
    },

    undo: () => {
      const { past, diagram } = get()
      if (!past.length) return
      set((s) => ({ diagram: past[past.length - 1], past: past.slice(0, -1), future: [diagram, ...s.future], selection: [] }))
    },
    redo: () => {
      const { future, diagram } = get()
      if (!future.length) return
      set((s) => ({ diagram: future[0], future: future.slice(1), past: [...s.past, diagram], selection: [] }))
    },

    newDiagram: () => {
      if (get().scenario) return
      mutate(() => emptyDiagram())
      set({ selection: [], ui: { ...get().ui, welcome: true } })
    },
    loadDiagram: (d) => set({ diagram: d, past: [], future: [], selection: [], cost: null }),
    // like loadDiagram but undoable, so opening someone else's design never destroys your own
    importDiagram: (d) => {
      mutate(() => d)
      set({ selection: [], ui: { ...get().ui, welcome: false } })
    },

    setSelection: (ids) => set({ selection: ids }),
    setTool: (tool) => set({ tool }),
    setViewport: (v) => set((s) => ({ viewport: { ...s.viewport, ...v } })),
    zoomAt: (factor, cx, cy) =>
      set((s) => {
        const z = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, s.viewport.zoom * factor))
        const k = z / s.viewport.zoom
        // keep the world point under (cx, cy) fixed
        return { viewport: { zoom: z, scrollX: cx - (cx - s.viewport.scrollX) * k, scrollY: cy - (cy - s.viewport.scrollY) * k } }
      }),
    setTheme: (theme) => {
      document.documentElement.setAttribute('data-theme', theme)
      set({ theme })
    },
    setUi: (p) => set((s) => ({ ui: { ...s.ui, ...p } })),
    setSim: (p) => set((s) => ({ sim: { ...s.sim, ...p } })),
    setStats: (stats) => set((st) => ({ stats: { ...stats, series: stats.series ?? [], events: stats.events ?? (stats.global.now > 0 && stats.global.now >= st.stats.global.now ? st.stats.events : []), nodes: stats.nodes ?? {}, edges: stats.edges ?? {}, global: { ...stats.global, run: { ...stats.global.run, latencyBuckets: stats.global.run.latencyBuckets ?? [] } } } })),
    setTraces: (traces) => set({ traces }),
    setNodeSeries: (nodeSeries) => set({ nodeSeries }),
    setCost: (cost) => set({ cost }),
    setFindings: (findings) => set({ findings }),
    recordStars: (id, stars) => set((s) => ((s.progress[id] ?? 0) >= stars ? {} : { progress: { ...s.progress, [id]: stars } })),
    setInterview: (interview) => set({ interview }),
    startInterview: (minutes) => set((s) => ({ interview: { ...(s.interview ?? emptyInterview()), session: newSession(Math.min(360, Math.max(1, minutes)) * 60_000, Date.now()) }, ui: { ...s.ui, welcome: false } })),
    interviewStage: (stage) => set((s) => (s.interview?.session ? { interview: { ...s.interview, session: switchStage(s.interview.session, stage, Date.now()) } } : {})),
    interviewPause: (paused) => set((s) => (s.interview?.session ? { interview: { ...s.interview, session: (paused ? pause : resume)(s.interview.session, Date.now()) } } : {})),
    endInterview: () => set((s) => (s.interview ? { interview: { ...s.interview, session: null } } : {})),
    setPinned: (pinned) => set({ pinned: pinned ? structuredClone(pinned) : null }),
    setConnection: (connection) => set({ connection }),

    toast: (text, kind = 'info', ms = 3500) => {
      const id = toastSeq++
      set((s) => ({ toasts: [...s.toasts.slice(-3), { id, kind, text }] }))
      setTimeout(() => get().dismissToast(id), ms)
    },
    dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),

    openScenario: (def) => {
      const s = get()
      set({
        sandbox: s.scenario ? s.sandbox : s.diagram,
        diagram: structuredClone(def.diagram),
        past: [],
        future: [],
        selection: [],
        stats: emptyStats(),
        traces: null,
        scenario: { def, hintLevel: 0, result: null, predictions: {}, solved: false, compare: null },
        sim: { ...s.sim, status: 'stopped', speed: def.speed || 1 },
        ui: { ...s.ui, picker: false, welcome: false, result: false, library: !def.configOnly && !isNarrow() },
      })
    },
    exitScenario: () => {
      const s = get()
      set({
        diagram: s.sandbox ?? emptyDiagram(),
        sandbox: null,
        past: [],
        future: [],
        selection: [],
        stats: emptyStats(),
        traces: null,
        scenario: null,
        sim: { ...s.sim, status: 'stopped' },
        ui: { ...s.ui, welcome: !(s.sandbox?.nodes.length), result: false, library: !isNarrow() },
      })
    },
    answerPrediction: (p, choice) =>
      set((s) => (s.scenario && !(p.id in s.scenario.predictions) ? { scenario: { ...s.scenario, predictions: { ...s.scenario.predictions, [p.id]: choice } } } : {})),
    revealHint: () => set((s) => (s.scenario ? { scenario: { ...s.scenario, hintLevel: Math.min(3, s.scenario.hintLevel + 1) } } : {})),
    showSolution: (d) => {
      if (!get().scenario) return
      const mine = structuredClone(get().diagram)
      mutate(() => structuredClone(d))
      set((s) => ({ scenario: s.scenario ? { ...s.scenario, solved: true, compare: { mine, reference: structuredClone(d) } } : null, selection: [] }))
    },
    adoptDiagram: (d) => set((s) => ({ diagram: d, selection: s.selection.filter((id) => d.nodes.some((n) => n.id === id)) })),
    compareWithReference: (reference) =>
      set((s) => (s.scenario ? { scenario: { ...s.scenario, compare: { mine: structuredClone(s.diagram), reference: structuredClone(reference) } } } : {})),
    setResult: (score) =>
      set((s) => {
        if (!s.scenario) return {}
        const id = s.scenario.def.id
        const better = score.stars > (s.progress[id] ?? 0)
        return { scenario: { ...s.scenario, result: score }, ui: { ...s.ui, result: true }, progress: better ? { ...s.progress, [id]: score.stars } : s.progress }
      }),

    hydrate: (p) =>
      set((s) => ({
        diagram: p.sandbox ?? s.diagram,
        theme: p.theme ?? s.theme,
        progress: p.progress ?? s.progress,
        viewport: p.viewport ?? s.viewport,
        interview: p.interview ?? s.interview,
        ui: { ...s.ui, welcome: !(p.sandbox?.nodes.length) },
      })),
  }
})
