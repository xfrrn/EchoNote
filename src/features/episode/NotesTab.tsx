import { useNavigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import type { Episode } from '../../shared/types'
import { useCaptureStore } from '../../shared/store/capture'
import { NoteItem } from './NoteItem'
import { EmptyState } from '../../shared/components/EmptyState'

export function NotesTab({ episode }: { episode: Episode }) {
  const navigate = useNavigate()
  const setEpisodeId = useCaptureStore((state) => state.setEpisodeId)

  const startCapture = () => {
    setEpisodeId(episode.id)
    navigate('/capture')
  }

  return (
    <div>
      {episode.notes.length === 0 ? (
        <EmptyState
          title="还没有笔记"
          detail="听节目时产生的想法，可以随时用底部「记录」快速记下来。笔记会按创建时间排列在这里。"
        />
      ) : (
        <div className="divide-y divide-hairline">
          {episode.notes.map((note) => (
            <NoteItem key={note.id} note={note} />
          ))}
        </div>
      )}

      <div className="mt-2 px-4">
        <button
          type="button"
          onClick={startCapture}
          className="flex min-h-11 w-full items-center gap-2 text-accent transition-colors duration-fast ease-ios active:bg-subtle"
        >
          <Plus size={19} strokeWidth={2} aria-hidden />
          <span className="text-callout">记录想法</span>
        </button>
      </div>
    </div>
  )
}
