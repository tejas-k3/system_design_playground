import { describe, expect, it } from 'vitest'
import { bandwidthGbps, dailyActiveUsers, humanBytes, humanRate, peakQps, qpsFromDaily, readWriteSplit, serversNeeded, storageBytes } from './estimate'

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
