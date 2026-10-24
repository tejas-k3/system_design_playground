import { Component, lazy, Suspense, useEffect, useRef, type ComponentType, type ReactNode } from 'react'
import { loadWithRetry, preloadSequentially } from './lazyLoad'
import { useStore, type State } from './state/store'

type Ui = State['ui']
type Loaded = { default: ComponentType }

interface Surface {
  label: string
  load: () => Promise<Loaded>
  /** What to set in the store if the chunk cannot be loaded, so the app is not left waiting on it. */
  abandon: Partial<Ui>
}

const SURFACES = {
  picker: { label: 'Challenges', load: () => import('./ui/ScenarioPicker').then((m) => ({ default: m.ScenarioPicker })), abandon: { picker: false } },
  skills: { label: 'Skill map', load: () => import('./ui/SkillMap').then((m) => ({ default: m.SkillMap })), abandon: { skills: false } },
  help: { label: 'Help', load: () => import('./ui/Help').then((m) => ({ default: m.Help })), abandon: { help: false } },
  templates: { label: 'Templates', load: () => import('./ui/TemplatesDialog').then((m) => ({ default: m.TemplatesDialog })), abandon: { templates: false } },
  capacity: { label: 'Capacity curve', load: () => import('./ui/CapacityDialog').then((m) => ({ default: m.CapacityDialog })), abandon: { capacity: false } },
  compare: { label: 'Compare', load: () => import('./ui/CompareDialog').then((m) => ({ default: m.CompareDialog })), abandon: { compare: false } },
  search: { label: 'Search', load: () => import('./ui/SearchDialog').then((m) => ({ default: m.SearchDialog })), abandon: { search: false } },
  question: { label: 'Question', load: () => import('./ui/QuestionDialog').then((m) => ({ default: m.QuestionDialog })), abandon: { question: null } },
  result: { label: 'Result', load: () => import('./ui/ResultModal').then((m) => ({ default: m.ResultModal })), abandon: { result: false } },
  present: { label: 'Present view', load: () => import('./ui/interview/PresentView').then((m) => ({ default: m.PresentView })), abandon: { present: false } },
} satisfies Record<string, Surface>

export type SurfaceName = keyof typeof SURFACES

const loaded: Partial<Record<SurfaceName, true>> = {}
const lazies: Partial<Record<SurfaceName, ReturnType<typeof lazy>>> = {}

function loadSurface(name: SurfaceName): Promise<Loaded> {
  return loadWithRetry(SURFACES[name].load).then((m) => {
    loaded[name] = true
    return m
  })
}

function lazyFor(name: SurfaceName) {
  // React.lazy remembers a rejection forever, so a failed one is dropped (see Boundary) and rebuilt on the next open.
  return (lazies[name] ??= lazy(() => loadSurface(name)))
}

export function preloadSurface(name: SurfaceName): void {
  if (!loaded[name]) loadSurface(name).catch(() => {})
}

type RIC = (cb: () => void, o?: { timeout: number }) => unknown

/** Preload every deferred surface in the background, one per idle slot. */
export function preloadAllWhenIdle(): void {
  const ric = (window as { requestIdleCallback?: RIC }).requestIdleCallback
  preloadSequentially(
    (Object.keys(SURFACES) as SurfaceName[]).map((n) => () => (loaded[n] ? Promise.resolve() : loadSurface(n))),
    { requestIdleCallback: ric?.bind(window), setTimeout: (cb, ms) => window.setTimeout(cb, ms) },
  )
}

/** Which surface a button opens, judged by its accessible name; used to start loading on hover or focus. */
const OPENERS: Array<[RegExp, SurfaceName]> = [
  [/^(challenges|start a challenge)$/i, 'picker'],
  [/^capacity curve$/i, 'capacity'],
  [/^compare designs$/i, 'compare'],
  [/^help$/i, 'help'],
  [/^(browse templates|starter templates…?)$/i, 'templates'],
  [/^show the result/i, 'result'],
  [/^present$/i, 'present'],
]

export function surfaceForButton(el: Element | null): SurfaceName | null {
  const b = el?.closest('button')
  if (!b) return null
  const name = (b.getAttribute('aria-label') ?? b.textContent ?? '').trim()
  for (const [re, s] of OPENERS) if (re.test(name)) return s
  return null
}

export function usePreloadSurfaces(): void {
  useEffect(() => {
    const hint = (e: Event) => {
      const s = surfaceForButton(e.target instanceof Element ? e.target : null)
      if (s) preloadSurface(s)
    }
    document.addEventListener('pointerover', hint)
    document.addEventListener('focusin', hint)
    preloadAllWhenIdle()
    return () => {
      document.removeEventListener('pointerover', hint)
      document.removeEventListener('focusin', hint)
    }
  }, [])
}

class Boundary extends Component<{ name: SurfaceName; children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  componentDidCatch() {
    const { name } = this.props
    delete lazies[name]
    const st = useStore.getState()
    st.toast(`${SURFACES[name].label} could not be loaded. Check your connection and try again.`, 'error')
    st.setUi(SURFACES[name].abandon)
  }
  render() { return this.state.failed ? null : this.props.children }
}

function Fallback({ label }: { label: string }) {
  return (
    <div role="status" aria-live="polite" style={{ position: 'fixed', left: '50%', bottom: 20, transform: 'translateX(-50%)', zIndex: 60, padding: '6px 14px', borderRadius: 6, background: 'var(--panel)', color: 'var(--ink)', border: '1px solid var(--line)', boxShadow: 'var(--shadow)', font: '14px var(--ui)' }}>
      Loading {label}…
    </div>
  )
}

/** Mounts a deferred surface only while `open`, shows a polite loading note meanwhile, and returns focus to the opener on close. */
export function Deferred({ name, open }: { name: SurfaceName; open: boolean }) {
  const was = useRef(false)
  const opener = useRef<Element | null>(null)
  if (open && !was.current) opener.current = document.activeElement // read before the dialog can take focus
  useEffect(() => {
    if (was.current && !open) {
      const el = opener.current
      if (el instanceof HTMLElement && el.isConnected && el !== document.body) el.focus()
      opener.current = null
    }
    was.current = open
  }, [open])
  if (!open) return null
  const Surface = lazyFor(name)
  return (
    <Boundary name={name}>
      <Suspense fallback={<Fallback label={SURFACES[name].label} />}>
        <Surface />
      </Suspense>
    </Boundary>
  )
}
