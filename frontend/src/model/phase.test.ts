import { describe, expect, it } from 'vitest'
import type { NodeStats } from '../api/types'
import { phaseChip } from './phase'

const ns = (extra: Partial<NodeStats>): NodeStats => ({ id: 'x', arr: 0, ok: 0, err: 0, util: 0, busy: 0, qLen: 0, hitRate: -1, ...extra }) as NodeStats

describe('phaseChip', () => {
  it('counts down the failover steps in whole seconds, rounding up so it never shows 0 while waiting', () => {
    expect(phaseChip(ns({ phase: 'detecting', phaseLeftSec: 3.2 }))).toBe('NOTICING 4 s')
    expect(phaseChip(ns({ phase: 'promoting', phaseLeftSec: 7.9 }))).toBe('PROMOTING 8 s')
    expect(phaseChip(ns({ phase: 'promoting', phaseLeftSec: 0.1 }))).toBe('PROMOTING 1 s')
  })
  it('counts servers that are still booting', () => {
    expect(phaseChip(ns({ booting: 1 }))).toBe('1 booting')
    expect(phaseChip(ns({ booting: 3 }))).toBe('3 booting')
  })
  it('is null when nothing is in progress', () => {
    expect(phaseChip(ns({}))).toBeNull()
    expect(phaseChip(undefined)).toBeNull()
  })
  it('prefers the failover step if both somehow apply', () => {
    expect(phaseChip(ns({ phase: 'detecting', phaseLeftSec: 1, booting: 2 }))).toBe('NOTICING 1 s')
  })
})
