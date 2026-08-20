import type { Note } from '../../shared/types'

export function NoteItem({ note }: { note: Note }) {
  return (
    <article className="px-3.5 py-3">
      <p className="flex items-center gap-1.5 text-caption tabular-nums text-ink-tertiary">
        <span className="h-1.5 w-1.5 rounded-full bg-accent" aria-hidden />
        {note.createdAt}
      </p>
      <p className="mt-1.5 font-serif text-body-serif text-balance text-ink">{note.text}</p>
    </article>
  )
}
