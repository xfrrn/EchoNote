import { useState } from 'react'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ChevronLeft, SquareArrowOutUpRight } from 'lucide-react'
import { useEpisode, statusLabels, useAiSummary } from '../../shared/mock/episodes'
import { SegmentedControl } from '../../shared/components/SegmentedControl'
import { EmptyState } from '../../shared/components/EmptyState'
import { ShowCover } from '../../shared/components/ShowCover'
import { EchoMark } from '../../shared/components/EchoMark'
import { Waveform } from '../../shared/components/Waveform'
import { ShareSheet } from '../../shared/components/ShareSheet'
import { NotesTab } from './NotesTab'
import { TranscriptTab } from './TranscriptTab'
import { AiTab } from './AiTab'

type EpisodeTab = 'notes' | 'transcript' | 'ai'

export function EpisodePage() {
  const { id = '' } = useParams()
  const episode = useEpisode(id)
  const summary = useAiSummary(id)
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const initialTab = (searchParams.get('tab') as EpisodeTab) ?? 'notes'
  const [tab, setTab] = useState<EpisodeTab>(['notes', 'transcript', 'ai'].includes(initialTab) ? initialTab : 'notes')
  const [shareOpen, setShareOpen] = useState(false)

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
      <header className="flex min-h-11 items-center justify-between px-2 pt-3">
        <button
          type="button"
          onClick={goBack}
          className="flex min-h-11 items-center gap-0.5 pl-1 pr-3 text-accent"
        >
          <ChevronLeft size={24} strokeWidth={2.2} aria-hidden />
          <span className="text-headline">节目</span>
        </button>
        {episode.aiAvailable ? (
          <button
            type="button"
            aria-label="分享到备忘录"
            onClick={() => setShareOpen(true)}
            className="flex h-11 w-11 items-center justify-center rounded-full text-accent transition-colors duration-fast ease-ios active:bg-subtle"
          >
            <SquareArrowOutUpRight size={21} strokeWidth={1.9} aria-hidden />
          </button>
        ) : null}
      </header>

      {/* 节目头：封面 + 出处 + 时长，像一篇文章的署名区 */}
      <div className="px-4 pt-2">
        <div className="flex items-center gap-3.5">
          <ShowCover showTitle={episode.showTitle} glyph={episode.coverLabel} size="lg" />
          <div className="min-w-0">
            <p className="text-subheadline font-medium text-ink">{episode.showTitle}</p>
            <p className="mt-0.5 text-caption text-ink-tertiary">
              {episode.durationMin > 0 ? `${episode.durationMin} 分钟` : '时长待定'}
              <span aria-hidden className="mx-1.5">
                ·
              </span>
              {episode.recordedLabel}
            </p>
          </div>
        </div>

        <h1 className="mt-4 font-serif text-title-1 text-balance leading-snug text-ink">
          {episode.episodeTitleLong}
        </h1>

        <div className="mt-3 flex items-center gap-2">
          {episode.status === 'transcribing' ? (
            <span className="flex items-center gap-2 text-caption-medium text-accent">
              <Waveform bars={12} seed={episode.id} animated className="h-3.5 w-14" />
              转录中
            </span>
          ) : episode.status === 'transcribed' ? (
            <span className="flex items-center gap-1.5 text-caption-medium text-ink-secondary">
              <EchoMark size={14} />
              {statusLabels[episode.status]}
            </span>
          ) : (
            <span className="text-caption-medium text-ink-secondary">{statusLabels[episode.status]}</span>
          )}
        </div>
      </div>

      <div className="sticky top-[env(safe-area-inset-top)] z-20 mt-5 border-b border-hairline bg-canvas px-4 pb-2 pt-1">
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

      <ShareSheet open={shareOpen} onOpenChange={setShareOpen} episode={episode} summary={summary} />
    </div>
  )
}
