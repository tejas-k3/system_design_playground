import type { Alternative, BuildResult, ChoiceResult, Diagram, ReviewResult, RunSummary, EstimateResult, Finding, PredictionAnswer, Question, QuestionSummary, Stats, SweepResponse, Registry, Scenario, ScenarioSummary } from './types'

async function json<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = res.statusText
    try {
      msg = ((await res.json()) as { error?: string }).error ?? msg
    } catch {
      /* not json */
    }
    throw new Error(msg)
  }
  return (await res.json()) as T
}

export const fetchRegistry = () => fetch('/api/registry').then((r) => json<Registry>(r))
export const fetchScenarios = () => fetch('/api/scenarios').then((r) => json<ScenarioSummary[]>(r))
export const fetchScenario = (id: string) => fetch(`/api/scenarios/${encodeURIComponent(id)}`).then((r) => json<Scenario>(r))
export const fetchAlternatives = (id: string) => fetch(`/api/scenarios/${encodeURIComponent(id)}/alternatives`).then((r) => json<Alternative[]>(r))
export const fetchHint = (id: string, level: number) => fetch(`/api/scenarios/${encodeURIComponent(id)}/hints/${level}`).then((r) => json<{ level: number; hint: string }>(r))
export const fetchDebrief = (id: string) => fetch(`/api/scenarios/${encodeURIComponent(id)}/debrief`).then((r) => json<{ debrief: string; deeper?: string }>(r))
/** The full answer, kept apart from the hints (the server sends it as `answer`). An older server has no such route and answers 404. */
export const fetchAnswerHint = (id: string) => fetch(`/api/scenarios/${encodeURIComponent(id)}/answer-hint`).then((r) => json<{ answer?: string; hint?: string }>(r)).then((d) => ({ hint: d.answer ?? d.hint }))
export const answerPrediction = (id: string, predictionId: string, choice: number) =>
  fetch(`/api/scenarios/${encodeURIComponent(id)}/predictions/${encodeURIComponent(predictionId)}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ choice }) }).then((r) => json<PredictionAnswer>(r))
export const fetchSolution = (id: string) => fetch(`/api/scenarios/${encodeURIComponent(id)}/solution`).then((r) => json<Diagram>(r))

/** Store a diagram on the server and return the id that opens it. */
export async function saveDesign(diagram: Diagram): Promise<string> {
  const res = await fetch('/api/designs', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ diagram }) })
  return (await json<{ id: string }>(res)).id
}

export async function fetchDesign(id: string): Promise<Diagram> {
  return (await json<{ diagram: Diagram }>(await fetch(`/api/designs/${encodeURIComponent(id)}`))).diagram
}

export async function validate(diagram: Diagram): Promise<{ diagram: Diagram; adjusted: boolean; costPerHour: number; findings: Finding[] }> {
  const res = await fetch('/api/validate', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ diagram }) })
  return json(res)
}

export async function sweepLoad(diagram: Diagram, loads: number[], durationSec = 20): Promise<SweepResponse> {
  const res = await fetch('/api/sweep', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ diagram, loads, durationSec }) })
  return json(res)
}

export interface RunResult {
  stats: Stats
  costPerHour: number
  truncated?: boolean
}

/** A headless run of a design (the scenario's own traffic and chaos if a scenario id is given). */
export async function runDiagram(diagram: Diagram, scenarioId?: string, durationSec = 30, seed = 42): Promise<RunResult> {
  const res = await fetch('/api/run', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ diagram, scenarioId, durationSec, seed }) })
  return json(res)
}

export async function sweepParam(diagram: Diagram, nodeId: string, key: string, values: number[], durationSec = 20): Promise<SweepResponse> {
  const res = await fetch('/api/sweep', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ diagram, param: { nodeId, key }, values, durationSec }) })
  return json(res)
}

export const fetchQuestions = () => fetch('/api/questions').then((r) => json<QuestionSummary[]>(r))
export const fetchQuestion = (id: string) => fetch(`/api/questions/${encodeURIComponent(id)}`).then((r) => json<Question>(r))
export async function answerEstimate(id: string, answers: Record<string, number>): Promise<EstimateResult> {
  const res = await fetch(`/api/questions/${encodeURIComponent(id)}/answer`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ answers }) })
  return json(res)
}

export async function answerChoice(id: string, choice: number): Promise<ChoiceResult> {
  const res = await fetch(`/api/questions/${encodeURIComponent(id)}/answer`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ choice }) })
  return json(res)
}

export async function runCandidate(id: string, candidate: number): Promise<RunSummary> {
  const res = await fetch(`/api/questions/${encodeURIComponent(id)}/run`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ candidate }) })
  return json(res)
}

export async function submitReview(id: string, marked: string[]): Promise<ReviewResult> {
  const res = await fetch(`/api/questions/${encodeURIComponent(id)}/review`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ marked }) })
  return json(res)
}

export async function buildEstimate(id: string, answers: Record<string, number>): Promise<BuildResult> {
  const res = await fetch(`/api/questions/${encodeURIComponent(id)}/build`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ answers }) })
  return json(res)
}
