import { MAX_ROWS, type Endpoint, type Table } from './interview'

export const addRow = <T,>(list: T[], row: T): T[] => (list.length >= MAX_ROWS ? list : [...list, row])
export const setRow = <T,>(list: T[], i: number, row: T): T[] => (i < 0 || i >= list.length ? list : list.map((x, j) => (j === i ? row : x)))
export const removeRow = <T,>(list: T[], i: number): T[] => (i < 0 || i >= list.length ? list : list.filter((_, j) => j !== i))

export const blankEndpoint = (): Endpoint => ({ method: 'GET', path: '', request: '', response: '', notes: '' })
export const blankTable = (): Table => ({ name: '', fields: '', key: '', indexes: '', access: '' })

/** Questions an interviewer would ask about the API sketch so far. */
export function apiHints(rows: Endpoint[]): string[] {
  const out: string[] = []
  for (const e of rows) {
    if (!e.path.trim()) {
      out.push('An endpoint has no path yet.')
      continue
    }
    if (e.method !== 'GET' && !/idempoten/i.test(e.request + ' ' + e.notes)) {
      out.push(`${e.method} ${e.path} changes data: what happens if the client retries it? Does it need an idempotency key?`)
    }
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
  }
  return out
}
