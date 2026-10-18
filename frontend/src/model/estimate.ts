// Back-of-envelope formulas for the interview calculator. Plain arithmetic, decimal units (1 TB = 1e12 bytes),
// 86,400 seconds a day. Nonsense input gives NaN rather than a confident wrong answer.

const SECONDS_PER_DAY = 86_400
const bad = (...xs: number[]) => xs.some((x) => !Number.isFinite(x) || x < 0)

export function dailyActiveUsers(monthlyActive: number, dailyPct: number): number {
  return bad(monthlyActive, dailyPct) || dailyPct > 100 ? NaN : (monthlyActive * dailyPct) / 100
}

/** Average requests a second from users a day and actions per user per day. */
export function qpsFromDaily(dailyUsers: number, actionsPerUser: number): number {
  return bad(dailyUsers, actionsPerUser) ? NaN : (dailyUsers * actionsPerUser) / SECONDS_PER_DAY
}

export function peakQps(average: number, factor = 3): number {
  return bad(average, factor) ? NaN : average * factor
}

export function readWriteSplit(qps: number, readPct: number): { read: number; write: number } {
  if (bad(qps, readPct) || readPct > 100) return { read: NaN, write: NaN }
  return { read: (qps * readPct) / 100, write: (qps * (100 - readPct)) / 100 }
}

export function storageBytes(x: { records: number; bytesEach: number; replication: number }): number {
  return bad(x.records, x.bytesEach, x.replication) ? NaN : x.records * x.bytesEach * x.replication
}

export function bandwidthGbps(qps: number, payloadBytes: number): number {
  return bad(qps, payloadBytes) ? NaN : (qps * payloadBytes * 8) / 1e9
}

/** App servers needed: load / (capacity of one, keeping `headroomPct` of it spare), rounded up, at least one. */
export function serversNeeded(qps: number, perServerQps: number, headroomPct: number): number {
  if (bad(qps, perServerQps, headroomPct) || perServerQps === 0 || headroomPct >= 100) return NaN
  return Math.max(1, Math.ceil(qps / (perServerQps * (1 - headroomPct / 100))))
}

export function humanRate(perSecond: number): string {
  if (!Number.isFinite(perSecond)) return '—'
  if (perSecond >= 1e6) return `${trim(perSecond / 1e6)} M/s`
  if (perSecond >= 100) return `${Math.round(perSecond).toLocaleString('en-US')} /s`
  return `${trim(perSecond)} /s`
}

export function humanCount(n: number): string {
  if (!Number.isFinite(n)) return '—'
  if (n >= 1e9) return `${trim(n / 1e9)} B`
  if (n >= 1e6) return `${trim(n / 1e6)} M`
  return n >= 100 ? Math.round(n).toLocaleString('en-US') : trim(n)
}

export function humanBytes(bytes: number): string {
  if (!Number.isFinite(bytes)) return '—'
  const units: [number, string][] = [[1e15, 'PB'], [1e12, 'TB'], [1e9, 'GB'], [1e6, 'MB'], [1e3, 'KB']]
  for (const [size, name] of units) if (bytes >= size) return `${trim(bytes / size)} ${name}`
  return `${Math.round(bytes)} B`
}

function trim(n: number): string {
  const r = n >= 100 ? Math.round(n) : Math.round(n * 10) / 10
  return String(r)
}

import type { EstimateInputs } from './interview'

export interface Estimate {
  dau: number
  avgQps: number
  peakQps: number
  readQps: number
  writeQps: number
  peakReadQps: number
  records: number // writes kept for the retention period
  storageBytes: number
  peakGbps: number
  servers: number
}

/** Every number the calculator shows, from the inputs on the pad. NaN where an input makes a number meaningless. */
export function calculate(i: EstimateInputs): Estimate {
  const dau = dailyActiveUsers(i.mau, i.dauPct)
  const avgQps = qpsFromDaily(dau, i.actionsPerDay)
  const peak = peakQps(avgQps, i.peakFactor)
  const avg = readWriteSplit(avgQps, i.readPct)
  const pk = readWriteSplit(peak, i.readPct)
  const records = avg.write * SECONDS_PER_DAY * 365 * i.retentionYears
  return {
    dau,
    avgQps,
    peakQps: peak,
    readQps: avg.read,
    writeQps: avg.write,
    peakReadQps: pk.read,
    records,
    storageBytes: storageBytes({ records, bytesEach: i.bytesEach, replication: i.replication }),
    peakGbps: bandwidthGbps(peak, i.payloadBytes),
    servers: serversNeeded(peak, i.perServerQps, i.headroomPct),
  }
}

/** The results as plain lines for the notes. */
export function estimateNotes(e: Estimate): string {
  return [
    'Estimate',
    `Daily users: ${humanCount(e.dau)}`,
    `Average load: ${humanRate(e.avgQps)} (reads ${humanRate(e.readQps)}, writes ${humanRate(e.writeQps)})`,
    `Peak load: ${humanRate(e.peakQps)}`,
    `Storage kept: ${humanBytes(e.storageBytes)} (${humanCount(e.records)} records)`,
    `Peak bandwidth: ${Number.isFinite(e.peakGbps) ? e.peakGbps.toFixed(2) : '—'} Gbit/s`,
    `Servers: ${Number.isFinite(e.servers) ? e.servers : '—'}`,
  ].join('\n')
}
