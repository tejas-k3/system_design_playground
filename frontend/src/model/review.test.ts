import { describe, expect, it } from 'vitest'
import type { Diagram, Node } from '../api/types'
import { reg } from '../test/fixtures'
import { percent, settingRows } from './review'

const node = (kind: string, params: Node['params']): Node => ({ id: 'n', kind, x: 0, y: 0, label: 'N', params })
const dia = (nodes: Node[], edges: Diagram['edges'] = []): Diagram => ({ nodes, edges, notes: [] }) as unknown as Diagram

describe('settingRows', () => {
  it('lists every visible setting with its value, defaults included, and marks the ones that were changed', () => {
    const n = node('appServer', { threads: 16, order: 'lifo' })
    expect(settingRows(dia([n]), n, reg)).toEqual([
      { label: 't', value: '16', changed: true },
      { label: 'o', value: 'Newest first', changed: true },
      { label: 'c', value: 'off', changed: false },
    ])
  })
  it('shows a default value as what it is, so a missing safeguard can be seen', () => {
    const n = node('appServer', {})
    const rows = settingRows(dia([n]), n, reg)
    expect(rows.find((r) => r.label === 'c')).toEqual({ label: 'c', value: 'off', changed: false })
    expect(rows.every((r) => !r.changed)).toBe(true)
  })
  it('leaves out settings that only apply with a connection the component does not have', () => {
    const app = node('appServer', {})
    const db: Node = { id: 'db', kind: 'database', x: 0, y: 0, label: 'DB', params: {} }
    const without = settingRows(dia([app, db]), app, reg).map((r) => r.label)
    const withEdge = settingRows(dia([{ ...app, id: 'a' }, db], [{ id: 'a>db', from: 'a', to: 'db', latencyMs: 5 }]), { ...app, id: 'a' }, reg).map((r) => r.label)
    expect(without).not.toContain('p')
    expect(withEdge).toContain('p')
  })
})

describe('percent', () => {
  it('rounds a ratio to a whole percent', () => {
    expect(percent(0.75)).toBe('75%')
    expect(percent(0.3333)).toBe('33%')
    expect(percent(0)).toBe('0%')
    expect(percent(1)).toBe('100%')
  })
})
