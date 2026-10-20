import { describe, expect, it } from 'vitest'
import { GLOSSARY, plain, splitMarkup, termById } from './glossary'

const REQUIRED = ['p50', 'p95', 'p99', 'throughput', 'utilisation', 'littles-law', 'queueing-knee', 'thread-pool', 'connection-pool', 'circuit-breaker', 'retry-storm', 'metastable-failure', 'idempotency', 'cache-hit-rate', 'ttl', 'stampede', 'eviction', 'replication-lag', 'failover', 'split-brain', 'fencing', 'quorum', 'consistency-levels', 'sharding', 'hot-key', 'backpressure', 'load-shedding', 'autoscaling', 'slo']

describe('glossary', () => {
  it('covers every term the ticket lists, once each', () => {
    const ids = GLOSSARY.map((t) => t.id)
    for (const id of REQUIRED) expect(ids, id).toContain(id)
    expect(new Set(ids).size).toBe(ids.length)
  })
  it('gives each term a one-sentence definition and a longer explanation', () => {
    for (const t of GLOSSARY) {
      expect(t.short.length, t.id).toBeGreaterThan(10)
      expect(t.short.split('. ').length, `${t.id} short is one sentence`).toBe(1)
      expect(t.long.length, t.id).toBeGreaterThan(t.short.length)
    }
  })
  it('looks a term up by id', () => expect(termById('p95')?.name).toBe('p95'))
})

describe('splitMarkup', () => {
  it('leaves plain text alone', () => expect(splitMarkup('nothing here')).toEqual(['nothing here']))
  it('turns {{id}} into the term, keeping the text around it', () => {
    const p = splitMarkup('The {{p95}} is high.')
    expect(p).toHaveLength(3)
    expect(p[0]).toBe('The ')
    expect(p[1]).toMatchObject({ term: { id: 'p95' }, text: 'p95' })
    expect(p[2]).toBe(' is high.')
  })
  it('shows the author words after a bar', () => {
    expect(splitMarkup('{{ttl|expiry time}}')[0]).toMatchObject({ term: { id: 'ttl' }, text: 'expiry time' })
  })
  it('shows an unknown id as plain words rather than braces', () => expect(splitMarkup('a {{nope}} b')).toEqual(['a ', 'nope', ' b']))
})

describe('plain', () => {
  it('drops the markup, keeping the words a reader sees', () => {
    expect(plain('Short {{ttl|expiry}} and a {{p95}}.')).toBe('Short expiry and a p95.')
    expect(plain('no markup')).toBe('no markup')
    expect(plain('a {{nope}} b')).toBe('a nope b')
  })
})
