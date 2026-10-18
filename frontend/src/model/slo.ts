import type { SeriesPoint } from '../api/types'

export interface Budget {
  allowed: number // failures the SLO permits so far
  used: number
  remaining: number // may be negative
  remainingFrac: number // 0..1
  burnFast: number // failure rate / allowed rate over the last 10 s (1 = exactly on pace)
  burnSlow: number // the same over the last 60 s
  status: 'ok' | 'burning' | 'exhausted'
}

const FAST_SEC = 10
const SLOW_SEC = 60
export const BURN_ALERT = 14 // the usual page-someone threshold for a fast window

/** Failure rate over the last `sec` seconds of the series, divided by the allowed failure rate. */
function burn(series: SeriesPoint[], sec: number, allowedRate: number): number {
  if (series.length === 0 || allowedRate <= 0) return 0
  const from = series[series.length - 1].t - sec
  let ok = 0
  let err = 0
  for (const p of series) {
    if (p.t > from) {
      ok += p.okRps
      err += p.errRps
    }
  }
  return ok + err > 0 ? err / (ok + err) / allowedRate : 0
}

export function errorBudget(run: { resolved: number; failed: number }, series: SeriesPoint[], sloPct: number): Budget {
  const allowedRate = Math.max(0, 1 - sloPct / 100)
  const allowed = run.resolved * allowedRate
  const remaining = allowed - run.failed
  const remainingFrac = run.resolved === 0 ? 1 : allowed > 0 ? Math.min(1, Math.max(0, remaining / allowed)) : run.failed > 0 ? 0 : 1
  const burnFast = burn(series, FAST_SEC, allowedRate)
  const burnSlow = burn(series, SLOW_SEC, allowedRate)
  const status = run.failed > allowed ? 'exhausted' : burnFast >= BURN_ALERT ? 'burning' : 'ok'
  return { allowed, used: run.failed, remaining, remainingFrac, burnFast, burnSlow, status }
}
