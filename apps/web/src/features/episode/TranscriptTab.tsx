import { useEffect, useState } from 'react'
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type EpisodeDetail, type TranscriptionProfile, type TranscriptionRun } from '../../shared/api/client'
import { TranscriptSegmentItem } from './TranscriptSegmentItem'

const terminal = (status?: TranscriptionRun['status']) => status === 'completed' || status === 'failed' || status === 'canceled'
const transcriptionEvents = ['queued', 'audio_download_started', 'audio_downloaded', 'audio_prepared', 'chunks_planned', 'chunk_transcription_started', 'chunk_transcription_completed', 'speaker_alignment_started', 'speaker_alignment_low_confidence', 'transcript_merged', 'completed', 'failed', 'retried', 'canceled']

export function TranscriptTab({ episode }: { episode: EpisodeDetail }) {
  const queryClient = useQueryClient()
  const [createdRunId, setCreatedRunId] = useState('')
  const runId = createdRunId || episode.transcription_run_id || ''
  const run = useQuery({ queryKey: ['transcription', runId], queryFn: () => api.getTranscription(runId), enabled: Boolean(runId), refetchInterval: (query) => terminal(query.state.data?.status) ? false : 2000 })
  const transcript = useQuery({ queryKey: ['transcript', episode.id], queryFn: () => api.getTranscript(episode.id), enabled: episode.transcription_status === 'completed' })
  const segments = useInfiniteQuery({
    queryKey: ['transcript-segments', transcript.data?.id],
    queryFn: ({ pageParam }) => api.listSegments(transcript.data!.id, pageParam),
    enabled: Boolean(transcript.data), initialPageParam: 0,
    getNextPageParam: (last) => last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined
  })
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['episode', episode.id] })
    if (runId) void queryClient.invalidateQueries({ queryKey: ['transcription', runId] })
  }
  const start = useMutation({ mutationFn: (profile: TranscriptionProfile) => api.createTranscription(episode.id, profile), onSuccess: (value) => { setCreatedRunId(value.id); refresh() } })
  const retry = useMutation({ mutationFn: () => api.retryTranscription(runId), onSuccess: refresh })
  const cancel = useMutation({ mutationFn: () => api.cancelTranscription(runId), onSuccess: refresh })
  const rename = useMutation({ mutationFn: ({ speakerId, name }: { speakerId: string; name: string }) => api.renameSpeaker(transcript.data!.id, speakerId, name), onSuccess: () => queryClient.invalidateQueries({ queryKey: ['transcript', episode.id] }) })
  const merge = useMutation({ mutationFn: ({ sourceId, targetId }: { sourceId: string; targetId: string }) => api.mergeSpeakers(transcript.data!.id, sourceId, targetId), onSuccess: () => { void queryClient.invalidateQueries({ queryKey: ['transcript', episode.id] }); void queryClient.invalidateQueries({ queryKey: ['transcript-segments', transcript.data?.id] }) } })

  useEffect(() => {
    if (!runId || !run.data || terminal(run.data.status)) return
    const source = new EventSource(`/api/v1/transcriptions/${encodeURIComponent(runId)}/events`)
    const update = () => refresh()
    transcriptionEvents.forEach((name) => source.addEventListener(name, update))
    return () => source.close()
  }, [runId, run.data?.status]) // eslint-disable-line react-hooks/exhaustive-deps

  const actionError = start.error ?? retry.error ?? cancel.error ?? rename.error ?? merge.error
  const current = run.data
  const active = current && !terminal(current.status)
  const allSegments = segments.data?.pages.flatMap((page) => page.items) ?? []
  const speakers = new Map(transcript.data?.speakers.map((speaker) => [speaker.id, speaker]))

  const renameSpeaker = (id: string, currentName: string) => {
    const name = window.prompt('新的说话人名称', currentName)?.trim()
    if (name && name !== currentName) rename.mutate({ speakerId: id, name })
  }
  const mergeSpeaker = (sourceId: string) => {
    const targets = transcript.data?.speakers.filter((speaker) => speaker.id !== sourceId) ?? []
    const targetName = window.prompt(`合并到哪位说话人？\n${targets.map((speaker) => speaker.display_name).join('、')}`)?.trim()
    const target = targets.find((speaker) => speaker.display_name === targetName)
    if (target) merge.mutate({ sourceId, targetId: target.id })
  }

  return (
    <div className="pb-6">
      <div className="px-4 pt-4">
        {current ? <p className="text-caption text-ink-tertiary">{current.model} · 转录阶段：{current.stage} · {current.completed_chunks}/{current.total_chunks} 个分片</p> : null}
        {current?.error ? <p role="alert" className="mt-2 text-callout text-danger">{current.error.code}: {current.error.message}</p> : null}
        {actionError ? <p role="alert" className="mt-2 text-callout text-danger">{actionError instanceof Error ? actionError.message : '操作失败。'}</p> : null}
        {episode.resolve_status !== 'completed' ? <p className="text-body text-ink-secondary">节目链接仍在解析，完成后才能开始转录。</p> : null}
        {episode.resolve_status === 'completed' && !runId ? <div className="grid grid-cols-2 gap-2"><button type="button" disabled={start.isPending} onClick={() => start.mutate('economy')} className="min-h-14 rounded-md bg-subtle px-3 text-callout font-medium text-ink disabled:opacity-40">标准转录<span className="block text-caption font-normal text-ink-tertiary">Paraformer V2</span></button><button type="button" disabled={start.isPending} onClick={() => start.mutate('quality')} className="min-h-14 rounded-md bg-accent px-3 text-callout font-medium text-on-accent disabled:opacity-40">高质量转录<span className="block text-caption font-normal opacity-80">Fun-ASR</span></button></div> : null}
        {current?.status === 'failed' ? <button type="button" disabled={retry.isPending} onClick={() => retry.mutate()} className="mt-3 min-h-11 rounded-md bg-accent px-4 text-callout font-medium text-on-accent disabled:opacity-40">重试转录</button> : null}
        {active ? <button type="button" disabled={cancel.isPending} onClick={() => cancel.mutate()} className="mt-3 min-h-11 rounded-md bg-subtle px-4 text-callout text-danger disabled:opacity-40">取消转录</button> : null}
      </div>

      {transcript.isPending && episode.transcription_status === 'completed' ? <p className="px-4 py-8 text-body text-ink-secondary">正在载入 Transcript…</p> : null}
      {transcript.data ? (
        <>
          <div className="px-4 pt-5"><p className="text-caption text-ink-tertiary">自动转录 · 已区分 {transcript.data.speakers.length} 位说话人</p><div className="mt-2 flex flex-wrap gap-2">{transcript.data.speakers.map((speaker) => <span key={speaker.id} className="rounded-md bg-subtle px-2 py-1 text-caption text-ink-secondary">{speaker.display_name}<button type="button" onClick={() => renameSpeaker(speaker.id, speaker.display_name)} className="ml-2 min-h-8 text-accent">重命名</button>{transcript.data.speakers.length > 1 ? <button type="button" onClick={() => mergeSpeaker(speaker.id)} className="ml-2 min-h-8 text-accent">合并</button> : null}</span>)}</div></div>
          <div className="mt-1">{allSegments.map((segment, index) => <TranscriptSegmentItem key={segment.id} segment={segment} speaker={speakers.get(segment.speaker_id)} showSpeaker={index === 0 || allSegments[index - 1].speaker_id !== segment.speaker_id} />)}</div>
          {segments.hasNextPage ? <div className="px-4 pt-6"><button type="button" disabled={segments.isFetchingNextPage} onClick={() => void segments.fetchNextPage()} className="min-h-11 w-full rounded-md bg-subtle text-callout text-accent">{segments.isFetchingNextPage ? '载入中…' : '载入更多'}</button></div> : allSegments.length ? <p className="px-4 pt-8 text-center text-caption text-ink-tertiary">—— 本期 Transcript 完 ——</p> : null}
        </>
      ) : !active && episode.transcription_status !== 'completed' ? <p className="px-4 py-8 text-body text-ink-secondary">Transcript 尚未生成。</p> : null}
    </div>
  )
}
