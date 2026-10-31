import { describe, expect, it } from 'vitest'
import type { Finding, Score } from '../api/types'
import { emptyStats } from '../state/store'
import { EMPTY_MEMO, costPerThousand, loadMemo, memoText, onCallView, runEvidence, saveMemo } from './memo'

const run = (r: Partial<Score['run']>): Score['run'] => ({ ...emptyStats().global.run, ...r })
const score = (r: Partial<Score['run']>, over: Partial<Score> = {}): Score => ({
  pass: true, stars: 3, costPerHour: 1.2, run: run({ resolved: 3600, ok: 3564, p95: 180, ...r }),
  checks: [{ label: 'Success rate', target: '≥ 97.0%', actual: '99.0%', pass: true }], ...over,
})
const finding = (rule: string, severity: Finding['severity']): Finding => ({ rule, severity, message: rule, fix: '', nodes: [] })

describe('costPerThousand', () => {
  it('divides the hourly cost by the thousands of requests in an hour', () => {
    // 3,600 requests in 60 s is 216,000 an hour; $1.20 an hour is 0.0056 per thousand
    expect(costPerThousand(score({}), 60)).toBeCloseTo(0.0056, 4)
  })
  it('is unknown without requests or time', () => {
    expect(costPerThousand(score({ resolved: 0 }), 60)).toBeNull()
    expect(costPerThousand(score({}), 0)).toBeNull()
  })
})

describe('runEvidence', () => {
  it('states what the run measured, and only the failure behaviours that happened', () => {
    const lines = runEvidence(score({ stuck: 4, unbalanced: 7, inDoubt: 0, unchecked: 12, errors: { 'Timeouts': 30, 'Queue full (503)': 6 } }), 60)
    expect(lines.join('\n')).toContain('$1.20 an hour')
    expect(lines.join('\n')).toContain('p95 of 180 ms')
    expect(lines.join('\n')).toContain('99.0% of requests succeeded')
    expect(lines.join('\n')).toContain('Orders left stuck: 4.')
    expect(lines.join('\n')).toContain('Books: entries that do not balance (these include the half-done orders): 7.')
    expect(lines.join('\n')).toContain('Requests let through unchecked: 12.')
    expect(lines.join('\n')).not.toContain('dead coordinator')
    expect(lines.join('\n')).toContain('Most failures were Timeouts (30)')
  })
})

describe('onCallView', () => {
  it('counts warnings and errors as alerts and the error budget a run used', () => {
    const v = onCallView(score({ resolved: 1000, ok: 985 }), [finding('spof-server', 'warn'), finding('no-call-timeout', 'warn'), finding('x', 'info')], 0.97)
    expect(v.alerts).toEqual(['spof-server', 'no-call-timeout']) // the findings' messages
    expect(v.budgetUsedPct).toBe(50) // 1.5% failed of the 3% the goal allows
  })
  it('has no budget when no success target is given, and a goal of 100% allows no failure', () => {
    expect(onCallView(score({}), []).budgetUsedPct).toBeNull()
    expect(onCallView(score({ resolved: 10, ok: 10 }), [], 1).budgetUsedPct).toBe(0)
    expect(onCallView(score({ resolved: 10, ok: 9 }), [], 1).budgetUsedPct).toBe(100)
  })
})

describe('memos', () => {
  it('round-trips through storage and survives a broken one', () => {
    const store = new Map<string, string>()
    const storage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v) } as Storage
    expect(loadMemo('c1', storage)).toEqual(EMPTY_MEMO)
    saveMemo('c1', { design: 'a', cost: 'b', watch: 'c' }, storage)
    expect(loadMemo('c1', storage)).toEqual({ design: 'a', cost: 'b', watch: 'c' })
    const broken = { getItem: () => { throw new Error('blocked') }, setItem: () => { throw new Error('blocked') } } as unknown as Storage
    expect(loadMemo('c1', broken)).toEqual(EMPTY_MEMO)
    expect(saveMemo('c1', EMPTY_MEMO, broken)).toBe(false)
  })
  it('writes the memo as text with the evidence under it', () => {
    const text = memoText('Slow news', { design: 'Gossip every second', cost: 'More messages', watch: 'Time to reach all nodes' }, ['Cost: $1 an hour'], [{ text: 'Alerts: two' }, { text: 'A', sub: true }])
    expect(text).toContain('Trade-off memo: Slow news')
    expect(text).toContain('Chosen design: Gossip every second')
    expect(text).toContain('What it costs: More messages')
    expect(text).toContain('What to monitor: Time to reach all nodes')
    expect(text).toContain('- Cost: $1 an hour')
    expect(text).toContain('- Alerts: two')
    expect(text).toContain('  - A')
  })
})
