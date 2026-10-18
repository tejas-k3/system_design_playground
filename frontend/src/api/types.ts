// Wire types shared with the Go server (see backend/internal/engine).

export type ParamValue = number | string | boolean
export type Params = Record<string, ParamValue>

export interface Node {
  id: string
  kind: string
  x: number
  y: number
  label: string
  params: Params
  locked?: boolean
}
export interface Edge {
  id: string
  from: string
  to: string
  latencyMs: number
}
export interface Note {
  id: string
  x: number
  y: number
  text: string
  fontSize?: number
}
export interface SweepPoint {
  value: number // load multiple, or the setting's value as used
  rps: number
  successRate: number
  p50: number
  p95: number
  p99: number
  costPerHour: number
  truncated?: boolean
}
export interface SweepResponse {
  points: SweepPoint[]
  truncated?: boolean
}

export interface Finding {
  rule: string
  severity: 'error' | 'warn' | 'info'
  message: string
  fix: string
  nodes: string[]
}

export interface Diagram {
  nodes: Node[]
  edges: Edge[]
  notes: Note[]
}

export interface ParamSpec {
  key: string
  label: string
  group: string
  type: 'number' | 'select' | 'boolean'
  default: ParamValue
  unit?: string
  min?: number
  max?: number
  step?: number
  help?: string
  options?: { value: string; label: string }[]
  showIf?: { key: string; in: string[] }
  needsEdgeTo?: string[]
  fixedInChallenge?: boolean
  advanced?: boolean
}

export interface NodeSpec {
  kind: string
  name: string
  description: string
  lesson: string
  accent: string
  width: number
  height: number
  canConnectTo: string[] | null
  params: ParamSpec[] | null
}

export interface LatencyRule {
  from?: string
  to?: string
  ms: number
}

export interface Registry {
  nodes: NodeSpec[]
  latency: { min: number; max: number; rules: LatencyRule[] }
  speeds: number[]
  reasons: { id: string; label: string }[]
}

export interface Goals {
  minSuccessRate: number
  maxP50Ms?: number
  maxP95Ms?: number
  maxCostPerHour?: number
  minJobsCompleted?: number
  maxJobP95Ms?: number
  maxConflicts?: number
  maxStaleReadPct?: number
  maxDuplicates?: number
  maxLostWrites?: number
  starCostPerHour?: number
  starP95Ms?: number
}

export interface Prediction {
  id: string
  question: string
  options: string[]
  answer: number
  explain: string
}

export interface ScenarioSummary {
  id: string
  title: string
  difficulty: string
  summary: string
  configOnly?: boolean
  open?: boolean // a blank canvas with requirements; the load is hidden and there is no solution to show
}

export interface Scenario extends ScenarioSummary {
  story: string
  diagram: Diagram
  goals: Goals
  durationSec: number
  warmupSec: number
  speed: number
  hints: [string, string, string]
  traffic?: Record<string, [number, number][]> // a client's rate over time, in seconds and rps, replacing its own setting
  predictions?: Prediction[]
  debrief: string
}

/** Something that happened during the run, for markers on the charts (times in seconds, like the series). */
/** One bar of the latency distribution: successes up to leMs (the last bar has leMs 1e9, meaning 'and above'). */
export interface LatencyBucket {
  leMs: number
  count: number
}

export interface SimEvent {
  t: number
  type: string
  nodeId: string
  label: string
  lost?: number
}

export interface NodeStats {
  id: string
  arr: number
  ok: number
  err: number
  util: number
  busy: number
  cap: number
  qLen: number
  hitRate: number
  size?: number
  reasons?: Record<string, number>
  down?: boolean
  cut?: boolean // partitioned from the network
  primary?: boolean // a replica that took over as the primary
  instances?: number // autoscaled app server: servers running now
  phase?: 'detecting' | 'promoting' // failover step in progress on a crashed database
  phaseLeftSec?: number
  booting?: number // autoscaled app server: servers still booting
  slow?: number
  extra?: Record<string, number>
}
export interface EdgeStats {
  id: string
  rps: number
  breaker?: string
  ejected?: boolean
}
export interface SeriesPoint {
  t: number
  okRps: number
  errRps: number
  p50: number
  p95: number
  successRate: number
  inFlight: number
}
export interface JobStats {
  enqueued: number
  completed: number
  lost: number
  retried: number
  backlog: number
  p50: number
  p95: number
}
export interface RunStats {
  resolved: number
  pending: number
  ok: number
  failed: number
  p50: number
  p95: number
  p99: number
  failP50: number // median time to failure; p50/p95/p99 count successes only
  latencyBuckets: LatencyBucket[] // distribution of successful requests
  mean: number
  errors: Record<string, number>
  conflicts: number
  freshReads: number
  staleReads: number
  duplicates: number
  lostWrites: number
  jobs?: JobStats
}
export interface Stats {
  global: { now: number; inFlight: number; cost: number; avgCost: number; run: RunStats }
  nodes: Record<string, NodeStats>
  edges: Record<string, EdgeStats>
  series: SeriesPoint[]
  events: SimEvent[]
}

export interface Check {
  label: string
  target: string
  actual: string
  pass: boolean
}
export interface Score {
  pass: boolean
  stars: number
  checks: Check[]
  run: RunStats
  costPerHour: number
}

export type SimStatus = 'stopped' | 'running' | 'paused'

export interface Particle {
  e: string
  b?: boolean
  f?: boolean
}
export interface Notice {
  level: 'info' | 'warn' | 'error'
  text: string
}

/** One visit of a request to a node. Times are ms since the request was created. */
export interface Span {
  node: string
  label: string
  kind: string
  depth: number
  attempt: number
  at: number // arrived
  start: number // began being served (after any queueing)
  end: number // answered
  ok: boolean
  reason?: string
  open?: boolean // still running when the trace was read
  late?: boolean // began after the request had already resolved
}

export interface Trace {
  id: number
  client: string
  write: boolean
  key: number
  ok: boolean
  reason?: string
  latencyMs: number
  attempts: number
  spans: Span[]
}

export interface TraceSet {
  failed: Trace[]
  ok: Trace[]
}

/** One second of a component's history (for the timelines in the inspector). */
export interface NodePoint {
  t: number
  util: number
  qLen: number
  arr: number
  err: number
  instances?: number
}

export type ServerMsg =
  | { type: 'frame'; status: SimStatus; speed: number; stats?: Stats; particles?: Particle[]; notices?: Notice[]; diagram?: Diagram }
  | { type: 'result'; scenarioId: string; score: Score }
  | ({ type: 'traces' } & TraceSet)
  | { type: 'nodeSeries'; nodeId: string; points: NodePoint[] }
  | { type: 'error'; message: string }

export type ClientMsg =
  | { type: 'init'; diagram?: Diagram; scenarioId?: string; seed?: number; speed?: number }
  | { type: 'diagram'; diagram: Diagram }
  | { type: 'play' | 'pause' | 'step' | 'reset' }
  | { type: 'speed'; speed: number }
  | { type: 'seek'; t: number }
  | { type: 'traces' }
  | { type: 'nodeSeries'; nodeId: string }
  | { type: 'crash' | 'restart'; nodeId: string }

export interface QuestionSummary {
  id: string
  title: string
  mode: string
  difficulty: string
  summary: string
}
export interface QuestionField {
  key: string
  label: string
  unit: string
}
export interface Symptoms {
  durationSec: number
  series: SeriesPoint[]
  errors: Record<string, number>
  trace: Trace | null
}
export interface ChoiceResult {
  correct: boolean
  answer: number
  explain: string
  stars: number
}
export interface RunSummary {
  resolved: number
  success: number
  p50: number
  p95: number
  p99: number
  costPerHour: number
  lostWrites: number
  staleReadPct: number
}
export interface DefectOut {
  id: string
  nodes: string[]
  explain: string
}
export interface ReviewResult {
  found: DefectOut[]
  missed: DefectOut[]
  falseMarks: string[]
  precision: number
  recall: number
  score: number
  stars: number
}
export interface BuildResult {
  servers: number
  rightServers: number
  yours: RunSummary
  rightSized: RunSummary
  verdict: 'under' | 'right' | 'over'
}
export interface Question extends QuestionSummary {
  buildHint?: string
  diagram?: Diagram
  candidates?: { label: string; detail: string }[]
  prompt: string
  options?: string[]
  symptoms?: Symptoms
  facts?: { label: string; value: string }[]
  fields?: QuestionField[]
}
export interface EstimateFieldResult extends QuestionField {
  given: number
  expected: number
  verdict: 'good' | 'close' | 'off'
  explain: string
}
export interface EstimateResult {
  fields: EstimateFieldResult[]
  score: number
  stars: number
}
