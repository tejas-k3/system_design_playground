import { describe, expect, it } from 'vitest'
import { GLOSSARY, plain, splitMarkup, termById } from './glossary'

const REQUIRED = ['p50', 'p95', 'p99', 'throughput', 'utilisation', 'littles-law', 'queueing-knee', 'thread-pool', 'connection-pool', 'circuit-breaker', 'retry-storm', 'metastable-failure', 'idempotency', 'cache-hit-rate', 'ttl', 'stampede', 'eviction', 'replication-lag', 'failover', 'split-brain', 'fencing', 'quorum', 'consistency-levels', 'sharding', 'hot-key', 'backpressure', 'load-shedding', 'autoscaling', 'slo', 'consistent-hashing', 'tail-latency-amplification', 'bulkhead', 'dead-letter-queue', 'cap-trade-off', 'write-ahead-log', 'leader-election', 'read-your-writes', 'exactly-once-delivery', 'thundering-herd']

// Keep this list in sync with IDs registered in backend/internal/scenarios/defs.go.
const CHALLENGE_IDS = ['hello-world', 'opening-night', 'slow-reads', 'connection-crunch', 'retry-pileup', 'stale-prices', 'double-booked', 'charged-twice', 'disappearing-money', 'vendor-wait', 'write-flood', 'server-down', 'live-scoreboard', 'nearby-drivers', 'scoreboard-survives', 'primary-dies', 'morning-rush', 'cut-off', 'flaky-neighbour', 'cache-stampede', 'sync-or-async', 'fence-the-old-primary', 'wave-rider', 'passed-the-load-test', 'flash-crowd', 'hot-shard', 'scan-pollution', 'search-tail', 'celebrity-post', 'image-egress', 'video-upload', 'gc-storm', 'disk-writes', 'one-heavy-query', 'nobody-on-call', 'slow-query', 'pick-the-store', 'poison-pill', 'consumer-lag', 'hot-cell', 'restore-drill', 'lost-cart', 'quorum-overlap', 'lease-expired', 'who-is-the-leader', 'launch-day', 'count-the-clicks', 'unique-visitors', 'top-ten', 'polite-crawler', 'slow-first-token', 'rag-under-load', 'the-gpu-budget', 'shed-the-cheap-traffic', 'url-shortener', 'rate-limiter-showdown', 'multi-region-reads', 'add-a-shard', 'heavy-tail', 'noisy-tenant', 'notification-fan-out', 'go-global', 'payment-saga', 'frozen-checkout', 'reconnect-storm', 'bad-deploy', 'zone-outage', 'retry-storm', 'bot-flood', 'upload-500kb', 'edge-cache', 'one-slow-server', 'split-the-database', 'launch-day-capstone']

describe('glossary', () => {
  it('covers every term the ticket lists, once each', () => {
    const ids = GLOSSARY.map((t) => t.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const id of REQUIRED) expect(ids, id).toContain(id)
    for (const term of GLOSSARY) {
      if (term.challenge) expect(CHALLENGE_IDS, `${term.id} challenge`).toContain(term.challenge)
      if (REQUIRED.slice(-10).includes(term.id)) {
        expect(term.short, `${term.id} short`).toMatch(/\.$/)
        expect(term.short, `${term.id} short`).not.toMatch(/[\r\n]/)
        expect(term.short.length, `${term.id} short length`).toBeLessThanOrEqual(140)
        expect(term.long.length, `${term.id} long length`).toBeGreaterThanOrEqual(150)
        expect(term.long.length, `${term.id} long length`).toBeLessThanOrEqual(600)
      }
    }
  })
  it('gives each term a one-sentence definition and a longer explanation', () => {
    for (const t of GLOSSARY) {
      expect(t.short.length, t.id).toBeGreaterThan(10)
      expect(t.short.split('. ').length, `${t.id} short is one sentence`).toBe(1)
      expect(t.long.length, t.id).toBeGreaterThan(t.short.length)
    }
  })
  it('explains both read and write quorum overlap rules', () => {
    const quorum = termById('quorum')!
    expect(quorum.short).toContain('copies that must answer')
    expect(quorum.long).toContain('R + W > N')
    expect(quorum.long).toContain('latest completed write')
    expect(quorum.long).toContain('W > N/2')
    expect(quorum.long).toContain('majority quorum')
    expect(quorum.long.trim().split(/\s+/).length).toBeLessThanOrEqual(70)
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
