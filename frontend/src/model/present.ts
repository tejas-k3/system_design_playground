import { calculate, humanBytes, humanCount, humanRate } from './estimate'
import { RUBRIC_AREAS, type InterviewData } from './interview'
import type { StageId } from './timer'

export type Slide =
  | { kind: 'lists'; groups: { title: string; items: string[] }[] }
  | { kind: 'facts'; rows: [string, string][] }
  | { kind: 'table'; head: string[]; rows: string[][] }
  | { kind: 'canvas' }

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

/** What the present view shows for a stage: the learner's own entries, read-only and large. The canvas stages show the canvas itself. */
export function presentSlide(stage: StageId, d: InterviewData): Slide {
  switch (stage) {
    case 'requirements':
      return {
        kind: 'lists',
        groups: [
          { title: 'Functional', items: d.functional.filter((f) => f.trim()) },
          { title: 'Non-functional', items: d.nonFunctional.filter((r) => r.label.trim()).map((r) => `${r.label}: ${r.target === null ? 'no number yet' : `${r.target}${r.unit ? ' ' + r.unit : ''}`}`) },
        ],
      }
    case 'estimation': {
      const e = calculate(d.estimate)
      return {
        kind: 'facts',
        rows: [
          ['Daily users', humanCount(e.dau)],
          ['Average load', humanRate(e.avgQps)],
          ['Peak load', humanRate(e.peakQps)],
          ['Reads / writes (average)', `${humanRate(e.readQps)} / ${humanRate(e.writeQps)}`],
          ['Storage kept', humanBytes(e.storageBytes)],
          ['Peak bandwidth', Number.isFinite(e.peakGbps) ? `${e.peakGbps.toFixed(2)} Gbit/s` : '—'],
          ['App servers needed', Number.isFinite(e.servers) ? String(e.servers) : '—'],
        ],
      }
    }
    case 'api':
      return { kind: 'table', head: ['Method', 'Path', 'In', 'Out', 'Notes'], rows: d.api.map((e) => [e.method, e.path, e.request, e.response, e.notes]) }
    case 'data':
      return { kind: 'table', head: ['Table', 'Fields', 'Key', 'Indexes', 'Queries'], rows: d.tables.map((t) => [t.name, t.fields, t.key, t.indexes, t.access]) }
    case 'design':
    case 'deepdive':
      return { kind: 'canvas' }
    case 'tradeoffs':
      return {
        kind: 'lists',
        groups: [
          { title: 'Self-assessment', items: RUBRIC_AREAS.map((a) => `${cap(a)}: ${d.rubric[a] > 0 ? `${d.rubric[a]} / 5` : 'not rated'}`) },
          { title: 'Notes', items: d.notes.trim() ? d.notes.trim().split('\n') : [] },
        ],
      }
  }
}
