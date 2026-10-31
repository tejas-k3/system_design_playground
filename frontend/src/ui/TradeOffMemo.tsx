import { useState } from 'react'
import type { Finding, Score } from '../api/types'
import { type Memo, loadMemo, memoText, onCallView, runEvidence, saveMemo } from '../model/memo'
import { copyText } from '../sim/designs'

const FIELDS: { key: keyof Memo; label: string; hint: string }[] = [
  { key: 'design', label: 'The design I chose', hint: 'One line: what it is and why' },
  { key: 'cost', label: 'What it costs', hint: 'Money, complexity, what it gives up' },
  { key: 'watch', label: 'What I would monitor', hint: 'The number that tells you it is going wrong' },
]

/** A short memo after a pass, kept in this browser, next to what the run measured and what the design would put on call. */
export function TradeOffMemo({ id, title, score, findings, seconds, minSuccessRate }: { id: string; title: string; score: Score; findings: Finding[]; seconds: number; minSuccessRate?: number }) {
  const [memo, setMemo] = useState<Memo>(() => loadMemo(id))
  const [copied, setCopied] = useState<'yes' | 'no' | null>(null)
  const [saved, setSaved] = useState(true)
  const evidence = runEvidence(score, seconds)
  const view = onCallView(score, findings, minSuccessRate)
  const onCall = [
    ...(view.alerts.length === 0 ? ['No design warnings would page anyone.'] : [`${view.alerts.length} design ${view.alerts.length === 1 ? 'warning' : 'warnings'} would raise an alert:`, ...view.alerts.map((a) => `  ${a}`)]),
    view.budgetUsedPct === null ? 'The goal states no success target, so there is no error budget to spend.' : `The run used ${view.budgetUsedPct}% of the error budget the success goal allows.`,
  ]
  const change = (key: keyof Memo, value: string) => {
    const next = { ...memo, [key]: value }
    setMemo(next)
    setSaved(saveMemo(id, next))
    setCopied(null)
  }
  const copy = async () => {
    setCopied((await copyText(memoText(title, memo, evidence, onCall))) ? 'yes' : 'no')
  }
  return (
    <section className="memo" aria-label="Trade-off memo">
      <h3>Trade-off memo</h3>
      <p className="muted small">Three lines, as you would hand them to a reviewer. They stay in this browser.{!saved && ' This browser would not keep them, so they are lost when the page closes.'}</p>
      {FIELDS.map((f) => (
        <label key={f.key} className="memo-field">
          <span>{f.label}</span>
          <input type="text" value={memo[f.key]} maxLength={240} placeholder={f.hint} onChange={(e) => change(f.key, e.target.value)} />
        </label>
      ))}
      <div className="memo-evidence">
        <b>What the run showed</b>
        <ul>{evidence.map((e) => <li key={e}>{e}</li>)}</ul>
        <b>On call</b>
        <ul>{onCall.map((e) => <li key={e} className={e.startsWith('  ') ? 'memo-sub' : undefined}>{e.trim()}</li>)}</ul>
      </div>
      <div className="row-btns">
        <button onClick={() => void copy()}>Copy as text</button>
        <span role="status" className="muted small">{copied === 'yes' ? 'Copied.' : copied === 'no' ? 'The browser would not copy. Select and copy the lines by hand.' : ''}</span>
      </div>
    </section>
  )
}
