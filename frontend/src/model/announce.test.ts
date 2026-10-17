import { describe, expect, it } from 'vitest'
import { emptyStats } from '../state/store'
import { describeChart, statusSummary } from './announce'

const run = (r: Partial<ReturnType<typeof emptyStats>['global']['run']>) => ({ ...emptyStats().global.run, ...r })
const label = (id: string) => (id === 'queueFull' ? 'Queue full (503)' : id)

describe('statusSummary', () => {
  it('says so when nothing has finished yet', () => {
    expect(statusSummary(run({}), label)).toBe('No requests have finished yet.')
  })
  it('reads out success rate, latency and volume for a healthy run', () => {
    expect(statusSummary(run({ resolved: 1298, ok: 1298, p50: 127.2, p95: 161.4 }), label)).toBe('1,298 requests, 100.0% succeeded, median 127 ms, 95th percentile 161 ms, no failures.')
  })
  it('names the main reason when requests fail', () => {
    const s = statusSummary(run({ resolved: 1000, ok: 600, failed: 400, p50: 90, p95: 300, errors: { queueFull: 350, timeout: 50 } }), label)
    expect(s).toContain('60.0% succeeded')
    expect(s).toContain('400 failed, mostly Queue full (503)')
  })
})

describe('describeChart', () => {
  it('lists the latest value of each line', () => {
    expect(describeChart('Throughput', 'req/s', [{ label: 'ok', value: 148 }, { label: 'errors', value: 0 }])).toBe('Throughput chart, req/s. Latest: ok 148, errors 0.')
  })
  it('is honest when there is no data yet', () => {
    expect(describeChart('Latency', 'ms', [])).toBe('Latency chart, ms. No data yet.')
  })
})
