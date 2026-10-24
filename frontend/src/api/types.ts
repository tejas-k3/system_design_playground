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
  bandwidthMbps?: number // 0 / absent = unlimited; only matters once the users set request or response sizes
  lossPct?: number // chance a packet is lost per trip: retransmitted after 200 ms, six in a row abort the connection
  jitterMs?: number // random 0..jitterMs added to each one-way trip
  protocol?: string // http | grpc | tcp | ws | sql | custom: a label for the reader, ignored by the simulation
  async?: boolean // drawn dashed with an open arrowhead: a fire-and-forget call, also only a label
}
export interface Note {
  id: string
  x: number
  y: number
  text: string
  fontSize?: number
}
export type GroupKind = 'az' | 'region' | 'vpc' | 'service' | 'note'
/** A labelled box drawn behind the components. Visual only: the simulation ignores it. Members are decided by geometry. */
export interface Group {
  id: string
  kind: GroupKind
  label: string
  x: number
  y: number
  w: number
  h: number
  collapsed?: boolean // reserved; the editor does not collapse yet
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
  groups?: Group[] // absent in older designs
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
  maxLostUpdates?: number
  maxSessionViolations?: number
  maxLostEvents?: number
  maxInconsistent?: number
  maxInDoubt?: number
  maxLateDropped?: number
  maxStateKB?: number
  maxTTFTMs?: number
  minRecall?: number
  minTopKRecall?: number
  maxCountErrorPct?: number
  maxResultDelaySec?: number
  minWorstTenant?: number
  minDeliveriesPerMessage?: number
  maxRpoSec?: number
  maxRtoSec?: number
  maxWriteSkew?: number
  maxJobsLost?: number
  minCustomerSuccess?: number
  starCostPerHour?: number
  starP95Ms?: number
}

/** A prediction question as the page first loads it: the answer and the explanation come only after a choice. */
export interface Prediction {
  id: string
  question: string
  options: string[]
}

/** What the server says to a choice: the right answer, and why. */
export interface PredictionAnswer {
  choice: number
  correct: boolean
  answer: number
  explain: string
}

export interface Alternative {
  label: string
  changes: { node: string; param: string; value: unknown }[]
  pass: boolean
  successPct: number
  p95Ms: number
  costPerHour: number
  why: string
}

export interface ScenarioSummary {
  id: string
  title: string
  difficulty: string
  summary: string
  configOnly?: boolean
  open?: boolean // a blank canvas with requirements; the load is hidden and there is no solution to show
  concepts?: string[] // the ideas it practises (see model/skills.ts)
}

export interface Scenario extends ScenarioSummary {
  story: string
  diagram: Diagram
  goals: Goals
  durationSec: number
  warmupSec: number
  speed: number
  hintCount: number // the hints themselves are fetched one at a time, when asked for
  traffic?: Record<string, [number, number][]> // a client's rate over time, in seconds and rps, replacing its own setting
  predictions?: Prediction[]
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
  mbps?: number
  util?: number
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
  orderViolations?: number // messages a log consumer finished after a newer message of the same key
  highP95?: number // p95 of the urgent jobs
  redelivered?: number // handed out again because the first worker did not acknowledge in time
  dead?: number // parked in the dead-letter queue after their last attempt
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
  writeSkew?: number // on-call pairs left with the rule broken (two doctors off at once)
  fanoutWrites?: number // timeline inserts caused by posts (feed workload)
  fanoutReads?: number // followed authors read to build timelines
  hedged?: number // second queries sent because a shard was slow
  hottestKeyShare?: number // share of requests that go to the single busiest item
  lightP95?: number // p95 of the light requests when the users send heavy ones too
  heavyP95?: number
  customerOk?: number // requests of real customers that succeeded (when the diagram has a bot)
  customerTotal?: number
  highOk?: number // urgent requests that succeeded
  highTotal?: number // urgent requests resolved
  deadlineSkipped?: number // requests skipped in a queue because their client had already given up
  partial?: number // answers sent with fewer shards than required
  tenants?: number
  worstTenantSuccess?: number // the worst-served of the quiet tenants
  inconsistent?: number // multi-step writes left half done
  compensated?: number // multi-step writes rolled back
  eventDups?: number // events a queue accepted a second time (a producer retried after a lost answer)
  inDoubt?: number // two-phase commits left waiting for a crashed coordinator
  lateDropped?: number // events a stream job dropped for arriving too late
  recall?: number // share of the true nearest neighbours the vector search finds
  ttftMs?: number // time to first token of LLM answers (p95)
  itlMs?: number // gap between tokens of LLM answers (p95)
  topKRecall?: number // share of the true ten most popular items a stream job found
  stateKB?: number // memory a stream job needs to count its different visitors
  countErrorPct?: number // error of an approximate visitor count
  resultDelaySec?: number // seconds after a window ends before a stream job first emits it
  blockedSec?: number // lock-seconds those transactions held
  stuck?: number // rolled back unsuccessfully
  eventsLost?: number // saved writes whose event never reached the queue
  rpoSec?: number // seconds of writes the last restore lost
  rtoSec?: number // seconds the database was unavailable restoring
  sessionReads?: number
  readYourWritesBroken?: number // reads older than the same client's own acknowledged write
  monotonicBroken?: number // reads older than one the same client already had
  lostUpdates?: number // transfers that silently overwrote a concurrent commit (read committed)
  txAborts?: number // transactions aborted by a write conflict or as a deadlock victim
  deadlocks?: number
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
