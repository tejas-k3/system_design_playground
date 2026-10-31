import { describe, expect, it } from 'vitest'
import type { Finding } from '../api/types'
import { guaranteeNote } from './guaranteeNote'

const finding = (rule: string, message = 'm', fix = 'f'): Finding => ({ rule, severity: 'warn', message, fix, nodes: [] })

describe('guaranteeNote', () => {
  it('shows what the design lint said and how to fix it, when the challenge has the guarantees goal', () => {
    const f = [finding('spof-server'), finding('guarantee-not-judged', 'Only judged on a key-value store.', 'Use a key-value store.')]
    expect(guaranteeNote(f, 0)).toBe('Only judged on a key-value store. Use a key-value store.')
  })
  it('says nothing without that finding', () => {
    expect(guaranteeNote([finding('spof-server')], 0)).toBeNull()
    expect(guaranteeNote([], 0)).toBeNull()
  })
  it('says nothing when the challenge has no guarantees goal', () => {
    expect(guaranteeNote([finding('guarantee-not-judged')], undefined)).toBeNull()
  })
})
