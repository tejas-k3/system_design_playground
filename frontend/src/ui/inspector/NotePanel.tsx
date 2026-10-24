import { useStore } from '../../state/store'

export function NotePanel({ id }: { id: string }) {
  const note = useStore((s) => s.diagram.notes.find((n) => n.id === id))!
  const updateNote = useStore((s) => s.updateNote)
  const del = useStore((s) => s.deleteIds)
  return (
    <div className="panel">
      <h2>Note</h2>
      <textarea className="note-input" value={note.text} onChange={(e) => updateNote(id, { text: e.target.value })} rows={5} aria-label="Note text" />
      <div className="field">
        <label htmlFor="fs">Text size</label>
        <input id="fs" type="range" min={12} max={40} value={note.fontSize ?? 18} onChange={(e) => updateNote(id, { fontSize: Number(e.target.value) })} />
      </div>
      <button className="danger-outline" onClick={() => del([id])}>Delete note</button>
    </div>
  )
}
