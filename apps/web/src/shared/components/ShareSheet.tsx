import { useEffect, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Check, Copy, SquareArrowOutUpRight } from 'lucide-react'
import type { CreateExportRequest, EpisodeDetail, ExportMode } from '../api/client'
import { api } from '../api/client'
import { Sheet } from './Sheet'

type Options = { notes: boolean; summary: boolean; points: boolean; review: boolean; transcript: boolean }
const defaults: Options = { notes: true, summary: true, points: true, review: true, transcript: false }
const modes: { value: ExportMode; label: string; detail: string }[] = [
  { value: 'notes_only', label: '仅我的笔记', detail: '只导出手动记录的内容' },
  { value: 'organized_note', label: '整理笔记', detail: '组合笔记、AI 与 Transcript' },
  { value: 'selected_transcript', label: 'Transcript 选段', detail: '只导出勾选的片段' },
  { value: 'full_transcript', label: '完整 Transcript', detail: '按时间顺序导出全文' }
]
const meta: { key: keyof Options; label: string; detail: string }[] = [
  { key: 'notes', label: '我的笔记', detail: '你记录的想法' },
  { key: 'summary', label: '一句话总结', detail: 'AI 提炼的本期主旨' },
  { key: 'points', label: '核心观点', detail: 'AI 梳理的重点' },
  { key: 'review', label: '值得回顾', detail: '值得重听的片段' },
  { key: 'transcript', label: 'Transcript 节选', detail: '前几段正文作为上下文' }
]

export function ShareSheet({ open, onOpenChange, episode }: { open: boolean; onOpenChange: (open: boolean) => void; episode: EpisodeDetail }) {
  const [mode, setMode] = useState<ExportMode>('organized_note')
  const [options, setOptions] = useState(defaults)
  const [selectedSegmentIds, setSelectedSegmentIds] = useState<string[]>([])
  const [feedback, setFeedback] = useState('')
  const [busy, setBusy] = useState(false)
  const canShare = typeof navigator.share === 'function'
  const selected = Object.values(options).filter(Boolean).length
  const transcript = useQuery({
    queryKey: ['transcript', episode.id], queryFn: () => api.getTranscript(episode.id),
    enabled: open && mode === 'selected_transcript'
  })
  const segments = useInfiniteQuery({
    queryKey: ['export-segments', transcript.data?.id],
    queryFn: ({ pageParam }) => api.listSegments(transcript.data!.id, pageParam),
    enabled: open && mode === 'selected_transcript' && Boolean(transcript.data), initialPageParam: 0,
    getNextPageParam: (last) => last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined
  })
  const segmentItems = segments.data?.pages.flatMap((page) => page.items) ?? []
  const canBuild = mode === 'organized_note' ? selected > 0 : mode !== 'selected_transcript' || selectedSegmentIds.length > 0

  useEffect(() => setSelectedSegmentIds([]), [transcript.data?.id])

  const build = () => {
    if (mode === 'organized_note') return api.createExport(episode.id, {
      mode, include_user_notes: options.notes, include_summary: options.summary,
      include_key_points: options.points, include_worth_reviewing: options.review, include_transcript: options.transcript
    } satisfies CreateExportRequest)
    if (mode === 'selected_transcript') return api.createExport(episode.id, { mode, transcript_segment_ids: selectedSegmentIds })
    return api.createExport(episode.id, { mode })
  }

  const copy = async () => {
    setBusy(true); setFeedback('')
    try { const output = await build(); await navigator.clipboard.writeText(output.text); setFeedback('已复制全文。') }
    catch (reason) { setFeedback(reason instanceof Error ? reason.message : '导出失败。') }
    finally { setBusy(false) }
  }

  const share = async () => {
    if (!canShare) { await copy(); return }
    setBusy(true); setFeedback('')
    try { const output = await build(); await navigator.share({ title: output.title, text: output.text }); setFeedback('已调出系统分享。') }
    catch (reason) { if (!(reason instanceof DOMException && reason.name === 'AbortError')) setFeedback(reason instanceof Error ? reason.message : '分享失败。') }
    finally { setBusy(false) }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="导出与分享" description="由服务器从同一份节目快照生成">
      <div className="px-4">
        <fieldset className="mt-2 overflow-hidden rounded-lg bg-surface">
          <legend className="sr-only">导出范围</legend>
          <div className="divide-y divide-hairline">{modes.map((item) => <label key={item.value} className="flex cursor-pointer items-center justify-between gap-3 px-4 py-3.5"><span><span className="block text-body text-ink">{item.label}</span><span className="mt-0.5 block text-caption text-ink-tertiary">{item.detail}</span></span><input type="radio" name="export-mode" value={item.value} checked={mode === item.value} onChange={() => { setMode(item.value); setFeedback('') }} className="h-5 w-5 accent-accent" /></label>)}</div>
        </fieldset>
        {mode === 'organized_note' ? <div className="mt-4 overflow-hidden rounded-lg bg-surface"><div className="divide-y divide-hairline">{meta.map((option) => { const checked = options[option.key]; return <button key={option.key} type="button" role="checkbox" aria-checked={checked} onClick={() => { setFeedback(''); setOptions((value) => ({ ...value, [option.key]: !value[option.key] })) }} className="flex w-full items-center justify-between gap-3 px-4 py-3.5 text-left active:bg-subtle"><span><span className="block text-body text-ink">{option.label}</span><span className="mt-0.5 block text-caption text-ink-tertiary">{option.detail}</span></span><span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full border ${checked ? 'border-accent bg-accent text-on-accent' : 'border-hairline-strong text-transparent'}`}><Check size={15} strokeWidth={2.6} aria-hidden /></span></button> })}</div></div> : null}
        {mode === 'selected_transcript' ? <fieldset className="mt-4 overflow-hidden rounded-lg bg-surface"><legend className="px-4 pb-2 pt-3 text-caption-medium text-ink-secondary">选择 Transcript 片段（最多 200 段）</legend>{segments.isPending ? <p className="px-4 pb-4 text-callout text-ink-secondary">正在载入…</p> : segments.isError || transcript.isError ? <p role="alert" className="px-4 pb-4 text-callout text-danger">无法载入 Transcript。</p> : <><div className="divide-y divide-hairline">{segmentItems.map((segment) => { const checked = selectedSegmentIds.includes(segment.id); return <label key={segment.id} className="flex cursor-pointer gap-3 px-4 py-3"><input type="checkbox" checked={checked} disabled={!checked && selectedSegmentIds.length >= 200} onChange={(event) => { setFeedback(''); setSelectedSegmentIds((ids) => event.target.checked ? [...ids, segment.id] : ids.filter((id) => id !== segment.id)) }} className="mt-0.5 h-5 w-5 shrink-0 accent-accent disabled:opacity-40" /><span className="text-callout text-ink">{segment.text}</span></label> })}</div>{segments.hasNextPage ? <button type="button" disabled={segments.isFetchingNextPage} onClick={() => void segments.fetchNextPage()} className="min-h-11 w-full border-t border-hairline text-callout text-accent disabled:opacity-40">{segments.isFetchingNextPage ? '载入中…' : '载入更多'}</button> : null}</>}</fieldset> : null}
        <div className="mt-5 space-y-2.5 pb-2">
          <button type="button" onClick={() => void share()} disabled={!canBuild || busy} className="flex min-h-12 w-full items-center justify-center gap-2 rounded-md bg-accent text-body font-medium text-on-accent disabled:opacity-40"><SquareArrowOutUpRight size={18} aria-hidden />{canShare ? '系统分享' : '复制到剪贴板'}</button>
          {canShare ? <button type="button" onClick={() => void copy()} disabled={!canBuild || busy} className="flex min-h-11 w-full items-center justify-center gap-2 rounded-md bg-subtle text-body text-ink disabled:opacity-40"><Copy size={17} aria-hidden />复制全文</button> : null}
          <p role="status" className="pt-1 text-center text-caption text-ink-tertiary">{busy ? '正在生成导出内容…' : feedback || 'iPhone 上可直接选择「备忘录」。'}</p>
        </div>
      </div>
    </Sheet>
  )
}
