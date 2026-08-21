import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type EpisodeDetail } from '../../shared/api/client'
import { EchoMark } from '../../shared/components/EchoMark'
import { AiChatSheet } from './AiChatSheet'

function AiSectionTitle({ children }: { children: string }) {
  return <h3 className="mb-3 mt-9 flex items-center gap-2 px-4 text-caption-medium tracking-[0.08em] text-ink-secondary first:mt-0"><span className="h-px w-4 bg-accent" aria-hidden />{children}</h3>
}

function time(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

export function AiTab({ episode }: { episode: EpisodeDetail }) {
  const queryClient = useQueryClient()
  const [chatOpen, setChatOpen] = useState(false)
  const artifacts = useQuery({
    queryKey: ['ai-artifacts', episode.id], queryFn: () => api.listArtifacts(episode.id),
    enabled: episode.transcription_status === 'completed',
    refetchInterval: (query) => query.state.data?.items.some((item) => item.status === 'queued' || item.status === 'generating') ? 2000 : false
  })
  const request = useMutation({ mutationFn: () => api.requestArtifact(episode.id), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['ai-artifacts', episode.id] }); void queryClient.invalidateQueries({ queryKey: ['episode', episode.id] }) } })
  const latest = artifacts.data?.items[0]
  const ready = artifacts.data?.items.find((item) => item.status === 'ready' && item.result)?.result

  if (episode.transcription_status !== 'completed') return <div className="px-4 py-10"><p className="text-headline text-ink">AI 整理尚未生成</p><p className="mt-2 text-body text-ink-secondary">Transcript 完成后才能整理与提问。</p></div>

  if (!ready) {
    const error = request.error ?? artifacts.error
    return <div className="px-4 py-10"><p className="text-headline text-ink">{latest?.status === 'queued' || latest?.status === 'generating' ? 'AI 正在整理' : latest?.status === 'failed' ? 'AI 整理失败' : '生成 AI 整理'}</p><p className="mt-2 text-body text-ink-secondary">{latest?.error_message ?? (error instanceof Error ? error.message : '基于 Transcript 与你的笔记生成摘要和重点。')}</p>{latest?.status !== 'queued' && latest?.status !== 'generating' ? <button type="button" disabled={request.isPending} onClick={() => request.mutate()} className="mt-4 min-h-11 rounded-md bg-accent px-4 text-callout font-medium text-on-accent disabled:opacity-40">{request.isPending ? '提交中…' : latest?.status === 'failed' ? '重新生成' : '开始整理'}</button> : null}</div>
  }

  return (
    <div className="pb-4">
      <div className="flex items-center gap-2 px-4 pt-4 text-caption text-ink-tertiary"><EchoMark size={14} className="text-accent" /><p>AI 整理 · 基于本期 Transcript 与笔记</p></div>
      <section aria-label="一句话总结"><AiSectionTitle>一句话总结</AiSectionTitle><p className="px-4 font-serif text-body-serif text-ink">{ready.one_sentence_summary}</p></section>
      <section aria-label="核心观点"><AiSectionTitle>核心观点</AiSectionTitle><ol>{ready.key_points.map((point, index) => <li key={`${index}-${point}`} className="flex gap-3.5 px-4 py-2.5"><span className="w-5 shrink-0 text-right font-serif text-body-serif font-semibold text-accent">{index + 1}</span><p className="font-serif text-body-serif text-ink">{point}</p></li>)}</ol></section>
      <section aria-label="人物观点"><AiSectionTitle>人物观点</AiSectionTitle>{ready.speaker_views.map((item) => <div key={item.speaker_id} className="px-4 py-3.5"><p className="text-subheadline font-semibold text-ink">{item.speaker_name}</p>{item.points.map((point) => <p key={point} className="mt-1.5 font-serif text-body-serif text-ink">{point}</p>)}</div>)}</section>
      <section aria-label="值得回顾"><AiSectionTitle>值得回顾</AiSectionTitle>{ready.worth_reviewing.map((item) => <div key={item.transcript_segment_id} className="px-4 py-3.5"><p className="text-caption tabular-nums text-ink-tertiary">{time(item.start_ms)} · {item.speaker_name}</p><p className="mt-1.5 font-serif text-body-serif font-medium text-ink">「{item.quote}」</p><p className="mt-1.5 text-callout text-ink-secondary">{item.reason}</p></div>)}</section>
      {ready.note_connections.length ? <section aria-label="结合我的笔记"><AiSectionTitle>结合我的笔记</AiSectionTitle>{ready.note_connections.map((item) => <div key={item.note_id} className="px-4 py-3.5"><p className="font-serif text-body-serif text-ink">{item.note}</p><p className="mt-2 border-l-2 border-accent-soft pl-3 text-callout text-ink-secondary">{item.insight}</p></div>)}</section> : null}
      <div className="h-24" />
      <div className="safe-sides fixed inset-x-0 bottom-[calc(env(safe-area-inset-bottom)+76px)] z-30 mx-auto w-full max-w-app px-4"><button type="button" onClick={() => setChatOpen(true)} className="glass-surface flex min-h-12 w-full items-center gap-2.5 rounded-full px-4 text-callout text-ink-secondary shadow-control active:text-ink"><EchoMark size={17} className="shrink-0 text-accent" />问这期节目…</button></div>
      <AiChatSheet open={chatOpen} onOpenChange={setChatOpen} episodeId={episode.id} episodeTitle={episode.title} />
    </div>
  )
}
