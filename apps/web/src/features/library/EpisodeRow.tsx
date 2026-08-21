import { ChevronRight, Trash2 } from 'lucide-react'
import type { EpisodeSummary } from '../../shared/api/client'
import { ShowCover } from '../../shared/components/ShowCover'
import { RowLink } from '../../shared/components/RowLink'

const statusLabel: Record<EpisodeSummary['transcription_status'], string> = {
  waiting: '等待转录', queued: '已排队', running: '转录中', completed: '转录完成', failed: '转录失败'
}

export function EpisodeRow({ episode, deleting, onDelete }: { episode: EpisodeSummary; deleting: boolean; onDelete: (id: string, title: string) => void }) {
  const showTitle = episode.podcast?.title ?? '待解析节目'
  return (
    <div className="flex items-center">
      <RowLink to={`/episode/${episode.id}`} className="group flex min-h-[76px] min-w-0 flex-1 items-center gap-3 py-3 pl-3.5 pr-1">
        <ShowCover showTitle={showTitle} glyph={showTitle.slice(0, 1)} size="md" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-caption text-ink-tertiary">{showTitle}</p>
          <h2 className="mt-0.5 line-clamp-2 text-headline leading-snug text-ink">{episode.title || '正在解析链接…'}</h2>
          <p className="mt-0.5 text-caption text-ink-tertiary">{episode.note_count ? `${episode.note_count} 条记录` : '暂无笔记'} · {statusLabel[episode.transcription_status]}</p>
        </div>
        <ChevronRight size={18} strokeWidth={1.8} className="shrink-0 text-ink-tertiary" aria-hidden />
      </RowLink>
      <button type="button" aria-label={`删除 ${episode.title}`} disabled={deleting} onClick={() => onDelete(episode.id, episode.title)} className="mr-1 flex h-11 w-11 shrink-0 items-center justify-center rounded-full text-ink-tertiary active:bg-subtle disabled:opacity-40"><Trash2 size={17} strokeWidth={1.8} aria-hidden /></button>
    </div>
  )
}
