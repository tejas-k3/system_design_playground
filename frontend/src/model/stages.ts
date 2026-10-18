import type { Mark, StageId } from './timer'
import { STAGES } from './timer'

export interface StageInfo {
  prompt: string
  tool: string // what the learner uses in this stage
}

const INFO: Record<StageId, StageInfo> = {
  requirements: { prompt: 'Clarify requirements. Write three functional requirements (what the system does) and three non-functional ones with numbers (availability, p95 latency, requests a second, data size).', tool: 'Requirements pad' },
  estimation: { prompt: 'Estimate the scale. From daily users and what each does, work out requests a second (average and peak), the read/write split, storage and bandwidth, and how many servers that means.', tool: 'Estimation calculator' },
  api: { prompt: 'Sketch the API. List the endpoints the clients need: method, path, what goes in, what comes back, and anything that needs an idempotency key.', tool: 'API pad' },
  data: { prompt: 'Model the data. For each table or collection: its fields, its key, its indexes, and the queries it must answer quickly.', tool: 'Data-model pad' },
  design: { prompt: 'Draw the high-level design. Put the components on the canvas, connect them, and run it against your own numbers.', tool: 'Canvas and simulator' },
  deepdive: { prompt: 'Deep dive. Pick the riskiest part (the database, the cache, the queue) and show what happens when it is slow or fails: crash it, watch the charts, fix it.', tool: 'Canvas, charts and traces' },
  tradeoffs: { prompt: 'Trade-offs and wrap-up. Say what you chose and what you gave up, what you would do with more time, and rate your own design.', tool: 'Rubric' },
}

export const stageInfo = (id: StageId): StageInfo => INFO[id]

/** Minutes and seconds left (hours when long), to the nearest second but never 0:00 while time remains. */
export function countdown(ms: number): string {
  const total = ms <= 0 ? 0 : Math.max(1, Math.round(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const ss = String(s).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

/** Where a key moves the stage tabs, or null for keys they do not handle. */
export function stageAfterKey(current: StageId, key: string): StageId | null {
  const i = STAGES.findIndex((s) => s.id === current)
  switch (key) {
    case 'ArrowRight':
    case 'ArrowDown':
      return STAGES[Math.min(STAGES.length - 1, i + 1)].id
    case 'ArrowLeft':
    case 'ArrowUp':
      return STAGES[Math.max(0, i - 1)].id
    case 'Home':
      return STAGES[0].id
    case 'End':
      return STAGES[STAGES.length - 1].id
    default:
      return null
  }
}

export function markText(m: Mark): string {
  return { half: 'Half the time has passed.', tenLeft: 'Ten minutes left.', fiveLeft: 'Five minutes left.', oneLeft: 'One minute left.', over: 'Time is up.' }[m]
}
