import type { Diagram } from '../api/types'
import { Reg, sanitizeDiagram, MAX_ZOOM, MIN_ZOOM } from '../model/diagram'
import { sanitizeInterview, type InterviewData } from '../model/interview'
import { useStore, type Viewport } from './store'

// The key keeps its v1 name so existing saves are found; the blob inside says which version wrote it. Version 2 adds
// the optional interview session. A version 1 blob has no such field and loads unchanged.
export const STORAGE_KEY = 'sdp:v1'
export const SAVED_VERSION = 2

interface Saved {
  version: typeof SAVED_VERSION
  interview: InterviewData | null
  sandbox: Diagram
  theme: 'light' | 'dark'
  progress: Record<string, number>
  viewport: Viewport
}

function storage(): Storage | null {
  try {
    if (typeof window === 'undefined') return null
    window.localStorage.getItem(STORAGE_KEY)
    return window.localStorage
  } catch {
    return null
  }
}

export function parseSaved(raw: string | null, reg: Reg) {
  if (!raw) return {}
  let obj: unknown
  try {
    obj = JSON.parse(raw)
  } catch {
    return {}
  }
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return {}
  const o = obj as Record<string, unknown>
  const out: { sandbox?: Diagram; theme?: 'light' | 'dark'; progress?: Record<string, number>; viewport?: Viewport; interview?: InterviewData } = {}
  const sd = sanitizeDiagram(reg, o.sandbox)
  if (sd) out.sandbox = sd.diagram
  if (o.theme === 'light' || o.theme === 'dark') out.theme = o.theme
  if (o.progress && typeof o.progress === 'object' && !Array.isArray(o.progress)) {
    const p: Record<string, number> = {}
    for (const [k, v] of Object.entries(o.progress as Record<string, unknown>)) if (typeof v === 'number' && v >= 0 && v <= 3) p[k] = v
    out.progress = p
  }
  const iv = sanitizeInterview(o.interview)
  if (iv) out.interview = iv
  const v = o.viewport as Record<string, unknown> | undefined
  if (v && [v.scrollX, v.scrollY, v.zoom].every((n) => typeof n === 'number' && Number.isFinite(n))) {
    out.viewport = { scrollX: v.scrollX as number, scrollY: v.scrollY as number, zoom: Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, v.zoom as number)) }
  }
  return out
}

/** Load saved state into the store and start saving changes (debounced, flushed on hide). */
export function startPersistence(reg: Reg): void {
  const ls = storage()
  if (!ls) return
  useStore.getState().hydrate(parseSaved(ls.getItem(STORAGE_KEY), reg))
  const theme = useStore.getState().theme
  document.documentElement.setAttribute('data-theme', theme)

  let timer: ReturnType<typeof setTimeout> | undefined
  const flush = () => {
    if (timer) clearTimeout(timer)
    timer = undefined
    const s = useStore.getState()
    const saved: Saved = {
      version: SAVED_VERSION,
      interview: s.interview,
      sandbox: s.scenario ? (s.sandbox ?? { nodes: [], edges: [], notes: [] }) : s.diagram,
      theme: s.theme,
      progress: s.progress,
      viewport: s.viewport,
    }
    try {
      ls.setItem(STORAGE_KEY, JSON.stringify(saved))
    } catch {
      /* quota or blocked: ignore */
    }
  }
  useStore.subscribe((s, p) => {
    if (s.diagram !== p.diagram || s.theme !== p.theme || s.progress !== p.progress || s.viewport !== p.viewport || s.sandbox !== p.sandbox || s.interview !== p.interview) {
      if (timer) clearTimeout(timer)
      timer = setTimeout(flush, 300)
    }
  })
  window.addEventListener('pagehide', flush)
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'hidden') flush()
  })
}
