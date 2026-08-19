import type { Note } from '../../shared/types'

export function NoteItem({ note }: { note: Note }) {
  return (
    <article className="px-4 py-5">
      <p className="text-caption-medium text-ink-secondary">{note.createdAt}</p>
      <p className="mt-1.5 text-body text-balance text-ink">{note.text}</p>
    </article>
  )
}
