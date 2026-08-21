import { useState } from 'react'
import { Pencil, Trash2 } from 'lucide-react'
import type { Note } from '../../shared/api/client'

export function NoteItem({ note, onSave, onDelete, busy }: { note: Note; onSave: (content: string) => Promise<unknown>; onDelete: () => void; busy: boolean }) {
  const [editing, setEditing] = useState(false)
  const [content, setContent] = useState(note.content)
  const [error, setError] = useState('')

  const save = async () => {
    const value = content.trim()
    if (!value || value === note.content) { setEditing(false); setContent(note.content); return }
    setError('')
    try {
      await onSave(value)
      setEditing(false)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败。')
    }
  }

  return (
    <article className="px-3.5 py-3">
      <p className="flex items-center gap-1.5 text-caption tabular-nums text-ink-tertiary"><span className="h-1.5 w-1.5 rounded-full bg-accent" aria-hidden />{new Date(note.created_at).toLocaleString()}</p>
      {editing ? (
        <div className="mt-2"><textarea value={content} onChange={(event) => setContent(event.target.value)} aria-label="编辑笔记" className="min-h-24 w-full resize-y rounded-md border border-hairline bg-canvas p-2 font-serif text-body-serif text-ink" />{error ? <p role="alert" className="mt-2 text-caption text-danger">{error}</p> : null}<div className="mt-2 flex gap-4"><button type="button" disabled={busy} onClick={() => void save()} className="min-h-11 text-callout text-accent">保存</button><button type="button" onClick={() => { setEditing(false); setContent(note.content); setError('') }} className="min-h-11 text-callout text-ink-secondary">取消</button></div></div>
      ) : (
        <><p className="mt-1.5 font-serif text-body-serif text-balance text-ink">{note.content}</p><div className="mt-2 flex gap-4"><button type="button" aria-label="编辑笔记" onClick={() => { setError(''); setEditing(true) }} className="flex min-h-11 items-center gap-1 text-caption text-ink-secondary"><Pencil size={15} aria-hidden />编辑</button><button type="button" aria-label="删除笔记" disabled={busy} onClick={() => { if (window.confirm('删除这条笔记？')) onDelete() }} className="flex min-h-11 items-center gap-1 text-caption text-danger"><Trash2 size={15} aria-hidden />删除</button></div></>
      )}
    </article>
  )
}
