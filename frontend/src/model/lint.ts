import type { Finding } from '../api/types'

export type Severity = Finding['severity']
const ORDER: Severity[] = ['error', 'warn', 'info']

export function severityLabel(s: Severity): string {
  return s === 'error' ? 'Error' : s === 'warn' ? 'Warning' : 'Note'
}

/** Findings grouped by severity, worst first; empty groups left out. */
export function groupFindings(fs: Finding[]): { severity: Severity; items: Finding[] }[] {
  return ORDER.map((severity) => ({ severity, items: fs.filter((f) => f.severity === severity) })).filter((g) => g.items.length > 0)
}

export function summarise(fs: Finding[]): string {
  if (fs.length === 0) return 'No findings'
  const word = { error: 'error', warn: 'warning', info: 'note' }
  return groupFindings(fs)
    .map((g) => `${g.items.length} ${word[g.severity]}${g.items.length === 1 ? '' : 's'}`)
    .join(', ')
}
