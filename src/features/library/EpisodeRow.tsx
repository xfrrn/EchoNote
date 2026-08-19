import { Link } from 'react-router-dom'
import { ChevronRight } from 'lucide-react'
import type { Episode } from '../../shared/types'
import { ShowCover } from '../../shared/components/ShowCover'
import { statusLabels, statusTextClass } from '../../shared/mock/episodes'

interface EpisodeRowProps {
  episode: Episode
}

export function EpisodeRow({ episode }: EpisodeRowProps) {
  const noteLabel = episode.notes.length === 0 ? '暂无笔记' : `${episode.notes.length} 条记录`

  return (
    <Link
      to={`/episode/${episode.id}`}
      className="group flex min-h-[92px] items-center gap-3.5 px-4 py-4 transition-colors duration-fast ease-ios active:bg-subtle"
    >
      <ShowCover showTitle={episode.showTitle} glyph={episode.coverLabel} size="md" />
      <div className="min-w-0 flex-1">
        <p className="text-caption text-ink-secondary">{episode.showTitle}</p>
        <h2 className="mt-0.5 line-clamp-2 text-headline text-ink">{episode.episodeTitle}</h2>
        <p className="mt-1 text-caption text-ink-tertiary">
          {noteLabel}
          <span aria-hidden className="mx-1.5">
            ·
          </span>
          <span className={statusTextClass(episode.status)}>{statusLabels[episode.status]}</span>
        </p>
      </div>
      <ChevronRight
        size={18}
        strokeWidth={1.8}
        className="shrink-0 text-ink-tertiary transition-transform duration-fast ease-ios group-active:translate-x-0.5"
        aria-hidden
      />
    </Link>
  )
}
