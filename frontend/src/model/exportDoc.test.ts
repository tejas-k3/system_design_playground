import { describe, expect, it } from 'vitest'
import type { Diagram } from '../api/types'
import { reg } from '../test/fixtures'
import { defaultEstimate, emptyInterview } from './interview'
import { buildMarkdown, fileStamp } from './exportDoc'

const diagram: Diagram = {
  nodes: [
    { id: 'users', kind: 'client', x: 0, y: 0, label: 'Users', params: { rps: 1200, readPct: 90 } },
    { id: 'app1', kind: 'appServer', x: 1, y: 1, label: 'App Server', params: { threads: 16 } },
    { id: 'db', kind: 'database', x: 2, y: 2, label: 'Database', params: {} },
  ],
  edges: [{ id: 'users>app1', from: 'users', to: 'app1', latencyMs: 40 }, { id: 'app1>db', from: 'app1', to: 'db', latencyMs: 5 }],
  notes: [],
}

const data = {
  ...emptyInterview(),
  functional: ['Upload a photo', 'View a feed'],
  nonFunctional: [{ label: 'Availability', target: 99.9, unit: '%' }, { label: 'Open question', target: null, unit: '' }],
  estimate: { ...defaultEstimate(), mau: 300e6, dauPct: 50, actionsPerDay: 2, perServerQps: 400, headroomPct: 30 },
  api: [{ method: 'POST', path: '/photos', request: '{file}', response: '201 {id}', notes: 'a | b' }],
  tables: [{ name: 'photos', fields: 'id, owner', key: 'id', indexes: 'owner', access: 'by owner' }],
  rubric: { scalability: 4, availability: 0, consistency: 3, cost: 2, communication: 5 },
  notes: 'Chose a cache.\nSecond line.',
}

describe('buildMarkdown', () => {
  it('assembles the whole write-up (golden)', () => {
    const md = buildMarkdown(data, diagram, reg, { title: 'Photo sharing', date: '2026-10-18', image: 'photo-sharing.png' })
    expect(md).toBe(`# Photo sharing

_System design practice, 2026-10-18_

## Requirements

**Functional**

- Upload a photo
- View a feed

**Non-functional**

- Availability: 99.9 %
- Open question: (no number yet)

## Estimates

- Daily users: 150 M
- Average load: 3,472 /s (reads 3,125 /s, writes 347 /s)
- Peak load: 10,417 /s
- Storage kept: 49.3 TB (32.8 B records)
- Peak bandwidth: 0.83 Gbit/s
- Servers: 38

## API

| Method | Path | Request | Response | Notes |
|---|---|---|---|---|
| POST | /photos | {file} | 201 {id} | a \\| b |

## Data model

| Table | Fields | Key | Indexes | Queries |
|---|---|---|---|---|
| photos | id, owner | id | owner | by owner |

## Design

![Design](photo-sharing.png)

Components:

- Users: rps 1200, reads 90
- App Server: t 16
- Database

Connections:

- Users → App Server
- App Server → Database

## Self-assessment

| Area | Rating |
|---|---|
| Scalability | 4 / 5 |
| Availability | not rated |
| Consistency | 3 / 5 |
| Cost | 2 / 5 |
| Communication | 5 / 5 |

## Notes

Chose a cache.
Second line.
`)
  })
  it('says so when sections are empty, and has no picture line without an image', () => {
    const md = buildMarkdown(emptyInterview(), { nodes: [], edges: [], notes: [] }, reg, { title: 'T', date: 'D' })
    expect(md).toContain('- (none written)')
    expect(md).toContain('(no endpoints written)')
    expect(md).toContain('(no tables written)')
    expect(md).toContain('(the canvas is empty)')
    expect(md).toContain('(none)')
    expect(md).not.toContain('![Design]')
    expect(md).toContain('| Scalability | not rated |')
  })
  it('keeps a table row on one line whatever was typed', () => {
    const md = buildMarkdown({ ...emptyInterview(), api: [{ method: 'GET', path: '/a\nb', request: 'x | y', response: '', notes: '' }] }, { nodes: [], edges: [], notes: [] }, reg, { title: 'T', date: 'D' })
    expect(md).toContain('| GET | /a b | x \\| y |  |  |')
  })
})

describe('buildMarkdown with a design name', () => {
  it('names the design under the title, and says nothing for an unnamed one', () => {
    const named = buildMarkdown(emptyInterview(), { name: 'Checkout | v2', nodes: [], edges: [], notes: [] }, reg, { title: 'T', date: 'D' })
    expect(named).toContain('\nDesign: **Checkout \\| v2**\n')
    const plain = buildMarkdown(emptyInterview(), { nodes: [], edges: [], notes: [] }, reg, { title: 'T', date: 'D' })
    expect(plain).not.toContain('Design: **')
  })
})

describe('fileStamp', () => {
  it('is year-month-day with leading zeros', () => {
    expect(fileStamp(new Date(2026, 0, 5))).toBe('2026-01-05')
  })
})
