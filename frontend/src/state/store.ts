import { create } from 'zustand'
import type { InterviewData } from '../model/interview'
import type { StageId } from '../model/timer'
import type { Diagram, Edge, Finding, Group, GroupKind, Node, Note, Shape, ShapeKind, NodePoint, Prediction, PredictionAnswer, Scenario, Score, SimStatus, Stats, TraceSet } from '../api/types'
import { MAX_ZOOM, MIN_ZOOM, Reg, emptyDiagram, newId, uniqueLabel, uniqueLabels } from '../model/diagram'
import { alignNodes, distributeNodes, type AlignMode } from '../model/arrange'
import { normalizeDesignName } from '../model/designName'
import { tidyDiagram } from '../model/tidy'
import { collapseView, expandFor, isHidden, setCollapsed, visibleIds } from '../model/collapse'
import { MAX_SHAPES, MAX_SHAPE_LABEL, clampShape, newShape, shapesOf } from '../model/shapes'
import { nodeRect } from '../draw/geometry'
import { clipIsEmpty, copySelection, pasteClip, type Clip } from '../model/clipboard'
import { GROUP_DEFAULT, MAX_GROUPS, MAX_GROUP_LABEL, clampGroupSize, groupLabel, groupsOf, withMembers } from '../model/groups'
import { isCompact, isNarrow } from '../model/responsive'
import { readLevel, type LearnerLevel } from '../model/level'
import type { ChallengeRecord } from '../model/record'
import { learningActions } from './learningActions'

export type Tool = 'select' | 'connect'
export interface Viewport {
  scrollX: number
  scrollY: number
  zoom: number
}
export interface ScenarioState {
  def: Scenario
  hintLevel: number
  hints: string[] // the hints revealed so far, each fetched when asked for
  debrief: string | null // fetched when the result is shown
  deeper: string | null // the optional deeper paragraph that came with the debrief
  weakPoints: string[] // open-challenge risks fetched with the debrief after a pass
  answer: string | null // the full answer, fetched only when "Show the answer" is pressed
  result: Score | null
  predictions: Record<string, PredictionAnswer> // answered prediction questions, with what the server said
  solved: boolean
  dailySeed?: number // set only when the run was started from the daily card: the seed derived from the date
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
const DEFAULT_SEED = 42 // the seed of every run except one started from the daily card

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
  records: Record<string, ChallengeRecord> // how each challenge went: wrong predictions, hints, attempts, answer used
  level: LearnerLevel // the learner's level; "all" hides nothing
  sim: { status: SimStatus; speed: number; seed: number }
  stats: Stats
  nodeSeries: { nodeId: string; points: NodePoint[] } | null // latest per-second history of the selected component
  traces: TraceSet | null // latest request traces from the server; null before the first answer
  cost: number | null // null until the server has priced the current diagram
  scenario: ScenarioState | null
  sandbox: Diagram | null // sandbox saved while a scenario is open
  ui: { library: boolean; help: boolean; welcome: boolean; picker: boolean; inspector: boolean; result: boolean; capacity: boolean; search: boolean; templates: boolean; skills: boolean; compare: boolean; question: string | null; present: boolean }
  toasts: Toast[]
  connection: Connection
  past: Diagram[]
  future: Diagram[]
  gesture: Diagram | null
  fitRequest: number
  /** Ask the canvas to bring this component to the middle of the view (the n changes on every request). */
  centerRequest: { id: string; n: number } | null

  setReg(reg: Reg): void
  requestFit(): void
  /** Select a component and bring it to the middle of the view. */
  focusNode(id: string): void
  clearRun(): void
  addNode(kind: string, x: number, y: number): string | null
  /** Move pieces; boxes among them take what is inside along, unless `exact` says the list already says everything to move. */
  moveNodes(ids: string[], dx: number, dy: number, exact?: boolean): void
  updateNode(id: string, patch: Partial<Pick<Node, 'label'>>): void
  setParam(id: string, key: string, value: number | string | boolean): void
  addEdge(from: string, to: string): string | null
  /** Move one end of an existing connection to another component: same arrow, new place. */
  rewireEdge(edgeId: string, end: 'from' | 'to', nodeId: string): boolean
  updateEdge(id: string, patch: Partial<Pick<Edge, 'latencyMs' | 'bandwidthMbps' | 'lossPct' | 'jitterMs' | 'protocol' | 'async'>>): void
  addNote(x: number, y: number): string
  updateNote(id: string, patch: Partial<Note>): void
  addGroup(kind: GroupKind, x: number, y: number): string | null
  updateGroup(id: string, patch: Partial<Pick<Group, 'label' | 'kind' | 'w' | 'h'>>): void
  /** Collapse a box to its header chip or expand it (no argument: flip it). Purely visual; one undo step. */
  toggleCollapsed(id: string, collapsed?: boolean): void
  /** Name the whole design (normalised; empty means the default name); one undo step. */
  setDiagramName(name: string): void
  addShape(kind: ShapeKind, x: number, y: number): string | null
  updateShape(id: string, patch: Partial<Pick<Shape, 'label' | 'w' | 'h'>>): void
  /** Drag one end of an arrow by (dx, dy); the other end stays put. */
  moveShapeEnd(id: string, end: 'from' | 'to', dx: number, dy: number): void
  deleteIds(ids: string[]): void
  deleteSelection(): void
  /** Remember the selected components (and the connections between them) for pasting. */
  /** Line the selected components up on one edge or centre line. */
  align(mode: AlignMode): void
  /** Space three or more selected components evenly. */
  distribute(axis: 'x' | 'y'): void
  /** Re-lay the whole design left to right from its connections (one undo step). */
  tidyLayout(): void
  copySelection(quiet?: boolean): void
  /** Add a copy of what was copied, offset a little; the copy becomes the selection. */
  paste(): void
  /** Copy and paste in one step. */
  duplicateSelection(): void
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
  /** Open a challenge. `dailySeed` is given only when it was started from the daily card; otherwise the default seed is used. */
  openScenario(def: Scenario, opts?: { dailySeed?: number }): void
  exitScenario(): void
  recordPrediction(p: Prediction, answer: PredictionAnswer): void
  addHint(text: string): void
  showAnswer(text: string): void
  setLevel(level: LearnerLevel): void
  importProgress(progress: Record<string, number>, records: Record<string, ChallengeRecord>): void
  setDebrief(text: string, deeper?: string, weakPoints?: string[]): void
  showSolution(d: Diagram): void
  compareWithReference(reference: Diagram): void
  adoptDiagram(d: Diagram): void
  /** Replace the design with a template's, as one step that can be undone. */
  applyTemplate(d: Diagram): void
  setResult(score: Score): void
  hydrate(p: { sandbox?: Diagram; theme?: 'light' | 'dark'; progress?: Record<string, number>; records?: Record<string, ChallengeRecord>; viewport?: Viewport; interview?: InterviewData }): void
}

let toastSeq = 1

/** What was last copied; kept outside the saved state because it is only for this tab's session. */
let clipboard: Clip | null = null

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
    records: {},
    level: readLevel(),
    sim: { status: 'stopped', speed: 1, seed: DEFAULT_SEED },
    stats: emptyStats(),
    nodeSeries: null,
    traces: null,
    cost: null,
    scenario: null,
    sandbox: null,
    ui: { library: !isNarrow(), help: false, welcome: true, picker: false, inspector: !isCompact(), result: false, capacity: false, search: false, templates: false, skills: false, compare: false, question: null, present: false },
    toasts: [],
    connection: 'connecting',
    past: [],
    future: [],
    gesture: null,
    fitRequest: 0,
    centerRequest: null,

    setReg: (reg) => set({ reg }),
    requestFit: () => set((s) => ({ fitRequest: s.fitRequest + 1 })),
    focusNode: (id) => {
      const reg = get().reg
      // a component hidden inside a collapsed box: open the box first (one undo step)
      if (reg) mutate((d) => expandFor(d, [id], (n) => nodeRect(n, reg)))
      set((s) => ({ selection: [id], centerRequest: { id, n: (s.centerRequest?.n ?? 0) + 1 } }))
    },
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
      mutate((d) => {
        const added = { ...d, nodes: [...d.nodes, { id, kind, x, y, label: uniqueLabel(d, spec.name), params: {} }] }
        // a component dropped inside a collapsed box would vanish: open the box instead
        return expandFor(added, [id], (n) => nodeRect(n, reg!))
      })
      set({ selection: [id], ui: { ...get().ui, welcome: false } })
      return id
    },

    moveNodes: (ids, dx, dy, exact) => {
      const reg = get().reg
      mutate((d) => {
        // moving a box takes what is inside it
        const set_ = new Set(!exact && reg && groupsOf(d).length ? withMembers(d, ids, (n) => nodeRect(n, reg)) : ids)
        return {
          ...d,
          nodes: d.nodes.map((n) => (set_.has(n.id) ? { ...n, x: n.x + dx, y: n.y + dy } : n)),
          notes: d.notes.map((n) => (set_.has(n.id) ? { ...n, x: n.x + dx, y: n.y + dy } : n)),
          ...(d.groups ? { groups: d.groups.map((g) => (set_.has(g.id) ? { ...g, x: g.x + dx, y: g.y + dy } : g)) } : {}),
          ...(d.shapes ? { shapes: d.shapes.map((s) => (ids.includes(s.id) ? { ...s, x: s.x + dx, y: s.y + dy } : s)) } : {}),
        }
      })
    },

    updateNode: (id, patch) => mutate((d) => ({ ...d, nodes: d.nodes.map((n) => (n.id === id ? { ...n, ...patch } : n)) })),

    setParam: (id, key, value) =>
      mutate((d) => ({
        ...d,
        nodes: d.nodes.map((n) => (n.id === id ? { ...n, params: { ...n.params, [key]: value } } : n)),
      })),

    rewireEdge: (edgeId, end, nodeId) => {
      const { reg, diagram } = get()
      const e = diagram.edges.find((x) => x.id === edgeId)
      if (!reg || !e) return false
      const from = end === 'from' ? nodeId : e.from
      const to = end === 'to' ? nodeId : e.to
      const f = diagram.nodes.find((n) => n.id === from)
      const t = diagram.nodes.find((n) => n.id === to)
      if (!f || !t || from === to || (end === 'from' ? e.from : e.to) === nodeId) return false
      if (guardTopology()) return false
      if (!reg.canConnect(f.kind, t.kind)) {
        get().toast(`${reg.spec(f.kind)?.name} can't connect to ${reg.spec(t.kind)?.name}`, 'warn')
        return false
      }
      if (diagram.edges.some((x) => x.id !== edgeId && x.from === from && x.to === to)) {
        get().toast('Those two are already connected', 'info')
        return false
      }
      mutate((d) => ({ ...d, edges: d.edges.map((x) => (x.id === edgeId ? { ...x, from, to, latencyMs: reg.defaultLatency(f.kind, t.kind) } : x)) }))
      return true
    },

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

    addGroup: (kind, x, y) => {
      if (groupsOf(get().diagram).length >= MAX_GROUPS) {
        get().toast(`At most ${MAX_GROUPS} boxes`, 'warn')
        return null
      }
      const id = newId('g')
      mutate((d) => ({ ...d, groups: [...groupsOf(d), { id, kind, label: groupLabel(d, kind), x, y, ...GROUP_DEFAULT }] }))
      set({ selection: [id], ui: { ...get().ui, welcome: false } })
      return id
    },

    setDiagramName: (raw) => {
      const name = normalizeDesignName(raw)
      mutate((d) => {
        if ((d.name ?? '') === name) return d
        const { name: _old, ...rest } = d
        void _old
        return name ? { name, ...rest } : rest
      })
    },

    toggleCollapsed: (id, collapsed) => {
      const g = groupsOf(get().diagram).find((x) => x.id === id)
      if (!g) return
      mutate((d) => setCollapsed(d, id, collapsed ?? !g.collapsed))
      set({ selection: [id] })
    },

    updateGroup: (id, patch) =>
      mutate((d) => ({
        ...d,
        groups: groupsOf(d).map((g) => {
          if (g.id !== id) return g
          const next = { ...g, ...patch }
          return { ...next, label: [...next.label].slice(0, MAX_GROUP_LABEL).join(''), w: clampGroupSize(next.w), h: clampGroupSize(next.h) }
        }),
      })),

    addShape: (kind, x, y) => {
      if (shapesOf(get().diagram).length >= MAX_SHAPES) {
        get().toast(`At most ${MAX_SHAPES} shapes`, 'warn')
        return null
      }
      const id = newId('s')
      mutate((d) => ({ ...d, shapes: [...shapesOf(d), newShape(id, kind, x, y)] }))
      set({ selection: [id], ui: { ...get().ui, welcome: false } })
      return id
    },

    updateShape: (id, patch) =>
      mutate((d) => ({
        ...d,
        shapes: shapesOf(d).map((s) => {
          if (s.id !== id) return s
          const next = { ...s, ...patch }
          return { ...next, label: [...next.label].slice(0, MAX_SHAPE_LABEL).join(''), ...clampShape(next.kind, next.w, next.h) }
        }),
      })),

    moveShapeEnd: (id, end, dx, dy) =>
      mutate((d) => ({
        ...d,
        shapes: shapesOf(d).map((s) => {
          if (s.id !== id || s.kind !== 'arrow') return s
          return end === 'to' ? { ...s, ...clampShape('arrow', s.w + dx, s.h + dy) } : { ...s, x: s.x + dx, y: s.y + dy, ...clampShape('arrow', s.w - dx, s.h - dy) }
        }),
      })),

    deleteIds: (ids) => {
      const kill = new Set(ids.filter((id) => !locked(id)))
      if (kill.size === 0) return
      const structural = get().diagram.nodes.some((n) => kill.has(n.id)) || get().diagram.edges.some((e) => kill.has(e.id))
      if (structural && guardTopology()) return
      mutate((d) => ({
        ...d,
        nodes: d.nodes.filter((n) => !kill.has(n.id)),
        edges: d.edges.filter((e) => !kill.has(e.id) && !kill.has(e.from) && !kill.has(e.to)),
        notes: d.notes.filter((n) => !kill.has(n.id)),
        ...(d.groups ? { groups: d.groups.filter((g) => !kill.has(g.id)) } : {}),
        ...(d.shapes ? { shapes: d.shapes.filter((s) => !kill.has(s.id)) } : {}),
      }))
      set((s) => ({ selection: s.selection.filter((id) => !kill.has(id)) }))
    },
    deleteSelection: () => get().deleteIds(get().selection),

    align: (mode) => {
      const reg = get().reg
      if (reg) mutate((d) => alignNodes(d, get().selection, mode, (n) => nodeRect(n, reg)))
    },
    distribute: (axis) => {
      const reg = get().reg
      if (reg) mutate((d) => distributeNodes(d, get().selection, axis, (n) => nodeRect(n, reg)))
    },
    tidyLayout: () => {
      const reg = get().reg
      if (!reg || get().diagram.nodes.length === 0) return
      mutate((d) => tidyDiagram(d, (n) => nodeRect(n, reg)))
      get().requestFit()
    },

    copySelection: (quiet) => {
      const { diagram, selection, reg } = get()
      // copying a box takes what is inside it
      const clip = copySelection(diagram, reg ? withMembers(diagram, selection, (n) => nodeRect(n, reg)) : selection)
      if (clipIsEmpty(clip)) return
      clipboard = clip
      if (!quiet) get().toast(`Copied ${clip.nodes.length + clip.notes.length + (clip.groups?.length ?? 0) + (clip.shapes?.length ?? 0)}`, 'info')
    },
    paste: () => {
      if (!clipboard || clipIsEmpty(clipboard)) return
      if (clipboard.nodes.length > 0 && guardTopology()) return
      let ids: string[] = []
      mutate((d) => {
        const out = pasteClip(d, clipboard!, newId)
        ids = out.ids
        return out.diagram
      })
      // only what can be seen is selected: a pasted collapsed box hides its members
      const reg = get().reg
      if (reg) {
        const d = get().diagram
        ids = visibleIds(d, collapseView(d, (n) => nodeRect(n, reg)), ids)
      }
      // paste again and the next copy lands a step further along
      clipboard = { ...clipboard, nodes: clipboard.nodes.map((n) => ({ ...n, x: n.x + 24, y: n.y + 24 })), notes: clipboard.notes.map((n) => ({ ...n, x: n.x + 24, y: n.y + 24 })), groups: (clipboard.groups ?? []).map((g) => ({ ...g, x: g.x + 24, y: g.y + 24 })), shapes: (clipboard.shapes ?? []).map((s) => ({ ...s, x: s.x + 24, y: s.y + 24 })) }
      set({ selection: ids, ui: { ...get().ui, welcome: false } })
    },
    duplicateSelection: () => {
      get().copySelection(true)
      get().paste()
    },

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

    setSelection: (ids) => {
      // selecting something that sits hidden in a collapsed box (a lint finding, the outline, a search) opens that box
      const { reg, diagram } = get()
      if (reg && diagram.groups?.some((g) => g.collapsed)) {
        const rect = (n: Node) => nodeRect(n, reg)
        const v = collapseView(diagram, rect)
        if (ids.some((id) => isHidden(v, id))) mutate((d) => expandFor(d, ids, rect))
      }
      set({ selection: ids })
    },
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
    setPinned: (pinned) => set({ pinned: pinned ? structuredClone(pinned) : null }),
    setConnection: (connection) => set({ connection }),

    toast: (text, kind = 'info', ms = 3500) => {
      const id = toastSeq++
      set((s) => ({ toasts: [...s.toasts.slice(-3), { id, kind, text }] }))
      setTimeout(() => get().dismissToast(id), ms)
    },
    dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),

    openScenario: (def, opts) => {
      const s = get()
      set({
        sandbox: s.scenario ? s.sandbox : s.diagram,
        diagram: structuredClone(def.diagram),
        past: [],
        future: [],
        selection: [],
        stats: emptyStats(),
        traces: null,
        scenario: { def, hintLevel: 0, hints: [], debrief: null, deeper: null, weakPoints: [], answer: null, result: null, predictions: {}, solved: false, compare: null, ...(opts?.dailySeed !== undefined ? { dailySeed: opts.dailySeed } : {}) },
        sim: { ...s.sim, status: 'stopped', speed: def.speed || 1, seed: opts?.dailySeed ?? DEFAULT_SEED },
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
        sim: { ...s.sim, status: 'stopped', seed: DEFAULT_SEED },
        ui: { ...s.ui, welcome: !(s.sandbox?.nodes.length), result: false, library: !isNarrow() },
      })
    },
    showSolution: (d) => {
      if (!get().scenario) return
      const mine = structuredClone(get().diagram)
      const ref = uniqueLabels(structuredClone(d))
      mutate(() => structuredClone(ref))
      set((s) => ({ scenario: s.scenario ? { ...s.scenario, solved: true, compare: { mine, reference: ref } } : null, selection: [] }))
    },
    applyTemplate: (d) => {
      if (guardTopology()) return
      const keep = get().diagram.name
      mutate(() => (keep ? { ...d, name: keep } : d)) // a template fills the canvas; the design keeps its name
      set({ selection: [], ui: { ...get().ui, welcome: false, templates: false } })
      get().requestFit()
    },
    adoptDiagram: (d) => set((s) => ({ diagram: d, selection: s.selection.filter((id) => d.nodes.some((n) => n.id === id)) })),
    compareWithReference: (reference) =>
      set((s) => (s.scenario ? { scenario: { ...s.scenario, compare: { mine: structuredClone(s.diagram), reference: uniqueLabels(structuredClone(reference)) } } } : {})),
    ...learningActions(set),

    hydrate: (p) =>
      set((s) => ({
        diagram: p.sandbox ?? s.diagram,
        theme: p.theme ?? s.theme,
        progress: p.progress ?? s.progress,
        records: p.records ?? s.records,
        viewport: p.viewport ?? s.viewport,
        interview: p.interview ?? s.interview,
        ui: { ...s.ui, welcome: !(p.sandbox?.nodes.length) },
      })),
  }
})
