import type { Diagram } from '../api/types'
import { calculate, estimateNotes } from './estimate'
import { RUBRIC_AREAS, type InterviewData } from './interview'
import { settingRows } from './review'
import type { Reg } from './diagram'

const cell = (s: string) => s.replace(/\|/g, '\\|').replace(/\r?\n/g, ' ').trim()
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

export interface ExportOptions {
  title: string
  date: string // already formatted, e.g. 2026-10-18
  image?: string // file name or data URL of the diagram picture
}

/** The design write-up as Markdown: requirements, estimates, API, data model, design, self-assessment, notes. */
export function buildMarkdown(d: InterviewData, diagram: Diagram, reg: Reg, o: ExportOptions): string {
  const out: string[] = [`# ${o.title}`, '', `_System design practice, ${o.date}_`, '']

  out.push('## Requirements', '')
  out.push('**Functional**', '')
  out.push(...(d.functional.length ? d.functional.map((f) => `- ${f}`) : ['- (none written)']), '')
  out.push('**Non-functional**', '')
  out.push(...(d.nonFunctional.length ? d.nonFunctional.map((r) => `- ${r.label}: ${r.target === null ? '(no number yet)' : `${r.target}${r.unit ? ' ' + r.unit : ''}`}`) : ['- (none written)']), '')

  out.push('## Estimates', '', ...estimateNotes(calculate(d.estimate)).split('\n').slice(1).map((l) => `- ${l}`), '')

  out.push('## API', '')
  if (d.api.length) out.push('| Method | Path | Request | Response | Notes |', '|---|---|---|---|---|', ...d.api.map((e) => `| ${cell(e.method)} | ${cell(e.path)} | ${cell(e.request)} | ${cell(e.response)} | ${cell(e.notes)} |`), '')
  else out.push('(no endpoints written)', '')

  out.push('## Data model', '')
  if (d.tables.length) out.push('| Table | Fields | Key | Indexes | Queries |', '|---|---|---|---|---|', ...d.tables.map((t) => `| ${cell(t.name)} | ${cell(t.fields)} | ${cell(t.key)} | ${cell(t.indexes)} | ${cell(t.access)} |`), '')
  else out.push('(no tables written)', '')

  out.push('## Design', '')
  if (o.image) out.push(`![Design](${o.image})`, '')
  if (diagram.nodes.length === 0) out.push('(the canvas is empty)', '')
  else {
    out.push('Components:', '')
    for (const n of diagram.nodes) {
      const kind = reg.spec(n.kind)?.name ?? n.kind
      const set = settingRows(diagram, n, reg).filter((r) => r.changed).map((r) => `${r.label.toLowerCase()} ${r.value}`)
      out.push(`- ${n.label}${n.label === kind ? '' : ` (${kind})`}${set.length ? ': ' + set.join(', ') : ''}`)
    }
    const name = (id: string) => diagram.nodes.find((n) => n.id === id)?.label ?? id
    out.push('', 'Connections:', '', ...(diagram.edges.length ? diagram.edges.map((e) => `- ${name(e.from)} → ${name(e.to)}`) : ['- (none)']), '')
  }

  out.push('## Self-assessment', '', '| Area | Rating |', '|---|---|')
  for (const a of RUBRIC_AREAS) out.push(`| ${cap(a)} | ${d.rubric[a] > 0 ? `${d.rubric[a]} / 5` : 'not rated'} |`)
  out.push('', '## Notes', '', d.notes.trim() || '(none)', '')
  return out.join('\n')
}

/** A file-name-safe stamp such as 2026-10-18 for the downloads. */
export const fileStamp = (d: Date): string => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
