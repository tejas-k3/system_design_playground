import type { Diagram } from '../api/types'
import raw from './templates.json'

export interface Template {
  id: string
  title: string
  summary: string
  diagram: Diagram
}

/** The starter designs. The JSON is checked against the real component registry by the backend's tests. */
export const templates: Template[] = raw as unknown as Template[]

/** A fresh copy of a template's design, safe to edit. */
export function templateDiagram(id: string): Diagram | null {
  const t = templates.find((x) => x.id === id)
  return t ? structuredClone(t.diagram) : null
}
