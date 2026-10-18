import { describe, expect, it } from 'vitest'
import { humanCount, bandwidthGbps, dailyActiveUsers, humanBytes, humanRate, peakQps, qpsFromDaily, readWriteSplit, serversNeeded, storageBytes } from './estimate'

const near = (got: number, want: number, rel = 0.01) => expect(Math.abs(got - want) / want).toBeLessThanOrEqual(rel)

describe('estimation formulas (textbook numbers)', () => {
  it('turns monthly users into daily users', () => {
    expect(dailyActiveUsers(300e6, 50)).toBe(150e6)
    expect(dailyActiveUsers(10e6, 100)).toBe(10e6)
  })
  it('300M MAU, half active daily, 1 request each = 1,736 QPS on average, 2 requests each = 3,472', () => {
    near(qpsFromDaily(dailyActiveUsers(300e6, 50), 1), 1736.1)
    near(qpsFromDaily(dailyActiveUsers(300e6, 50), 2), 3472.2)
  })
  it('10M daily users x 50 actions = 5,787 QPS', () => {
    near(qpsFromDaily(10e6, 50), 5787)
  })
  it('peak is the average times a factor (3x by default)', () => {
    near(peakQps(3472.2), 10416.7)
    near(peakQps(1000, 2), 2000)
  })
  it('splits reads and writes from a read percentage', () => {
    const s = readWriteSplit(3472.2, 90)
    near(s.read, 3125, 0.001)
    near(s.write, 347.2, 0.001)
  })
  it('storage = records x bytes x years x replicas: 6 billion x 500 B = 3 TB, x3 replicas = 9 TB', () => {
    near(storageBytes({ records: 6e9, bytesEach: 500, replication: 1 }), 3e12)
    near(storageBytes({ records: 6e9, bytesEach: 500, replication: 3 }), 9e12)
  })
  it('bandwidth = QPS x payload: 5,787 QPS x 200 KB = 9.26 Gbit/s', () => {
    near(bandwidthGbps(5787, 200e3), 9.26)
  })
  it('servers = QPS / (per-server capacity x (1 - headroom)), rounded up', () => {
    expect(serversNeeded(10417, 400, 30)).toBe(38)
    expect(serversNeeded(1157, 400, 0)).toBe(3)
    expect(serversNeeded(1, 400, 30)).toBe(1)
  })
  it('refuses nonsense rather than answering with it', () => {
    expect(Number.isNaN(qpsFromDaily(-5, 10))).toBe(true)
    expect(Number.isNaN(serversNeeded(100, 0, 30))).toBe(true)
    expect(Number.isNaN(serversNeeded(100, 400, 100))).toBe(true)
    expect(Number.isNaN(readWriteSplit(100, 120).read)).toBe(true)
  })
})

describe('humanCount', () => {
  it('counts things, without a per-second unit', () => {
    expect(humanCount(150e6)).toBe('150 M')
    expect(humanCount(1.2e9)).toBe('1.2 B')
    expect(humanCount(48_000)).toBe('48,000')
    expect(humanCount(7.5)).toBe('7.5')
    expect(humanCount(NaN)).toBe('—')
  })
})

describe('human formatting', () => {
  it('rates', () => {
    expect(humanRate(3472.2)).toBe('3,472 /s')
    expect(humanRate(38.6)).toBe('38.6 /s')
    expect(humanRate(2.5e6)).toBe('2.5 M/s')
  })
  it('bytes in decimal units', () => {
    expect(humanBytes(3e12)).toBe('3 TB')
    expect(humanBytes(219e12)).toBe('219 TB')
    expect(humanBytes(1.5e9)).toBe('1.5 GB')
    expect(humanBytes(900)).toBe('900 B')
  })
})

import { calculate, estimateNotes } from './estimate'
import { defaultEstimate } from './interview'

describe('calculate', () => {
  const inputs = { ...defaultEstimate(), mau: 300e6, dauPct: 50, actionsPerDay: 2, peakFactor: 3, readPct: 90, bytesEach: 500, retentionYears: 5, replication: 3, payloadBytes: 200e3, perServerQps: 400, headroomPct: 30 }
  const r = calculate(inputs)
  it('chains the formulas on the textbook case', () => {
    near(r.dau, 150e6)
    near(r.avgQps, 3472.2)
    near(r.peakQps, 10416.7)
    near(r.readQps, 3125, 0.001)
    near(r.writeQps, 347.2, 0.001)
    near(r.peakReadQps, 9375, 0.001)
    expect(r.servers).toBe(38)
  })
  it('sizes storage from the writes kept for the retention period, times the replicas', () => {
    const records = 347.2222 * 86_400 * 365 * 5
    near(r.records, records, 0.001)
    near(r.storageBytes, records * 500 * 3, 0.001)
  })
  it('sizes bandwidth from the peak', () => {
    near(r.peakGbps, (10416.7 * 200e3 * 8) / 1e9, 0.001)
  })
  it('reports NaN for what it cannot compute instead of guessing', () => {
    const bad = calculate({ ...inputs, perServerQps: 0 })
    expect(Number.isNaN(bad.servers)).toBe(true)
    expect(bad.avgQps).toBeCloseTo(r.avgQps, 5)
  })
})

describe('estimateNotes', () => {
  it('writes the results as text a person could paste into notes', () => {
    const t = estimateNotes(calculate({ ...defaultEstimate(), mau: 300e6, dauPct: 50, actionsPerDay: 2, perServerQps: 400, headroomPct: 30 }))
    expect(t).toContain('Daily users: 150 M')
    expect(t).not.toContain('M/s')
    expect(t).toContain('Average load: 3,472 /s')
    expect(t).toContain('Peak load: 10,417 /s')
    expect(t).toMatch(/Servers: 38/)
    expect(t.split('\n').length).toBeGreaterThanOrEqual(6)
  })
})

import { payloadWarnings } from './estimate'

describe('payloadWarnings', () => {
  it('says nothing about small payloads', () => {
    expect(payloadWarnings(10e3)).toEqual([])
    expect(payloadWarnings(200e3)).toEqual([])
  })
  it('warns above 256 KB that most message queues will refuse it, and what to do instead', () => {
    const w = payloadWarnings(500e3)
    expect(w).toHaveLength(1)
    expect(w[0]).toMatch(/500 KB/)
    expect(w[0]).toMatch(/256 KB/)
    expect(w[0]).toMatch(/pointer|object store/i)
  })
  it('warns separately above 1 MB, where even Kafka-style logs refuse by default', () => {
    const w = payloadWarnings(5e6)
    expect(w).toHaveLength(2)
    expect(w[1]).toMatch(/1 MB/)
    expect(w[0]).toMatch(/5 MB/)
  })
  it('does not warn about nonsense', () => {
    expect(payloadWarnings(NaN)).toEqual([])
    expect(payloadWarnings(-1)).toEqual([])
  })
})
