import { describe, expect, it } from 'vitest'
import type { Finding } from '../api/types'
import { groupFindings, severityLabel, summarise } from './lint'

const f = (rule: string, severity: Finding['severity'], nodes: string[] = ['a']): Finding => ({ rule, severity, message: rule, fix: 'fix', nodes })

describe('lint helpers', () => {
  it('orders groups errors first, then warnings, then notes, keeping the server order inside a group', () => {
    const g = groupFindings([f('w1', 'warn'), f('i1', 'info'), f('e1', 'error'), f('w2', 'warn')])
    expect(g.map((x) => x.severity)).toEqual(['error', 'warn', 'info'])
    expect(g[1].items.map((x) => x.rule)).toEqual(['w1', 'w2'])
  })
  it('omits empty groups', () => {
    expect(groupFindings([f('w', 'warn')]).map((x) => x.severity)).toEqual(['warn'])
    expect(groupFindings([])).toEqual([])
  })
  it('names severities in words so colour is never the only cue', () => {
    expect(severityLabel('error')).toBe('Error')
    expect(severityLabel('warn')).toBe('Warning')
    expect(severityLabel('info')).toBe('Note')
  })
  it('summarises with plural-aware text', () => {
    expect(summarise([])).toBe('No findings')
    expect(summarise([f('a', 'error')])).toBe('1 error')
    expect(summarise([f('a', 'error'), f('b', 'error'), f('c', 'warn'), f('d', 'info')])).toBe('2 errors, 1 warning, 1 note')
  })
})
