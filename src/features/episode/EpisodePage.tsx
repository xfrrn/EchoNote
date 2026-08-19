import { useState } from 'react'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ChevronLeft } from 'lucide-react'
import { useEpisode, statusLabels } from '../../shared/mock/episodes'
import { SegmentedControl } from '../../shared/components/SegmentedControl'
import { EmptyState } from '../../shared/components/EmptyState'
import { NotesTab } from './NotesTab'
import { TranscriptTab } from './TranscriptTab'
import { AiTab } from './AiTab'

type EpisodeTab = 'notes' | 'transcript' | 'ai'

export function EpisodePage() {
  const { id = '' } = useParams()
  const episode = useEpisode(id)
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const initialTab = (searchParams.get('tab') as EpisodeTab) ?? 'notes'
  const [tab, setTab] = useState<EpisodeTab>(['notes', 'transcript', 'ai'].includes(initialTab) ? initialTab : 'notes')

  const goBack = () => {
    if (location.key !== 'default') navigate(-1)
    else navigate('/library')
  }

  if (!episode) {
    return (
      <div>
        <header className="flex min-h-11 items-center px-2 pt-3">
          <button
            type="button"
            onClick={goBack}
            className="flex min-h-11 items-center gap-0.5 pl-1 pr-3 text-accent"
          >
            <ChevronLeft size={24} strokeWidth={2.2} aria-hidden />
            <span className="text-headline">节目</span>
          </button>
        </header>
        <EmptyState title="没有找到这个节目" detail="它可能只是存在于 Mock 数据之外。" />
      </div>
    )
  }

  return (
    <div>
      <header className="px-2 pt-3">
        <button
          type="button"
          onClick={goBack}
          className="flex min-h-11 items-center gap-0.5 pl-1 pr-3 text-accent"
        >
          <ChevronLeft size={24} strokeWidth={2.2} aria-hidden />
          <span className="text-headline">节目</span>
        </button>
      </header>

      <div className="px-4 pt-2">
        <p className="text-caption-medium text-ink-secondary">{episode.showTitle}</p>
        <h1 className="mt-2 text-title-1 text-balance text-ink">
          {episode.episodeTitleLong}
        </h1>
        <p className="mt-3 text-caption text-ink-tertiary">
          {episode.durationMin} 分钟
          <span aria-hidden className="mx-1.5">
            ·
          </span>
          {statusLabels[episode.status]}
        </p>
      </div>

      <div className="sticky top-[env(safe-area-inset-top)] z-20 mt-4 border-b border-hairline bg-canvas px-4 pb-2 pt-1">
        <SegmentedControl<EpisodeTab>
          ariaLabel="节目内容视图"
          value={tab}
          onChange={setTab}
          options={[
            { value: 'notes', label: '笔记' },
            { value: 'transcript', label: 'Transcript' },
            { value: 'ai', label: 'AI' }
          ]}
        />
      </div>

      {tab === 'notes' ? <NotesTab episode={episode} /> : null}
      {tab === 'transcript' ? <TranscriptTab episode={episode} /> : null}
      {tab === 'ai' ? <AiTab episode={episode} /> : null}
    </div>
  )
}
