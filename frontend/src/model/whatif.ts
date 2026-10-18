import type { ParamSpec } from '../api/types'

const MAX_POINTS = 20

/** A handful of values to try for a setting: its current value halved and doubled a few times, inside its range. */
export function suggestValues(spec: ParamSpec, current: number): number[] {
  const min = spec.min ?? 0
  const max = spec.max ?? Infinity
  const whole = (spec.step ?? 1) >= 1
  const out = new Set<number>()
  for (const f of [0.25, 0.5, 1, 2, 4]) {
    let v = current * f
    v = whole ? Math.round(v) : Math.round(v * 100) / 100
    out.add(Math.min(max, Math.max(min, v)))
  }
  return [...out].sort((a, b) => a - b)
}

export function parseValues(text: string): { values: number[]; error: string } {
  const parts = text.split(/[\s,]+/).filter(Boolean)
  if (parts.length === 0) return { values: [], error: 'Give at least one value.' }
  const nums = parts.map(Number)
  if (nums.some((n) => !Number.isFinite(n))) return { values: [], error: 'Every value must be a number.' }
  const values = [...new Set(nums)].sort((a, b) => a - b)
  if (values.length > MAX_POINTS) return { values: [], error: `At most ${MAX_POINTS} values.` }
  return { values, error: '' }
}
