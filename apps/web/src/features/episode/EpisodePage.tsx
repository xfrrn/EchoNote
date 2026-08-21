import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { ChevronLeft, SquareArrowOutUpRight } from 'lucide-react'
import { api, ApiError, type EpisodeDetail, type EpisodeListResponse } from '../../shared/api/client'
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
const statusLabel = { waiting: '等待转录', queued: '已排队', running: '转录中', completed: '转录完成', failed: '转录失败' } as const

export function EpisodePage() {
  const { id = '' } = useParams()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams] = useSearchParams()
  const initialTab = (searchParams.get('tab') as EpisodeTab) ?? 'notes'
  const [tab, setTab] = useState<EpisodeTab>(['notes', 'transcript', 'ai'].includes(initialTab) ? initialTab : 'notes')
  const [shareOpen, setShareOpen] = useState(false)
  const episode = useQuery({
    queryKey: ['episode', id],
    queryFn: () => api.getEpisode(id),
    enabled: Boolean(id),
    placeholderData: () => {
      const summary = queryClient.getQueryData<EpisodeListResponse>(['episodes'])?.items.find((item) => item.id === id)
      if (summary) return { ...summary, description: '', sources: [] } satisfies EpisodeDetail
      if (id) return {
        id, title: '离线记录', description: '', duration_ms: 0, cover_url: '', resolve_status: 'pending',
        transcription_status: 'waiting', ai_status: 'waiting', source_count: 0, sources: [],
        created_at: new Date().toISOString(), updated_at: new Date().toISOString()
      } satisfies EpisodeDetail
      return undefined
    },
    refetchInterval: (query) => ['pending'].includes(query.state.data?.resolve_status ?? '') || ['queued', 'running'].includes(query.state.data?.transcription_status ?? '') || ['queued', 'running'].includes(query.state.data?.ai_status ?? '') ? 2000 : false
  })

  const goBack = () => location.key !== 'default' ? navigate(-1) : navigate('/library')

  if (episode.error instanceof ApiError && episode.error.status === 404) return <EmptyState title="没有找到这个节目" detail="它可能已被删除。" />
  const cached = queryClient.getQueryData<EpisodeListResponse>(['episodes'])?.items.find((entry) => entry.id === id)
  const item: EpisodeDetail = episode.data ?? (cached ? { ...cached, description: '', sources: [] } : {
    id, title: '离线记录', description: '', duration_ms: 0, cover_url: '', resolve_status: 'pending',
    transcription_status: 'waiting', ai_status: 'waiting', source_count: 0, sources: [],
    created_at: new Date().toISOString(), updated_at: new Date().toISOString()
  })
  const showTitle = item.podcast?.title ?? '待解析节目'
  const duration = item.duration_ms > 0 ? `${Math.round(item.duration_ms / 60000)} 分钟` : '时长待定'
  const published = item.published_at ? new Date(item.published_at).toLocaleDateString() : '发布日期待定'

  return (
    <div>
      <header className="flex min-h-11 items-center justify-between px-2 pt-3">
        <button type="button" onClick={goBack} className="flex min-h-11 items-center gap-0.5 pl-1 pr-3 text-accent"><ChevronLeft size={24} strokeWidth={2.2} aria-hidden /><span className="text-headline">节目</span></button>
        <button type="button" aria-label="导出或分享" onClick={() => setShareOpen(true)} className="flex h-11 w-11 items-center justify-center rounded-full text-accent active:bg-subtle"><SquareArrowOutUpRight size={21} strokeWidth={1.9} aria-hidden /></button>
      </header>

      <div className="px-4 pt-2">
        {episode.isError ? <p className="mb-3 text-caption text-ink-tertiary">当前离线，节目详情将在联网后刷新。</p> : null}
        <div className="flex items-center gap-3.5"><ShowCover showTitle={showTitle} glyph={showTitle.slice(0, 1)} size="lg" /><div className="min-w-0"><p className="text-subheadline font-medium text-ink">{showTitle}</p><p className="mt-0.5 text-caption text-ink-tertiary">{duration} · {published}</p></div></div>
        <h1 className="mt-4 font-serif text-title-1 text-balance leading-snug text-ink">{item.title || '正在解析链接…'}</h1>
        <div className="mt-3 flex items-center gap-2">
          {item.transcription_status === 'running' ? <div className="flex items-center gap-2 text-caption-medium text-accent"><Waveform bars={12} seed={item.id} animated className="h-3.5 w-14" />转录中</div> : item.transcription_status === 'completed' ? <span className="flex items-center gap-1.5 text-caption-medium text-ink-secondary"><EchoMark size={14} />转录完成</span> : <span className={item.transcription_status === 'failed' ? 'text-caption-medium text-danger' : 'text-caption-medium text-ink-secondary'}>{statusLabel[item.transcription_status]}</span>}
        </div>
      </div>

      <div className="sticky top-[env(safe-area-inset-top)] z-20 mt-5 border-b border-hairline bg-canvas px-4 pb-2 pt-1"><SegmentedControl<EpisodeTab> ariaLabel="节目内容视图" value={tab} onChange={setTab} options={[{ value: 'notes', label: '笔记' }, { value: 'transcript', label: 'Transcript' }, { value: 'ai', label: 'AI' }]} /></div>
      {tab === 'notes' ? <NotesTab episode={item} /> : null}
      {tab === 'transcript' ? <TranscriptTab episode={item} /> : null}
      {tab === 'ai' ? <AiTab episode={item} /> : null}
      <ShareSheet open={shareOpen} onOpenChange={setShareOpen} episode={item} />
    </div>
  )
}
