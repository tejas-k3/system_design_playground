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
  const fails = { notes: '404 if missing, 429 when rate limited' } // an endpoint that says what happens on failure
  const NO_ERRORS = 'No endpoint says what it returns when it fails: which status codes (400, 404, 409, 429) and error body?'
  it('asks about an idempotency key for a POST that does not mention one', () => {
    const h = apiHints([ep('POST', '/payments', fails), ep('GET', '/payments/{id}')])
    expect(h).toEqual(['POST /payments changes data: what happens if the client retries it? Does it need an idempotency key?'])
  })
  it('is satisfied by the word idempotency or idempotent in the request or notes', () => {
    expect(apiHints([ep('POST', '/payments', { request: '{amount, Idempotency-Key header}', response: '201, 409 on a repeat' })])).toEqual([])
    expect(apiHints([ep('POST', '/payments', { notes: 'idempotent by design; 400 on a bad amount' })])).toEqual([])
  })
  it('does not ask a PUT or DELETE for a key: repeating them has the same effect by definition', () => {
    expect(apiHints([ep('PUT', '/items/{id}', fails), ep('DELETE', '/items/{id}', fails)])).toEqual([])
  })
  it('asks how a list is paged, but not for one item or when paging is mentioned', () => {
    const ask = 'GET /posts returns a list: how is it paged? A limit and a cursor stop one call returning everything.'
    expect(apiHints([ep('GET', '/posts', fails)])).toEqual([ask])
    expect(apiHints([ep('GET', '/posts/{id}', fails)])).toEqual([])
    expect(apiHints([ep('GET', '/posts/42', fails)])).toEqual([])
    expect(apiHints([ep('GET', '/posts?limit=20&cursor=abc', fails)])).toEqual([])
    expect(apiHints([ep('GET', '/posts', { response: '{items, next_cursor}', notes: '404 if the user is gone' })])).toEqual([])
    expect(apiHints([ep('GET', '/users/{id}/posts', { ...fails, request: 'page, per_page' })])).toEqual([])
  })
  it('asks once, for the whole API, when no endpoint says what a failure looks like', () => {
    expect(apiHints([ep('GET', '/posts/{id}'), ep('PUT', '/posts/{id}')])).toEqual([NO_ERRORS])
    expect(apiHints([ep('GET', '/posts/{id}'), ep('PUT', '/posts/{id}', { response: '200, or 409 on a clash' })])).toEqual([])
    expect(apiHints([ep('GET', '/posts/{id}', { response: '{error: string}' })])).toEqual([])
  })
  it('does not take a path such as /status for a description of failures', () => {
    expect(apiHints([ep('GET', '/status/{id}')])).toEqual([NO_ERRORS])
    expect(apiHints([ep('GET', '/status/{id}', { notes: 'returns 404 for an unknown id' })])).toEqual([])
  })
  it('asks for a path when a row has none, and leaves empty rows alone', () => {
    expect(apiHints([ep('GET', '')])).toEqual(['An endpoint has no path yet.'])
    expect(apiHints([])).toEqual([])
  })
})

describe('data hints', () => {
  const table = (extra: Partial<ReturnType<typeof blankTable>>) => ({ ...blankTable(), name: 'users', ...extra })
  it('asks for a key and the queries that matter', () => {
    const t = { ...blankTable(), name: 'orders' }
    expect(dataHints([t])).toEqual(['orders: which field is the key?', 'orders: which queries must be fast? Do they need an index?'])
    expect(dataHints([{ ...t, key: 'id', access: 'by customer' }])).toEqual([])
  })
  it('does not nag about unnamed rows', () => {
    expect(dataHints([blankTable()])).toEqual([])
  })
  it('flags a lookup by a field that is neither the key nor indexed', () => {
    const t = table({ fields: 'id, email, name', key: 'id', access: 'look up by email at login' })
    expect(dataHints([t])).toEqual(['users: you look up by email, but the key is id and no index lists email, so each lookup scans the whole table.'])
  })
  it('accepts the field when it is in the key, in a composite key or in an index', () => {
    expect(dataHints([table({ fields: 'id, email', key: 'email', access: 'by email' })])).toEqual([])
    expect(dataHints([table({ fields: 'user_id, created_at, text', key: 'user_id, created_at', access: 'by user_id, newest first' })])).toEqual([])
    expect(dataHints([table({ fields: 'id, email', key: 'id', indexes: 'email (unique)', access: 'by email' })])).toEqual([])
  })
  it('only judges words that are fields of the table, and says each field once', () => {
    expect(dataHints([table({ fields: 'id, name', key: 'id', access: 'by customer' })])).toEqual([])
    const t = table({ fields: 'id, email', key: 'id', access: 'by email; also by email domain' })
    expect(dataHints([t])).toHaveLength(1)
  })
})
