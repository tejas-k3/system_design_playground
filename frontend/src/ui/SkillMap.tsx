import { useRef } from 'react'
import { downloadText } from './download'
import { exportProgress, mergeProgress, parseImport, MAX_FILE_BYTES } from '../model/progressIO'
import { REVIEW_EXPLAIN, reviewReason } from '../model/record'
import { levelSummary, skillMap } from '../model/skills'
import { loadScenario } from '../sim/loadScenario'
import { useStore } from '../state/store'
import { LevelSwitch } from './LevelSwitch'
import { useEscapeKey } from './useEscapeKey'
import { useScenarioList } from './useScenarioList'

/** One row for each idea the challenges practise: how many stars you have there and what to try next. */
export function SkillMap() {
  const open = useStore((s) => s.ui.skills)
  const setUi = useStore((s) => s.setUi)
  const progress = useStore((s) => s.progress)
  const records = useStore((s) => s.records)
  const level = useStore((s) => s.level)
  const fileRef = useRef<HTMLInputElement>(null)
  const { list, fromCache, pending, error } = useScenarioList(open)
  const err = list ? '' : error
  const close = () => setUi({ skills: false })
  useEscapeKey(open, close)
  if (!open) return null
  const rows = list ? skillMap(list, progress, level, records) : []
  const levels = list ? levelSummary(list, progress) : []
  const title = (id: string) => list?.find((s) => s.id === id)?.title ?? id
  return (
    <div className="modal-back" role="dialog" aria-modal aria-label="Skill map" onClick={close}>
      <div className="modal wide" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head"><h2>Skill map</h2><button className="x" autoFocus onClick={close} aria-label="Close">×</button></header>
        <p className="muted small">Each row is an idea the challenges practise. The bar shows stars earned out of the stars available there; the button starts the one to try next (fewest stars first, easiest first).</p>
        {fromCache && !pending && <p className="muted small" role="status">Showing the last known list. Challenges still open through the server.</p>}
        {levels.length > 0 && (
          <section aria-label="Stars by level">
            <h3>Stars by level</h3>
            <ul className="skills levels">
              {levels.map((l) => (
                <li key={l.level}>
                  <b>{l.label}</b>
                  <span className="muted small">{l.text}</span>
                  <progress max={l.maxStars || 1} value={l.stars} aria-label={`${l.label}: ${l.stars} of ${l.maxStars} stars`} />
                </li>
              ))}
            </ul>
          </section>
        )}
        <LevelSwitch />
        <div className="row-btns">
          <button onClick={() => { downloadText('sdp-progress.json', exportProgress({ progress, records })); useStore.getState().toast('Progress exported', 'info') }}>Export progress</button>
          <button onClick={() => fileRef.current?.click()}>Import progress</button>
          <input ref={fileRef} type="file" accept="application/json,.json" hidden aria-label="Import progress file" onChange={async (e) => {
            const f = e.target.files?.[0]
            e.target.value = ''
            const st = useStore.getState()
            if (!f) return
            if (f.size > MAX_FILE_BYTES) return st.toast('That file is too large to be a progress export.', 'error')
            let text = ''
            try {
              text = await f.text()
            } catch {
              return st.toast('The file could not be read.', 'error')
            }
            const r = parseImport(text)
            if (!r.ok) return st.toast(r.error, 'error')
            const m = mergeProgress({ progress: st.progress, records: st.records }, r)
            st.importProgress(m.progress, m.records)
            st.toast(`Progress imported${r.skipped ? ` (${r.skipped} unusable ${r.skipped === 1 ? 'entry' : 'entries'} skipped)` : ''}`, 'info')
          }} />
        </div>
        {err && <p className="bad">{err}</p>}
        {!list && !err && <p className="muted">Loading…</p>}
        <ul className="skills">
          {rows.map((r) => (
            <li key={r.concept}>
              <b>{r.label}{r.review.length > 0 && <>{' '}<span className="review" tabIndex={0} role="note" title={REVIEW_EXPLAIN} aria-label={`Needs review. ${REVIEW_EXPLAIN} ${r.review.map((id) => `${title(id)}: ${reviewReason(records[id])}.`).join(' ')}`}>needs review</span></>}</b>
              <span className="muted small">{r.solved} of {r.total} challenges solved · {r.stars} of {r.maxStars} stars</span>
              <progress max={r.maxStars} value={r.stars} aria-label={`${r.label}: ${r.stars} of ${r.maxStars} stars`} />
              {r.next ? <button onClick={() => { setUi({ skills: false, picker: false }); loadScenario(r.next!) }}>Next: {title(r.next)}</button> : <span className="muted small">All at three stars</span>}
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
