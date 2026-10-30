import { MAX_ROWS, type Endpoint, type Table } from './interview'

export const addRow = <T,>(list: T[], row: T): T[] => (list.length >= MAX_ROWS ? list : [...list, row])
export const setRow = <T,>(list: T[], i: number, row: T): T[] => (i < 0 || i >= list.length ? list : list.map((x, j) => (j === i ? row : x)))
export const removeRow = <T,>(list: T[], i: number): T[] => (i < 0 || i >= list.length ? list : list.filter((_, j) => j !== i))

export const blankEndpoint = (): Endpoint => ({ method: 'GET', path: '', request: '', response: '', notes: '' })
export const blankTable = (): Table => ({ name: '', fields: '', key: '', indexes: '', access: '' })

const words = (text: string): Set<string> => new Set(text.toLowerCase().split(/\W+/).filter(Boolean))
const mentions = (e: Endpoint) => `${e.path} ${e.request} ${e.response} ${e.notes}`
const describes = (e: Endpoint) => `${e.request} ${e.response} ${e.notes}` // what the learner wrote about it, without the path

const PAGING = /\b(page|pages|paged|paging|pagination|limit|cursor|offset|per_page)\b|next_?(cursor|page|token)/i
const FAILURE = /\b[45]\d\d\b|\b[45]xx\b|\berror|\bstatus\b/i

/** A GET whose path ends in a name rather than an id returns a collection. */
const returnsList = (e: Endpoint): boolean => {
  if (e.method !== 'GET') return false
  const last = e.path.split('?')[0].split('/').filter(Boolean).pop() ?? ''
  return last !== '' && !/^[{:<[]|^\d+$/.test(last)
}

/** Questions an interviewer would ask about the API sketch so far. */
export function apiHints(rows: Endpoint[]): string[] {
  const out: string[] = []
  for (const e of rows) {
    if (!e.path.trim()) {
      out.push('An endpoint has no path yet.')
      continue
    }
    // PUT and DELETE repeat harmlessly by definition; a POST is where a retry creates a second thing
    if (e.method === 'POST' && !/idempoten/i.test(e.request + ' ' + e.notes)) {
      out.push(`${e.method} ${e.path} changes data: what happens if the client retries it? Does it need an idempotency key?`)
    }
    if (returnsList(e) && !PAGING.test(mentions(e))) {
      out.push(`${e.method} ${e.path} returns a list: how is it paged? A limit and a cursor stop one call returning everything.`)
    }
  }
  const sketched = rows.filter((e) => e.path.trim())
  if (sketched.length > 0 && !sketched.some((e) => FAILURE.test(describes(e)))) {
    out.push('No endpoint says what it returns when it fails: which status codes (400, 404, 409, 429) and error body?')
  }
  return out
}

export function dataHints(rows: Table[]): string[] {
  const out: string[] = []
  for (const t of rows) {
    const name = t.name.trim()
    if (!name) continue
    if (!t.key.trim()) out.push(`${name}: which field is the key?`)
    if (!t.access.trim()) out.push(`${name}: which queries must be fast? Do they need an index?`)
    out.push(...unservedLookups(name, t))
  }
  return out
}

/** Lookups "by <field>" on a field of the table that the key and the indexes do not cover, so each one scans the table. */
function unservedLookups(name: string, t: Table): string[] {
  const fields = words(t.fields)
  const covered = new Set([...words(t.key), ...words(t.indexes)])
  const asked = new Set([...t.access.matchAll(/\bby\s+([a-z_]\w*)/gi)].map((m) => m[1].toLowerCase()))
  return [...asked]
    .filter((f) => fields.has(f) && !covered.has(f))
    .map((f) => `${name}: you look up by ${f}, but the key is ${t.key.trim() || 'not set'} and no index lists ${f}, so each lookup scans the whole table.`)
}
