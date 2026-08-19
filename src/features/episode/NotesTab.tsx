import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import type { Episode } from '../../shared/types'
import { useCaptureStore } from '../../shared/store/capture'
import { NoteItem } from './NoteItem'
import { EmptyState } from '../../shared/components/EmptyState'
import { InsetGroup } from '../../shared/components/InsetGroup'

export function NotesTab({ episode }: { episode: Episode }) {
  const navigate = useNavigate()
  const setEpisodeId = useCaptureStore((state) => state.setEpisodeId)

  const startCapture = () => {
    setEpisodeId(episode.id)
    navigate('/capture')
  }

  return (
    <div className="pb-4">
      {episode.notes.length === 0 ? (
        <EmptyState
          title="还没有笔记"
          detail="听节目时冒出的想法，随时用底部「记录」记下来。它们会按时间排在这里，并被 AI 整理进这期节目。"
        />
      ) : (
        <div className="px-4 pt-4">
          <InsetGroup>
            {episode.notes.map((note) => (
              <NoteItem key={note.id} note={note} />
            ))}
          </InsetGroup>
        </div>
      )}

      <div className="mt-2 px-4">
        <button
          type="button"
          onClick={startCapture}
          className="flex min-h-11 w-full items-center gap-2 rounded-md text-accent transition-colors duration-fast ease-ios active:bg-subtle"
        >
          <Plus size={19} strokeWidth={2} aria-hidden />
          <span className="text-callout">记录想法</span>
        </button>
      </div>
    </div>
  )
}
