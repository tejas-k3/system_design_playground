import type { Diagram, Finding, Registry, Scenario, ScenarioSummary } from './types'

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
