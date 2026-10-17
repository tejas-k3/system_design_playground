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
}

export interface Scenario extends ScenarioSummary {
  story: string
  diagram: Diagram
  goals: Goals
  durationSec: number
  warmupSec: number
  speed: number
  hints: [string, string, string]
  predictions?: Prediction[]
  debrief: string
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
  mean: number
  errors: Record<string, number>
  conflicts: number
  freshReads: number
  staleReads: number
  duplicates: number
  jobs?: JobStats
}
export interface Stats {
  global: { now: number; inFlight: number; cost: number; run: RunStats }
  nodes: Record<string, NodeStats>
  edges: Record<string, EdgeStats>
  series: SeriesPoint[]
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

export type ServerMsg =
  | { type: 'frame'; status: SimStatus; speed: number; stats?: Stats; particles?: Particle[]; notices?: Notice[] }
  | { type: 'result'; scenarioId: string; score: Score }
  | { type: 'error'; message: string }

export type ClientMsg =
  | { type: 'init'; diagram?: Diagram; scenarioId?: string; seed?: number; speed?: number }
  | { type: 'diagram'; diagram: Diagram }
  | { type: 'play' | 'pause' | 'step' | 'reset' }
  | { type: 'speed'; speed: number }
  | { type: 'crash' | 'restart'; nodeId: string }
