/** Where and how large to draw the in-place note editor, in screen pixels. */
export interface NoteEdit {
  id: string
  left: number
  top: number
  w: number
  size: number
}

/** A textarea laid over a note on the canvas. Enter or blur commits, Escape cancels into a commit too. */
export function NoteEditor({ edit, initialText, onCommit }: { edit: NoteEdit; initialText: string; onCommit: (text: string) => void }) {
  return (
    <textarea
      autoFocus
      className="note-edit"
      defaultValue={initialText}
      style={{ left: edit.left, top: edit.top, width: edit.w, fontSize: edit.size }}
      onBlur={(e) => onCommit(e.currentTarget.value)}
      onKeyDown={(e) => {
        if (e.key === 'Escape') e.currentTarget.blur()
        if (e.key === 'Enter' && !e.shiftKey) {
          e.preventDefault()
          e.currentTarget.blur()
        }
      }}
    />
  )
}
