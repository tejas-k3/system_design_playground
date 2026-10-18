import { describe, expect, it } from 'vitest'
import { MAX_ROWS } from './interview'
import { addRow, apiHints, blankEndpoint, blankTable, dataHints, removeRow, setRow } from './pads'

describe('list editing', () => {
  it('adds, edits and removes rows without touching the original', () => {
    const a = ['x']
    const b = addRow(a, 'y')
    expect(a).toEqual(['x'])
    expect(b).toEqual(['x', 'y'])
    expect(setRow(b, 1, 'z')).toEqual(['x', 'z'])
    expect(removeRow(b, 0)).toEqual(['y'])
  })
  it('stops adding at the row limit', () => {
    let l: string[] = []
    for (let i = 0; i < MAX_ROWS + 5; i++) l = addRow(l, String(i))
    expect(l).toHaveLength(MAX_ROWS)
  })
  it('ignores an edit or removal outside the list', () => {
    expect(setRow(['a'], 5, 'x')).toEqual(['a'])
    expect(removeRow(['a'], -1)).toEqual(['a'])
  })
})

describe('api hints', () => {
  const ep = (method: string, path: string, extra: Partial<ReturnType<typeof blankEndpoint>> = {}) => ({ ...blankEndpoint(), method, path, ...extra })
  it('asks about an idempotency key for writes that do not mention one', () => {
    const h = apiHints([ep('POST', '/payments'), ep('GET', '/payments/{id}')])
    expect(h).toEqual(['POST /payments changes data: what happens if the client retries it? Does it need an idempotency key?'])
  })
  it('is satisfied by the word idempotency or idempotent in the request or notes', () => {
    expect(apiHints([ep('POST', '/payments', { request: '{amount, Idempotency-Key header}' })])).toEqual([])
    expect(apiHints([ep('PUT', '/items/{id}', { notes: 'idempotent by design' })])).toEqual([])
  })
  it('asks for a path when a row has none, and leaves empty rows alone', () => {
    expect(apiHints([ep('GET', '')])).toEqual(['An endpoint has no path yet.'])
    expect(apiHints([])).toEqual([])
  })
})

describe('data hints', () => {
  it('asks for a key and the queries that matter', () => {
    const t = { ...blankTable(), name: 'orders' }
    expect(dataHints([t])).toEqual(['orders: which field is the key?', 'orders: which queries must be fast? Do they need an index?'])
    expect(dataHints([{ ...t, key: 'id', access: 'by customer' }])).toEqual([])
  })
  it('does not nag about unnamed rows', () => {
    expect(dataHints([blankTable()])).toEqual([])
  })
})
